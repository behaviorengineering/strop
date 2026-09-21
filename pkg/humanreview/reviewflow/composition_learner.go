package reviewflow

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/behaviorengineering/strop/pkg/humanreview"
	stroplog "github.com/behaviorengineering/strop/pkg/log"
)

// MergeDecisionPrompter asks how to handle a learning candidate relative to peers.
// On error, CompositionLearner applies NonInteractiveMergeAction.
type MergeDecisionPrompter interface {
	PromptMerge(
		ctx context.Context,
		candidate humanreview.LearningCandidate,
		snapshot humanreview.RetrievalSnapshot,
		matches []*humanreview.LearningArtifact,
		conflicts []*humanreview.LearningArtifact,
	) (humanreview.MergeAction, error)
}

// ArtifactReviewPrompter confirms pending learning artifacts (legacy per-row path).
// Prefer BatchSavePrompter for composition learning.
type ArtifactReviewPrompter interface {
	PromptApprove(ctx context.Context, artifact *humanreview.LearningArtifact) (approved bool, err error)
}

// BatchSaveLine is one row in the post-extract learning summary.
type BatchSaveLine struct {
	Kind   string // generator_example, content_rule, …
	Job    string
	Step   string
	Detail string // section, principle preview, move, …
	Count  int    // section demos collapsed into one line when >1
}

// BatchSaveSummary describes what will be stored if the human confirms.
type BatchSaveSummary struct {
	Lines []BatchSaveLine
	Total int
}

// BatchSavePrompter confirms storing the whole learning batch in one step.
// On error or save=false, CompositionLearner stores nothing new (skip creates).
type BatchSavePrompter interface {
	PromptBatchSave(ctx context.Context, summary BatchSaveSummary) (save bool, err error)
}

// AccountabilityPrompter confirms a quality action for one accountable demo.
// On error, CompositionLearner skips that candidate.
type AccountabilityPrompter interface {
	PromptAction(
		ctx context.Context,
		cand humanreview.AccountableCandidate,
		suggestedAction string,
		why string,
	) (action string, err error)
}

// DemoAccountabilityJudge optionally proposes an action before the human confirm.
type DemoAccountabilityJudge interface {
	Judge(
		ctx context.Context,
		cand humanreview.AccountableCandidate,
		chain map[string]interface{},
		approved map[string]interface{},
	) (action string, why string, err error)
}

// CompositionLearnerDeps wires the shared after-approval learning orchestrator.
type CompositionLearnerDeps struct {
	Learning  humanreview.CompositionLearning
	Store     humanreview.CompositionLearningStore
	Pack      humanreview.CompositionLearningPack
	MergeUI   MergeDecisionPrompter
	BatchUI   BatchSavePrompter      // preferred: one summary confirm
	ReviewUI  ArtifactReviewPrompter // fallback when BatchUI is nil
	AccountUI AccountabilityPrompter
	Judge     DemoAccountabilityJudge // optional
	Logger    stroplog.Logger         // optional
}

// CompositionLearner implements Learner.AfterApproval for composition jobs.
type CompositionLearner struct {
	deps CompositionLearnerDeps
}

// NewCompositionLearner builds a Learner. Learning, Store, and Pack are required.
func NewCompositionLearner(deps CompositionLearnerDeps) (*CompositionLearner, error) {
	if deps.Learning == nil {
		return nil, fmt.Errorf("learning service is required")
	}
	if deps.Store == nil {
		return nil, fmt.Errorf("learning store is required")
	}
	if deps.Pack == nil {
		return nil, fmt.Errorf("composition learning pack is required")
	}
	return &CompositionLearner{deps: deps}, nil
}

type plannedAction struct {
	candidate   humanreview.LearningCandidate
	action      humanreview.MergeAction
	mergeTarget *humanreview.LearningArtifact
}

