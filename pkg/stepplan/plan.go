package stepplan

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// PlanStatus values for Plan.Status.
const (
	PlanStatusOpen      = "open"
	PlanStatusCompleted = "completed"
	PlanStatusFailed    = "failed"
)

// StepStatus values for Checkpoint.Status.
const (
	StepStatusComplete = "complete"
	StepStatusFailed   = "failed"
)

// FilePlan is the well-known plan artifact basename under a plan directory.
const FilePlan = "plan.json"

// Budget bounds one step's inference call. Zero fields mean "unset" (host/driver defaults apply).
type Budget struct {
	Timeout       time.Duration `json:"timeout,omitempty"`
	MaxIterations int           `json:"max_iterations,omitempty"`
	MaxChars      int           `json:"max_chars,omitempty"`
}

// InputRef is a content-addressed reference to step input (hash, not pasted context).
type InputRef struct {
	// Key is a host label for the input (e.g. "package", "evidence").
	Key string `json:"key"`
	// Hash is a content digest of the referenced bytes (host-chosen algorithm).
	Hash string `json:"hash"`
	// URI is an optional locator the host understands (path, forge URL, cache key).
	URI string `json:"uri,omitempty"`
}

// DoneCriteria is host-interpreted completion rules for a step.
// Strop validates shape only; the driver/host decides whether output satisfies these fields.
type DoneCriteria struct {
	// RequiredKeys lists output map keys that must be non-empty when present.
	RequiredKeys []string `json:"required_keys,omitempty"`
	// Notes is free-form guidance for the host validator (not executed by strop).
	Notes string `json:"notes,omitempty"`
}

// Step is one unit of work in a Plan.
type Step struct {
	// ID is unique within the plan (filesystem-safe: letters, digits, -, _).
	ID string `json:"id"`
	// Goal is a short human/host description of what the step must produce.
	Goal string `json:"goal"`
	// InputRefs address step inputs by hash so resume can detect stale inputs.
	InputRefs []InputRef `json:"input_refs,omitempty"`
	// Budget caps the single inference call for this step.
	Budget Budget `json:"budget,omitempty"`
	// DoneCriteria describes expected output shape for host validation.
	DoneCriteria DoneCriteria `json:"done_criteria,omitempty"`
	// CheckpointKey defaults to ID when empty; used as the checkpoint store key.
	CheckpointKey string `json:"checkpoint_key,omitempty"`
}

// EffectiveCheckpointKey returns CheckpointKey or ID.
func (s Step) EffectiveCheckpointKey() string {
	key := strings.TrimSpace(s.CheckpointKey)
	if key == "" {
		return strings.TrimSpace(s.ID)
	}
	return key
}

// Plan is an ordered, persisted work list. Persist before execution so resume never replans.
type Plan struct {
	// ID is unique under the store root (filesystem-safe).
	ID string `json:"id"`
	// Kind is an optional host label (e.g. "slice_grounding", "ci_pipeline").
	Kind string `json:"kind,omitempty"`
	// Status is open | completed | failed.
	Status string `json:"status"`
	// Steps is the ordered execution list. Order is significant.
	Steps []Step `json:"steps"`
	// CreatedAt / UpdatedAt are UTC timestamps.
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	// Extra holds host-specific metadata without strop knowing product fields.
	Extra map[string]any `json:"extra,omitempty"`
}

// TokenUsage is optional inference accounting on a checkpoint.
type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens,omitempty"`
}

// Checkpoint is the durable result of one completed (or failed) step.
type Checkpoint struct {
	// PlanID associates the checkpoint with its plan.
	PlanID string `json:"plan_id"`
	// StepID is the plan step id (may differ from CheckpointKey).
	StepID string `json:"step_id"`
	// CheckpointKey is the store key (usually step id).
	CheckpointKey string `json:"checkpoint_key"`
	// Status is complete | failed.
	Status string `json:"status"`
	// InputFingerprint is a host digest over InputRefs (or equivalent) at run time.
	InputFingerprint string `json:"input_fingerprint,omitempty"`
	// Output is opaque JSON produced by the step runner.
	Output json.RawMessage `json:"output,omitempty"`
	// Error is a durable failure message when Status is failed.
	Error string `json:"error,omitempty"`
	// Usage is optional token accounting.
	Usage *TokenUsage `json:"usage,omitempty"`
	// CompletedAt is when the checkpoint was written.
	CompletedAt time.Time `json:"completed_at"`
}

