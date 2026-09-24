package trajectory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/behaviorengineering/strop/pkg/orchestration"
	"github.com/behaviorengineering/strop/pkg/stepplan"
	"github.com/behaviorengineering/strop/pkg/streaming"
)

const (
	extraPipelineKey          = "pipeline"
	extraJobKey               = "job"
	extraEntityKey            = "entity_id"
	extraVersionKey           = "version"
	extraDefinitionVersionKey = "definition_version"
	extraSourceFingerprintKey = "source_fingerprint"
	extraLabelPrefix          = "label_"
)

// Session is one durable predefined trajectory rooted under a filesystem sidecar.
type Session struct {
	Store   *stepplan.FileStore
	Plan    *stepplan.Plan
	Root    string
	PlanDir string
	ID      Identity
	// Resumed is true when an existing plan.json was loaded.
	Resumed bool
	// ResumedFrom is the first incomplete step id when Resumed (empty if none left).
	ResumedFrom string
}

// StepSpec describes one immutable step in the predefined path.
type StepSpec struct {
	ID           string
	Goal         string
	MaxAttempts  int
	RequiredKeys []string
}

// OpenOptions configures Open. Root must be a durable path supplied by the host.
type OpenOptions struct {
	Root  string
	ID    Identity
	Steps []StepSpec
	// ForceNew deletes any existing plan for this identity and starts fresh.
	ForceNew bool
}

// Open creates or loads a trajectory plan under Root/plans/<planID>/.
func Open(ctx context.Context, opts OpenOptions) (*Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root := strings.TrimSpace(opts.Root)
	if root == "" {
		return nil, fmt.Errorf("trajectory: root is required")
	}
	id := opts.ID
	if strings.TrimSpace(id.Pipeline) == "" || strings.TrimSpace(id.Job) == "" || strings.TrimSpace(id.EntityID) == "" {
		return nil, fmt.Errorf("trajectory: pipeline, job, and entity_id are required")
	}
	if id.DefinitionVersion <= 0 {
		id.DefinitionVersion = DefinitionVersion
	}
	if strings.TrimSpace(id.SourceFingerprint) == "" {
		return nil, fmt.Errorf("trajectory: source fingerprint is required")
	}
	if len(opts.Steps) == 0 {
		return nil, fmt.Errorf("trajectory: steps are required")
	}

	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("trajectory: mkdir root: %w", err)
	}
	store, err := stepplan.NewFileStore(root)
	if err != nil {
		return nil, fmt.Errorf("trajectory: file store: %w", err)
	}
	planID := PlanID(id)
	planDir, err := store.PlanDir(planID)
	if err != nil {
		return nil, fmt.Errorf("trajectory: plan dir: %w", err)
	}

	if opts.ForceNew {
		if err := os.RemoveAll(planDir); err != nil {
			return nil, fmt.Errorf("trajectory: force new remove: %w", err)
		}
	}

	existing, loadErr := store.LoadPlan(ctx, planID)
	if loadErr == nil {
		if err := assertIdentity(existing, id); err != nil {
			return nil, err
		}
		if err := syncSourceFingerprint(ctx, store, existing, id); err != nil {
			return nil, err
		}
		if err := store.SavePlan(ctx, existing); err != nil {
			return nil, fmt.Errorf("trajectory: save plan: %w", err)
		}
		sess := &Session{
			Store:   store,
			Plan:    existing,
			Root:    root,
			PlanDir: planDir,
			ID:      id,
			Resumed: true,
		}
		sess.ResumedFrom = firstIncomplete(ctx, store, existing)
		return sess, nil
	}
	if loadErr != nil && !isNotFound(loadErr) {
		return nil, fmt.Errorf("trajectory: load plan: %w", loadErr)
	}

	steps := make([]stepplan.Step, 0, len(opts.Steps))
	for _, spec := range opts.Steps {
		maxIter := spec.MaxAttempts
		if maxIter <= 0 {
			maxIter = orchestration.DefaultMaxAttemptsPerStep
		}
		steps = append(steps, stepplan.Step{
			ID:   sanitizeID(spec.ID),
			Goal: strings.TrimSpace(spec.Goal),
			Budget: stepplan.Budget{
				MaxIterations: maxIter,
			},
			InputRefs: []stepplan.InputRef{
				{Key: "source", Hash: id.SourceFingerprint},
			},
			DoneCriteria: stepplan.DoneCriteria{RequiredKeys: append([]string(nil), spec.RequiredKeys...)},
		})
	}
	plan, err := stepplan.NewPlan(planID, id.Pipeline+"_"+id.Job, steps, identityExtra(id))
	if err != nil {
		return nil, fmt.Errorf("trajectory: new plan: %w", err)
	}
	if err := store.SavePlan(ctx, plan); err != nil {
		return nil, fmt.Errorf("trajectory: save plan: %w", err)
	}
	return &Session{
		Store:   store,
		Plan:    plan,
		Root:    root,
		PlanDir: planDir,
		ID:      id,
		Resumed: false,
	}, nil
}