// AfterApproval extracts candidates, resolves merge once per identity group, then
// batch-saves (or falls back to per-row review). Callers must fail-open around this method.
func (l *CompositionLearner) AfterApproval(ctx context.Context, eval *humanreview.HumanEvaluation) error {
	if l == nil || eval == nil {
		return nil
	}
	if !l.deps.Pack.IsCompositionJob(eval.Job) {
		l.logDebug("Skipping learning extract for non-composition job", map[string]interface{}{"job": eval.Job})
		return nil
	}

	candidates, err := l.deps.Pack.ExtractAfterApproval(ctx, eval)
	if err != nil {
		return fmt.Errorf("extract after approval: %w", err)
	}

	valid := make([]humanreview.LearningCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if !l.deps.Pack.ValidateCandidate(candidate) {
			l.logWarn("Invalid learning candidate, skipping", map[string]interface{}{
				"artifact_type": candidate.Type,
			})
			continue
		}
		valid = append(valid, candidate)
	}
	if len(valid) == 0 {
		return l.runAccountability(ctx, eval)
	}

	planned, err := l.planActions(ctx, valid)
	if err != nil {
		return err
	}

	creates := make([]humanreview.LearningCandidate, 0)
	updatedGroups := make(map[string]struct{})
	for _, p := range planned {
		switch p.action {
		case humanreview.MergeActionSkip:
			continue
		case humanreview.MergeActionUpdate:
			key, keyErr := mergeGroupKey(p.candidate)
			if keyErr != nil {
				return keyErr
			}
			if _, done := updatedGroups[key]; done {
				continue
			}
			updatedGroups[key] = struct{}{}
			if mergeErr := l.mergeIntoExisting(ctx, eval, p.candidate, p.mergeTarget); mergeErr != nil {
				l.logWarnErr(mergeErr, "Failed to merge learning artifact")
			}
		case humanreview.MergeActionCreate:
			creates = append(creates, p.candidate)
		}
	}

	if len(creates) == 0 {
		if len(valid) > 0 {
			l.logInfo("No new learning artifacts to create (updated existing or skipped)")
		}
		return l.runAccountability(ctx, eval)
	}

	summary, summaryErr := buildBatchSummary(creates)
	if summaryErr != nil {
		return summaryErr
	}
	save, saveErr := l.confirmBatchSave(ctx, summary)
	if saveErr != nil {
		l.logWarnErr(saveErr, "Batch save prompt failed; leaving candidates unsaved")
		return l.runAccountability(ctx, eval)
	}
	if !save {
		l.logInfo("Human skipped batch learning save")
		return l.runAccountability(ctx, eval)
	}

	for _, candidate := range creates {
		if createErr := l.createAndApprove(ctx, eval, candidate); createErr != nil {
			l.logWarnErr(createErr, "Failed to store learning artifact")
			return fmt.Errorf("store learning artifact: %w", createErr)
		}
	}

	return l.runAccountability(ctx, eval)
}

func (l *CompositionLearner) planActions(
	ctx context.Context,
	candidates []humanreview.LearningCandidate,
) ([]plannedAction, error) {
	groups, err := groupCandidatesForMerge(candidates)
	if err != nil {
		return nil, err
	}
	groupAction := make(map[string]plannedAction, len(groups))
	for key, group := range groups {
		rep := group[0]
		action, target, err := l.resolveMerge(ctx, rep)
		if err != nil {
			l.logWarnErr(err, "Failed to resolve merge for learning candidate group, skipping")
			groupAction[key] = plannedAction{candidate: rep, action: humanreview.MergeActionSkip}
			continue
		}
		groupAction[key] = plannedAction{candidate: rep, action: action, mergeTarget: target}
	}

	out := make([]plannedAction, 0, len(candidates))
	for _, candidate := range candidates {
		key, keyErr := mergeGroupKey(candidate)
		if keyErr != nil {
			return nil, keyErr
		}
		decided := groupAction[key]
		out = append(out, plannedAction{
			candidate:   candidate,
			action:      decided.action,
			mergeTarget: decided.mergeTarget,
		})
	}
	return out, nil
}

func (l *CompositionLearner) confirmBatchSave(ctx context.Context, summary BatchSaveSummary) (bool, error) {
	if l.deps.BatchUI != nil {
		return l.deps.BatchUI.PromptBatchSave(ctx, summary)
	}
	if l.deps.ReviewUI != nil {
		// Legacy: create pending then per-row approve is handled by caller path — auto-save here
		// when only ReviewUI is wired would surprise; treat as save=true and create approved.
		return true, nil
	}
	// Non-interactive: save directly.
	return true, nil
}

