package humanreview

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/behaviorengineering/strop/log"
)

// Gate persists approval-gate sessions (start, alignment comment, reset).
// It does not change Status on RecordAlignment; the app sets status after content approve/reject.
type Gate struct {
	store  Store
	logger log.Logger
}

// NewGate creates an approval gate. store and logger must be non-nil.
func NewGate(store Store, logger log.Logger) *Gate {
	if store == nil {
		panic("store cannot be nil")
	}
	if logger == nil {
		panic("logger cannot be nil")
	}
	return &Gate{store: store, logger: logger}
}

// Start returns an in-progress evaluation for pipelineType + root + job.
// Existing in_progress is resumed; rejected is reset; completed/ready_for_learning is replaced.
// History and criterion rows may be empty; the app fills them when needed.
func (g *Gate) Start(ctx context.Context, pipelineType PipelineType, rootEntityID uuid.UUID, job Job) (*HumanEvaluation, error) {
	if pipelineType == "" {
		return nil, ErrEmptyPipelineType()
	}
	step, err := GetStepForJob(job)
	if err != nil {
		return nil, err
	}

	existing, err := g.store.GetByRootEntityID(ctx, rootEntityID, pipelineType, job)
	if err != nil {
		g.logger.WithError(err).Error("Failed to load existing evaluation")
		return nil, err
	}
	if existing != nil {
		switch existing.Status {
		case StatusInProgress:
			g.logger.WithField("evaluation_id", existing.ID).Debug("Resuming in-progress evaluation")
			return existing, nil
		case StatusRejected:
			if err := g.ResetRejected(ctx, existing.ID); err != nil {
				return nil, err
			}
			return g.store.GetByID(ctx, existing.ID)
		case StatusCompleted, StatusReadyForLearning:
			if err := g.store.DeleteByRootEntityID(ctx, rootEntityID, pipelineType, job); err != nil {
				return nil, err
			}
		default:
			return nil, ErrUnknownEvaluationStatus(existing.Status)
		}
	}

	now := time.Now()
	evaluation := &HumanEvaluation{
		ID:                   uuid.New(),
		PipelineType:         pipelineType,
		RootEntityID:         rootEntityID,
		Job:                  job,
		Step:                 step,
		PipelineHistory:      PipelineHistory{PipelineType: string(pipelineType)},
		CriterionEvaluations: []CriterionScoreEvaluation{},
		Status:               StatusInProgress,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	if err := g.store.Create(ctx, evaluation); err != nil {
		g.logger.WithError(err).Error("Failed to create evaluation")
		return nil, err
	}
	g.logger.WithField("evaluation_id", evaluation.ID).Debug("Started human evaluation")
	return evaluation, nil
}

// RecordAlignment stores the human agree/disagree decision and comment.
// It does not change Status. If alignment was already recorded, it is a no-op.
func (g *Gate) RecordAlignment(ctx context.Context, evaluationID uuid.UUID, agrees bool, comment string) error {
	evaluation, err := g.store.GetByID(ctx, evaluationID)
	if err != nil {
		return err
	}
	if evaluation == nil {
		return ErrEvaluationNotFound(evaluationID)
	}
	if evaluation.HumanAlignmentAgrees != nil {
		g.logger.WithField("evaluation_id", evaluationID).Warn("Human alignment already reviewed, skipping")
		return nil
	}
	now := time.Now()
	evaluation.HumanAlignmentAgrees = &agrees
	evaluation.HumanAlignmentComment = comment
	evaluation.HumanAlignmentReviewedAt = &now
	evaluation.UpdatedAt = now
	return g.store.Update(ctx, evaluation)
}

// ResetAlignment clears alignment fields so the human can be prompted again.
func (g *Gate) ResetAlignment(ctx context.Context, evaluationID uuid.UUID) error {
	evaluation, err := g.store.GetByID(ctx, evaluationID)
	if err != nil {
		return err
	}
	if evaluation == nil {
		return ErrEvaluationNotFound(evaluationID)
	}
	clearAlignment(evaluation)
	evaluation.UpdatedAt = time.Now()
	return g.store.Update(ctx, evaluation)
}

// ResetRejected clears alignment and criterion rows and sets status back to in_progress.
// Only rejected evaluations can be reset.
func (g *Gate) ResetRejected(ctx context.Context, evaluationID uuid.UUID) error {
	evaluation, err := g.store.GetByID(ctx, evaluationID)
	if err != nil {
		return err
	}
	if evaluation == nil {
		return ErrEvaluationNotFound(evaluationID)
	}
	if evaluation.Status != StatusRejected {
		return ErrResetNotRejected(evaluation.Status)
	}
	clearAlignment(evaluation)
	for i := range evaluation.CriterionEvaluations {
		resetCriterionScoreEvaluation(&evaluation.CriterionEvaluations[i])
	}
	evaluation.Status = StatusInProgress
	evaluation.TotalScore = 0
	evaluation.UpdatedAt = time.Now()
	return g.store.Update(ctx, evaluation)
}

// SetStatus updates evaluation status. Apps use this after content approve/reject; RecordAlignment does not.
func (g *Gate) SetStatus(ctx context.Context, evaluationID uuid.UUID, status string) error {
	evaluation, err := g.store.GetByID(ctx, evaluationID)
	if err != nil {
		return err
	}
	if evaluation == nil {
		return ErrEvaluationNotFound(evaluationID)
	}
	evaluation.Status = status
	evaluation.UpdatedAt = time.Now()
	return g.store.Update(ctx, evaluation)
}

func clearAlignment(evaluation *HumanEvaluation) {
	evaluation.HumanAlignmentAgrees = nil
	evaluation.HumanAlignmentComment = ""
	evaluation.HumanAlignmentReviewedAt = nil
}

func resetCriterionScoreEvaluation(criterion *CriterionScoreEvaluation) {
	criterion.ProposedScore = 0
	criterion.Rationale = ""
	criterion.Analysis = ""
	criterion.Evidence = nil
	criterion.HumanAgrees = nil
	criterion.HumanScore = nil
	criterion.HumanComment = ""
	criterion.FinalScore = nil
	criterion.FinalRationale = ""
	criterion.Status = CriterionScoreStatusProposed
	criterion.FirstVersionAtMax = nil
	criterion.VersionsWithImprovement = 0
	criterion.SmellPassedAtV1 = false
	criterion.PerformanceSummary = ""
	criterion.ProposedAt = time.Time{}
	criterion.ReviewedAt = nil
	criterion.AgreedAt = nil
}