// Run executes the plan with checkpoint resume via orchestration.RunStepPlan.
func (s *Session) Run(
	ctx context.Context,
	runner orchestration.StepRunner,
	cfg orchestration.StepPlanConfig,
	eventChan streaming.EventChannel,
) (*orchestration.StepPlanResult, error) {
	if s == nil || s.Plan == nil || s.Store == nil {
		return nil, fmt.Errorf("trajectory: session is not open")
	}
	if s.Resumed && s.ResumedFrom != "" {
		streaming.SendInfo(eventChan, fmt.Sprintf(
			"Trajectory resume plan=%s from=%s path=%s",
			s.Plan.ID,
			s.ResumedFrom,
			s.PlanDir,
		))
	} else {
		streaming.SendInfo(eventChan, fmt.Sprintf(
			"Trajectory start plan=%s path=%s",
			s.Plan.ID,
			s.PlanDir,
		))
	}
	return orchestration.RunStepPlan(ctx, s.Plan, s.Store, runner, cfg, eventChan)
}

// LoadEvidence loads a complete step checkpoint payload.
func (s *Session) LoadEvidence(ctx context.Context, stepID string) (Evidence, error) {
	var zero Evidence
	if s == nil || s.Store == nil || s.Plan == nil {
		return zero, fmt.Errorf("trajectory: session is not open")
	}
	key := sanitizeID(stepID)
	cp, err := s.Store.LoadStep(ctx, s.Plan.ID, key)
	if err != nil {
		return zero, fmt.Errorf("trajectory: load step %q: %w", stepID, err)
	}
	if cp.Status != stepplan.StepStatusComplete {
		return zero, fmt.Errorf("trajectory: step %q status %q", stepID, cp.Status)
	}
	return UnmarshalEvidence(cp.Output)
}

// ListCompleted returns complete checkpoint keys in plan order.
func (s *Session) ListCompleted(ctx context.Context) ([]string, error) {
	if s == nil || s.Store == nil || s.Plan == nil {
		return nil, fmt.Errorf("trajectory: session is not open")
	}
	done, err := s.Store.ListCompleted(ctx, s.Plan.ID)
	if err != nil {
		return nil, fmt.Errorf("trajectory: list completed: %w", err)
	}
	doneSet := make(map[string]struct{}, len(done))
	for _, k := range done {
		doneSet[k] = struct{}{}
	}
	ordered := make([]string, 0, len(done))
	for _, step := range s.Plan.Steps {
		key := step.EffectiveCheckpointKey()
		if _, ok := doneSet[key]; ok {
			ordered = append(ordered, key)
		}
	}
	return ordered, nil
}

// IsStepComplete reports whether stepID has a complete checkpoint with matching fingerprint.
func (s *Session) IsStepComplete(ctx context.Context, stepID string) bool {
	if s == nil || s.Store == nil || s.Plan == nil {
		return false
	}
	step, ok := findStep(s.Plan, stepID)
	if !ok {
		return false
	}
	cp, err := s.Store.LoadStep(ctx, s.Plan.ID, step.EffectiveCheckpointKey())
	if err != nil || cp.Status != stepplan.StepStatusComplete {
		return false
	}
	want := stepplan.FingerprintInputs(step.InputRefs)
	if want == "" {
		return true
	}
	return cp.InputFingerprint == "" || cp.InputFingerprint == want
}

// SaveCompleteEvidence writes a complete checkpoint for stepID.
func (s *Session) SaveCompleteEvidence(ctx context.Context, stepID string, ev Evidence) error {
	if s == nil || s.Store == nil || s.Plan == nil {
		return fmt.Errorf("trajectory: session is not open")
	}
	step, ok := findStep(s.Plan, stepID)
	if !ok {
		return fmt.Errorf("trajectory: unknown step %q", stepID)
	}
	raw, err := MarshalEvidence(ev)
	if err != nil {
		return err
	}
	cp, err := stepplan.NewCompleteCheckpoint(s.Plan.ID, step, raw, nil)
	if err != nil {
		return fmt.Errorf("trajectory: new checkpoint: %w", err)
	}
	if err := s.Store.SaveStep(ctx, cp); err != nil {
		return fmt.Errorf("trajectory: save step: %w", err)
	}
	return nil
}