// NewPlan builds an open plan with the given ordered steps.
// Call Validate before persisting. IDs must already be filesystem-safe.
func NewPlan(id, kind string, steps []Step, extra map[string]any) (*Plan, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("stepplan: plan id is required")
	}
	now := time.Now().UTC()
	p := &Plan{
		ID:        id,
		Kind:      strings.TrimSpace(kind),
		Status:    PlanStatusOpen,
		Steps:     append([]Step(nil), steps...),
		CreatedAt: now,
		UpdatedAt: now,
		Extra:     cloneMap(extra),
	}
	if err := Validate(p); err != nil {
		return nil, err
	}
	return p, nil
}

// Validate checks plan and step shape. It does not execute done criteria.
func Validate(p *Plan) error {
	if p == nil {
		return fmt.Errorf("stepplan: plan is nil")
	}
	if err := validateID(p.ID, "plan id"); err != nil {
		return err
	}
	switch strings.TrimSpace(p.Status) {
	case "", PlanStatusOpen, PlanStatusCompleted, PlanStatusFailed:
	default:
		return fmt.Errorf("stepplan: invalid plan status %q", p.Status)
	}
	if len(p.Steps) == 0 {
		return fmt.Errorf("stepplan: plan %q has no steps", p.ID)
	}
	seen := make(map[string]struct{}, len(p.Steps))
	seenKeys := make(map[string]struct{}, len(p.Steps))
	for i, step := range p.Steps {
		if err := validateStep(step, i); err != nil {
			return err
		}
		id := strings.TrimSpace(step.ID)
		if _, ok := seen[id]; ok {
			return fmt.Errorf("stepplan: duplicate step id %q", id)
		}
		seen[id] = struct{}{}
		key := step.EffectiveCheckpointKey()
		if err := validateID(key, "checkpoint key"); err != nil {
			return err
		}
		if _, ok := seenKeys[key]; ok {
			return fmt.Errorf("stepplan: duplicate checkpoint key %q", key)
		}
		seenKeys[key] = struct{}{}
	}
	return nil
}

func validateStep(step Step, index int) error {
	if err := validateID(step.ID, "step id"); err != nil {
		return fmt.Errorf("stepplan: steps[%d]: %w", index, err)
	}
	if strings.TrimSpace(step.Goal) == "" {
		return fmt.Errorf("stepplan: steps[%d] (%s): goal is required", index, step.ID)
	}
	if step.Budget.MaxIterations < 0 {
		return fmt.Errorf("stepplan: steps[%d] (%s): max_iterations must be >= 0", index, step.ID)
	}
	if step.Budget.MaxChars < 0 {
		return fmt.Errorf("stepplan: steps[%d] (%s): max_chars must be >= 0", index, step.ID)
	}
	if step.Budget.Timeout < 0 {
		return fmt.Errorf("stepplan: steps[%d] (%s): timeout must be >= 0", index, step.ID)
	}
	for j, ref := range step.InputRefs {
		if strings.TrimSpace(ref.Key) == "" {
			return fmt.Errorf("stepplan: steps[%d] (%s): input_refs[%d]: key is required", index, step.ID, j)
		}
		if strings.TrimSpace(ref.Hash) == "" {
			return fmt.Errorf("stepplan: steps[%d] (%s): input_refs[%d]: hash is required", index, step.ID, j)
		}
	}
	return nil
}

func validateID(id, label string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("stepplan: %s is required", label)
	}
	if strings.Contains(id, "/") || strings.Contains(id, `\`) || id == "." || id == ".." {
		return fmt.Errorf("stepplan: invalid %s %q", label, id)
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '-', r == '_':
		default:
			return fmt.Errorf("stepplan: invalid %s %q", label, id)
		}
	}
	return nil
}

func cloneMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