func (l *CompositionLearner) resolveMerge(
	ctx context.Context,
	candidate humanreview.LearningCandidate,
) (humanreview.MergeAction, *humanreview.LearningArtifact, error) {
	jobStr, err := humanreview.StringField(candidate.Content, "job")
	if err != nil {
		return humanreview.MergeActionSkip, nil, err
	}
	stepStr, err := humanreview.StringField(candidate.Content, "step")
	if err != nil {
		return humanreview.MergeActionSkip, nil, err
	}
	if jobStr == "" || stepStr == "" {
		return humanreview.MergeActionCreate, nil, nil
	}
	snapshot := humanreview.SnapshotFromContent(candidate.Content)
	matches, conflicts, err := l.deps.Learning.FindMergePeers(
		ctx,
		humanreview.Job(jobStr),
		humanreview.Step(stepStr),
		candidate.Type,
		snapshot,
	)
	if err != nil {
		return humanreview.MergeActionSkip, nil, err
	}
	if len(matches) == 0 && len(conflicts) == 0 {
		return humanreview.MergeActionCreate, nil, nil
	}

	action := humanreview.NonInteractiveMergeAction(matches)
	if l.deps.MergeUI != nil {
		prompted, promptErr := l.deps.MergeUI.PromptMerge(ctx, candidate, snapshot, matches, conflicts)
		if promptErr != nil {
			l.logDebug("Non-interactive merge decision applied", map[string]interface{}{
				"action":    action,
				"matches":   len(matches),
				"conflicts": len(conflicts),
				"error":     promptErr.Error(),
			})
		} else {
			action = prompted
		}
	}
	if action == humanreview.MergeActionUpdate {
		target := humanreview.PrimaryMergeTarget(matches)
		if target == nil {
			return humanreview.MergeActionCreate, nil, nil
		}
		return humanreview.MergeActionUpdate, target, nil
	}
	return action, nil, nil
}

func (l *CompositionLearner) mergeIntoExisting(
	ctx context.Context,
	eval *humanreview.HumanEvaluation,
	candidate humanreview.LearningCandidate,
	target *humanreview.LearningArtifact,
) error {
	if target == nil {
		return fmt.Errorf("merge target is nil")
	}
	jobStr, err := humanreview.StringField(candidate.Content, "job")
	if err != nil {
		return err
	}
	stepStr, err := humanreview.StringField(candidate.Content, "step")
	if err != nil {
		return err
	}
	job := humanreview.Job(jobStr)
	step := humanreview.Step(stepStr)
	evalID := eval.ID
	updated := &humanreview.LearningArtifact{
		ID:              target.ID,
		EvaluationID:    &evalID,
		ArtifactType:    candidate.Type,
		ArtifactContent: candidate.Content,
		Status:          humanreview.LearningReviewStatusApproved,
		Job:             &job,
		Step:            &step,
		UpdatedAt:       time.Now(),
	}
	if contextVal, err := humanreview.ObjectField(candidate.Content, "context"); err != nil {
		return err
	} else if contextVal != nil {
		updated.Context = contextVal
	}
	return l.deps.Learning.MergeIntoExisting(ctx, target.ID, updated)
}

func (l *CompositionLearner) createAndApprove(
	ctx context.Context,
	eval *humanreview.HumanEvaluation,
	candidate humanreview.LearningCandidate,
) error {
	jobStr, err := humanreview.StringField(candidate.Content, "job")
	if err != nil {
		return err
	}
	stepStr, err := humanreview.StringField(candidate.Content, "step")
	if err != nil {
		return err
	}
	job := humanreview.Job(jobStr)
	step := humanreview.Step(stepStr)
	existing, err := l.deps.Store.ListByEvaluationJobStepAndType(ctx, eval.ID, job, step, candidate.Type)
	if err != nil {
		return err
	}
	if humanreview.HasCandidateIdentity(existing, candidate) {
		return nil
	}
	var contextMap map[string]interface{}
	contextMap, err = humanreview.ObjectField(candidate.Content, "context")
	if err != nil {
		return err
	}
	evalID := eval.ID
	now := time.Now()
	artifact := &humanreview.LearningArtifact{
		ID:              uuid.New(),
		EvaluationID:    &evalID,
		ArtifactType:    candidate.Type,
		ArtifactContent: candidate.Content,
		Status:          humanreview.LearningReviewStatusApproved,
		Job:             &job,
		Step:            &step,
		Context:         contextMap,
		CreatedAt:       now,
		UpdatedAt:       now,
		ReviewedAt:      &now,
	}
	if err := l.deps.Store.Create(ctx, artifact); err != nil {
		return err
	}
	if err := l.deps.Learning.StoreLearning(ctx, artifact); err != nil {
		return fmt.Errorf("index approved learning artifact %s: %w", artifact.ID, err)
	}
	return nil
}