func assertIdentity(plan *stepplan.Plan, id Identity) error {
	extra := plan.Extra
	if extra == nil {
		return fmt.Errorf("trajectory: plan %q missing identity metadata", plan.ID)
	}
	if got := stringField(extra, extraPipelineKey); got != strings.TrimSpace(id.Pipeline) {
		return fmt.Errorf("trajectory: pipeline mismatch want %q got %q", id.Pipeline, got)
	}
	if got := stringField(extra, extraJobKey); got != strings.TrimSpace(id.Job) {
		return fmt.Errorf("trajectory: job mismatch want %q got %q", id.Job, got)
	}
	if got := stringField(extra, extraEntityKey); got != strings.TrimSpace(id.EntityID) {
		return fmt.Errorf("trajectory: entity mismatch want %q got %q", id.EntityID, got)
	}
	defVer := intField(extra, extraDefinitionVersionKey)
	wantDef := id.DefinitionVersion
	if wantDef <= 0 {
		wantDef = DefinitionVersion
	}
	if defVer != wantDef {
		return fmt.Errorf("trajectory: definition_version mismatch want %d got %d", wantDef, defVer)
	}
	for k, v := range id.Labels {
		key := extraLabelPrefix + sanitizeID(k)
		got := stringField(extra, key)
		if got != strings.TrimSpace(v) {
			return fmt.Errorf("trajectory: label %q mismatch want %q got %q", k, v, got)
		}
	}
	return nil
}

func syncSourceFingerprint(ctx context.Context, store stepplan.Store, plan *stepplan.Plan, id Identity) error {
	if plan.Extra == nil {
		plan.Extra = map[string]any{}
	}
	prev := stringField(plan.Extra, extraSourceFingerprintKey)
	plan.Extra[extraSourceFingerprintKey] = id.SourceFingerprint
	for i := range plan.Steps {
		refs := plan.Steps[i].InputRefs
		replaced := false
		for j := range refs {
			if refs[j].Key == "source" {
				refs[j].Hash = id.SourceFingerprint
				replaced = true
			}
		}
		if !replaced {
			plan.Steps[i].InputRefs = append([]stepplan.InputRef{{Key: "source", Hash: id.SourceFingerprint}}, refs...)
		} else {
			plan.Steps[i].InputRefs = refs
		}
	}
	if prev != "" && prev != id.SourceFingerprint {
		// Source changed: drop all checkpoints. Required so resume cannot reuse
		// scored work from a different input fingerprint.
		if err := stepplan.InvalidateFrom(ctx, store, plan, 0); err != nil {
			return fmt.Errorf("trajectory: invalidate after source change: %w", err)
		}
	}
	return nil
}

func identityExtra(id Identity) map[string]any {
	extra := map[string]any{
		extraPipelineKey:          strings.TrimSpace(id.Pipeline),
		extraJobKey:               strings.TrimSpace(id.Job),
		extraEntityKey:            strings.TrimSpace(id.EntityID),
		extraVersionKey:           id.Version,
		extraDefinitionVersionKey: id.DefinitionVersion,
		extraSourceFingerprintKey: id.SourceFingerprint,
	}
	for k, v := range id.Labels {
		extra[extraLabelPrefix+sanitizeID(k)] = strings.TrimSpace(v)
	}
	return extra
}

func firstIncomplete(ctx context.Context, store stepplan.Store, plan *stepplan.Plan) string {
	for _, step := range plan.Steps {
		cp, err := store.LoadStep(ctx, plan.ID, step.EffectiveCheckpointKey())
		if err != nil || cp.Status != stepplan.StepStatusComplete {
			return step.ID
		}
		want := stepplan.FingerprintInputs(step.InputRefs)
		if want != "" && cp.InputFingerprint != "" && cp.InputFingerprint != want {
			return step.ID
		}
	}
	return ""
}

func findStep(plan *stepplan.Plan, stepID string) (stepplan.Step, bool) {
	want := sanitizeID(stepID)
	for _, step := range plan.Steps {
		if step.EffectiveCheckpointKey() == want || sanitizeID(step.ID) == want {
			return step, true
		}
	}
	return stepplan.Step{}, false
}

func stringField(extra map[string]any, key string) string {
	v, ok := extra[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	default:
		return strings.TrimSpace(fmt.Sprint(t))
	}
}

func intField(extra map[string]any, key string) int {
	v, ok := extra[key]
	if !ok || v == nil {
		return 0
	}
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	case json.Number:
		n, _ := t.Int64()
		return int(n)
	default:
		var n int
		_, _ = fmt.Sscanf(fmt.Sprint(t), "%d", &n)
		return n
	}
}

func isNotFound(err error) bool {
	return errors.Is(err, stepplan.ErrNotFound)
}
