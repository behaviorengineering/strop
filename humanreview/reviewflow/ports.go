package reviewflow

import (
	"context"

	"github.com/google/uuid"

	"github.com/behaviorengineering/strop/evaluation/criteria"
	"github.com/behaviorengineering/strop/humanreview"
	"github.com/behaviorengineering/strop/regenerate"
)

// GenerateResult is the portable generate/regen handoff.
type GenerateResult struct {
	RootID    uuid.UUID
	ContentID uuid.UUID
}

// Prompter is the UI port (no TUI types). Regen mode is optional; return false, nil to skip.
type Prompter interface {
	PromptAlignment(ctx context.Context, eval *humanreview.HumanEvaluation) (agrees bool, comment string, err error)
	PromptRegenMode(ctx context.Context) (useResearch bool, err error)
}

// Generator runs first generate and force regen.
type Generator interface {
	Generate(ctx context.Context, rootID uuid.UUID, job humanreview.Job) (*GenerateResult, error)
	Regenerate(ctx context.Context, rootID uuid.UUID, job humanreview.Job, opts regenerate.RegenerateOptions) (*GenerateResult, error)
}

// Session is the evaluation/content port used by the live graph.
type Session interface {
	StartEvaluation(
		ctx context.Context,
		rootID uuid.UUID,
		job humanreview.Job,
		criterionIDs []criteria.CriterionID,
	) (*humanreview.HumanEvaluation, error)
	Refresh(ctx context.Context, evaluationID uuid.UUID) (*humanreview.HumanEvaluation, error)
	RecordAlignment(ctx context.Context, evaluationID uuid.UUID, agrees bool, comment string) error
	ProposeCriteria(ctx context.Context, eval *humanreview.HumanEvaluation, agrees bool, comment string) error
	AutoAgreePending(ctx context.Context, evaluationID uuid.UUID) error
	ApprovePendingContent(ctx context.Context, eval *humanreview.HumanEvaluation) error
}

// Learner runs after content approval. Nil on Ports means skip (today's behavior).
// Implementations must fail open from the completion handler's point of view:
// completion never fails because learning failed.
type Learner interface {
	AfterApproval(ctx context.Context, eval *humanreview.HumanEvaluation) error
}

// Ports is the injected surface for RegisterDefaultHandlers.
type Ports struct {
	Prompter     Prompter
	Generator    Generator
	Session      Session
	Gate         *humanreview.Gate
	Normalizer   humanreview.FeedbackNormalizer
	PipelineType humanreview.PipelineType
	Learner      Learner
	Packs        *humanreview.LearningPackRegistry
}

// RunState is mutable engine context shared by default handlers.
type RunState struct {
	EvaluationID           uuid.UUID
	RootID                 uuid.UUID
	Job                    humanreview.Job
	CriterionIDs           []criteria.CriterionID
	LastStructuredFeedback string
	ContentID              uuid.UUID
	UseResearch            bool
}