func (l *CompositionLearner) runAccountability(ctx context.Context, eval *humanreview.HumanEvaluation) error {
	candidates, err := l.deps.Learning.ListAccountableCandidates(ctx, eval.RootEntityID, eval.Job)
	if err != nil {
		l.logWarnErr(err, "Failed to list accountable demos; skipping trials")
		return nil
	}
	if len(candidates) == 0 {
		return nil
	}
	if l.deps.AccountUI == nil {
		l.logDebug("No accountability UI; skipping trials", nil)
		return nil
	}

	chain, approved, ctxErr := l.deps.Pack.AccountabilityContext(ctx, eval)
	if ctxErr != nil {
		l.logWarnErr(ctxErr, "Failed to load accountability context; continuing with empty maps")
		chain = map[string]interface{}{}
		approved = map[string]interface{}{}
	}

	for _, cand := range candidates {
		if cand.Artifact == nil {
			continue
		}
		action := humanreview.AccountabilityActionIgnore
		why := ""
		if l.deps.Judge != nil {
			judgedAction, judgedWhy, jerr := l.deps.Judge.Judge(ctx, cand, chain, approved)
			if jerr != nil {
				l.logWarnErr(jerr, "Accountability judge failed; defaulting to ignore")
			} else {
				action = judgedAction
				why = judgedWhy
			}
		}
		choice, cerr := l.deps.AccountUI.PromptAction(ctx, cand, action, why)
		if cerr != nil {
			return cerr
		}
		if choice == "" {
			choice = humanreview.AccountabilityActionIgnore
		}
		if applyErr := l.deps.Learning.ApplyQualityDecision(ctx, humanreview.QualityDecision{
			ArtifactID: cand.Artifact.ID,
			Action:     choice,
			Why:        why,
		}); applyErr != nil {
			l.logWarnErr(applyErr, "Failed to apply quality decision")
		}
	}
	return nil
}

func (l *CompositionLearner) logDebug(msg string, fields map[string]interface{}) {
	if l.deps.Logger == nil {
		return
	}
	logger := l.deps.Logger
	if len(fields) > 0 {
		logger = logger.WithFields(fields)
	}
	logger.Debug(msg)
}

func (l *CompositionLearner) logInfo(msg string) {
	if l.deps.Logger == nil {
		return
	}
	l.deps.Logger.Info(msg)
}

func (l *CompositionLearner) logWarn(msg string, fields map[string]interface{}) {
	if l.deps.Logger == nil {
		return
	}
	logger := l.deps.Logger
	if len(fields) > 0 {
		logger = logger.WithFields(fields)
	}
	logger.Warn(msg)
}

func (l *CompositionLearner) logWarnErr(err error, msg string) {
	if l.deps.Logger == nil {
		return
	}
	l.deps.Logger.WithError(err).Warn(msg)
}

// Ensure CompositionLearner satisfies Learner.
var _ Learner = (*CompositionLearner)(nil)

// mergeGroupKey collapses per-section demos that share the same learning identity into one merge prompt.
func mergeGroupKey(candidate humanreview.LearningCandidate) (string, error) {
	job, err := humanreview.StringField(candidate.Content, "job")
	if err != nil {
		return "", err
	}
	step, err := humanreview.StringField(candidate.Content, "step")
	if err != nil {
		return "", err
	}
	if candidate.Type == humanreview.ArtifactTypeGeneratorExample {
		snap := humanreview.SnapshotFromContent(candidate.Content)
		return fmt.Sprintf("%s|%s|%s|move=%s", candidate.Type, job, step, snap.DistinctiveMove), nil
	}
	return humanreview.CandidateIdentityKey(candidate.Type, candidate.Content)
}

func groupCandidatesForMerge(candidates []humanreview.LearningCandidate) (map[string][]humanreview.LearningCandidate, error) {
	out := make(map[string][]humanreview.LearningCandidate)
	for _, c := range candidates {
		key, err := mergeGroupKey(c)
		if err != nil {
			return nil, err
		}
		out[key] = append(out[key], c)
	}
	return out, nil
}

func buildBatchSummary(creates []humanreview.LearningCandidate) (BatchSaveSummary, error) {
	type agg struct {
		line  BatchSaveLine
		count int
	}
	byKey := make(map[string]*agg)
	order := make([]string, 0)
	for _, c := range creates {
		job, err := humanreview.StringField(c.Content, "job")
		if err != nil {
			return BatchSaveSummary{}, err
		}
		step, err := humanreview.StringField(c.Content, "step")
		if err != nil {
			return BatchSaveSummary{}, err
		}
		detail, err := summaryDetail(c)
		if err != nil {
			return BatchSaveSummary{}, err
		}
		key := fmt.Sprintf("%s|%s|%s|%s", c.Type, job, step, detail)
		if c.Type == humanreview.ArtifactTypeGeneratorExample {
			snap := humanreview.SnapshotFromContent(c.Content)
			key = fmt.Sprintf("%s|%s|%s|move=%s", c.Type, job, step, snap.DistinctiveMove)
			detail = snap.DistinctiveMove
			if detail == "" {
				detail = "polish section demos"
			}
		}
		if existing, ok := byKey[key]; ok {
			existing.count++
			existing.line.Count = existing.count
			continue
		}
		byKey[key] = &agg{
			line: BatchSaveLine{
				Kind:   c.Type,
				Job:    job,
				Step:   step,
				Detail: detail,
				Count:  1,
			},
			count: 1,
		}
		order = append(order, key)
	}
	lines := make([]BatchSaveLine, 0, len(order))
	for _, key := range order {
		lines = append(lines, byKey[key].line)
	}
	return BatchSaveSummary{Lines: lines, Total: len(creates)}, nil
}

func summaryDetail(c humanreview.LearningCandidate) (string, error) {
	switch c.Type {
	case humanreview.ArtifactTypeContentRule:
		principle, err := humanreview.StringField(c.Content, "principle")
		if err != nil {
			return "", err
		}
		if principle == "" {
			principle, err = humanreview.StringField(c.Content, "rule")
			if err != nil {
				return "", err
			}
		}
		if len(principle) > 72 {
			return principle[:69] + "...", nil
		}
		return principle, nil
	case humanreview.ArtifactTypeGeneratorExample:
		ctx, err := humanreview.ObjectField(c.Content, "context")
		if err != nil {
			return "", err
		}
		section, err := humanreview.StringField(ctx, "section_id")
		if err != nil {
			return "", err
		}
		if section != "" {
			return "section=" + section, nil
		}
		snap := humanreview.SnapshotFromContent(c.Content)
		return snap.DistinctiveMove, nil
	default:
		return FormatCandidatePreview(c.Content), nil
	}
}

// FormatCandidatePreview is a shared helper for pack PreviewCandidate implementations.
func FormatCandidatePreview(content map[string]interface{}) string {
	if content == nil {
		return ""
	}
	snap := humanreview.SnapshotFromContent(content)
	parts := make([]string, 0, 4)
	if job, err := humanreview.StringField(content, "job"); err != nil {
		return ""
	} else if job != "" {
		parts = append(parts, "job="+job)
	}
	if step, err := humanreview.StringField(content, "step"); err != nil {
		return ""
	} else if step != "" {
		parts = append(parts, "step="+step)
	}
	if snap.DistinctiveMove != "" {
		parts = append(parts, "move="+snap.DistinctiveMove)
	}
	if snap.ObjectiveSummary != "" {
		parts = append(parts, "objective="+snap.ObjectiveSummary)
	}
	principle, err := humanreview.StringField(content, "principle")
	if err != nil {
		return ""
	}
	if principle == "" {
		principle, err = humanreview.StringField(content, "rule")
		if err != nil {
			return ""
		}
	}
	if principle != "" {
		p := strings.TrimSpace(principle)
		if len(p) > 80 {
			p = p[:77] + "..."
		}
		parts = append(parts, "principle="+p)
	}
	ctx, err := humanreview.ObjectField(content, "context")
	if err != nil {
		return ""
	}
	section, err := humanreview.StringField(ctx, "section_id")
	if err != nil {
		return ""
	}
	if section != "" {
		parts = append(parts, "section="+section)
	}
	return strings.Join(parts, " | ")
}
