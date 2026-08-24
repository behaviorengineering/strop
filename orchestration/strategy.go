package orchestration

import (
	"context"

	"github.com/behaviorengineering/strop/refinement"
	"github.com/behaviorengineering/strop/streaming"

	"github.com/google/uuid"
)

// LoopContext is the initial context for a refinement loop (produced by the strategy from the entity ID).
type LoopContext struct {
	NextVersion      int
	PreviousFeedback string
	State            interface{} // Job-specific state for the first iteration (e.g. previous output, nil if none).
	// InitialPreviousScore seeds loop previousScore when set (score-decrease and guardrails on the first iteration).
	InitialPreviousScore *float64
	// InitialSelectedID seeds loop selectedID when set (return last persisted version on score-decrease stop).
	InitialSelectedID *uuid.UUID
}

// IterationOutput is the result of one generate+evaluate step (opaque to the loop; strategy uses it for save and next state).
type IterationOutput struct {
	Score         float64
	Feedback      string
	Rationale     string
	EvalRationale string
	OutputState   interface{} // Parsed output to save and to pass as State to the next iteration (or for healing).
	// EvalPayload is optional; strategy may set it (e.g. *evaluation.AggregatedEvaluation) for SaveVersion to use (CriterionScores, etc.).
	EvalPayload interface{}
}

// RefinementStrategy is the per-job strategy for the refinement loop.
// Each pipeline (sayings translation, explanation, YouTube speakers, etc.) implements this and calls RunRefinementLoop.
type RefinementStrategy interface {
	// LoadContext loads the entity and returns initial version, feedback, and optional state (e.g. previous output).
	// The strategy may use refinement.ServiceInterface.CalculateVersionAndFeedback or pipeline-specific logic.
	LoadContext(ctx context.Context, entityID uuid.UUID) (*LoopContext, error)
	// GenerateAndEvaluate runs one iteration (generate + evaluate). Returns score, feedback, rationales, and output state.
	// State is the value from the previous iteration's OutputState (or from LoadContext.State for the first iteration); may be nil.
	GenerateAndEvaluate(ctx context.Context, version int, previousFeedback string, state interface{}, eventChan streaming.EventChannel) (*IterationOutput, error)
	// SaveVersion persists the version. Returns the new version's ID.
	SaveVersion(ctx context.Context, version int, outputState interface{}, out *IterationOutput) (versionID uuid.UUID, err error)
	// ContextID returns the entity ID for stopping conditions (e.g. sayingID, videoID).
	ContextID() uuid.UUID
}

// StoppingPolicy is the subset of refinement used by the loop. refinement.ServiceInterface satisfies this.
// All pipelines (sayings, YouTube) pass the global refinement service directly for full compliance.
type StoppingPolicy interface {
	CheckStoppingConditions(currentScore, previousScore float64, version int, contextID, selectedID uuid.UUID, feedback string) (shouldStop bool, returnID uuid.UUID)
	MaxHealingAttempts() int
	AttemptHealing(ctx context.Context, diagnosis *refinement.ProblemDiagnosis, originalFeedback string, previousScore, currentScore float64) (*refinement.HealingResult, error)
}

// HealingStrategy is optional: when score decreases, the loop can ask for a diagnosis and attempt healing.
// If the strategy does not implement this, no self-healing is performed.
type HealingStrategy interface {
	// DiagnoseForHealing produces a diagnosis from previous vs current state and feedback. Return nil to skip healing.
	DiagnoseForHealing(previousState, currentState interface{}, feedback string) *refinement.ProblemDiagnosis
}

// PerItemRefinementStrategy is for pipelines that have N items (e.g. chapters); the loop runs a refinement
// sub-loop per item (generate → evaluate → refine that item until accepted or max), then saves once at the end.
// State must hold a collection of items; GenerateAndEvaluateOne updates the item at itemIndex in state.
type PerItemRefinementStrategy interface {
	// LoadContext loads the entity and returns next version, initial feedback (for the first item), and state.
	// State must be ready to receive items: the strategy must pre-allocate or ensure ItemCount(state) is correct.
	LoadContext(ctx context.Context, entityID uuid.UUID) (*LoopContext, error)
	// ItemCount returns the number of items to process (e.g. number of chapters).
	ItemCount(state interface{}) int
	// GenerateAndEvaluateOne generates and evaluates a single item at itemIndex, writes the result into state,
	// and returns score, feedback, and rationale for that item. State is mutated (e.g. state.Items[itemIndex] = result).
	GenerateAndEvaluateOne(ctx context.Context, itemIndex, round int, previousFeedback string, state interface{}, eventChan streaming.EventChannel) (score float64, feedback, rationale string, err error)
	// SaveVersion persists the full state (all items) once at the end. Returns the new version's ID.
	SaveVersion(ctx context.Context, version int, state interface{}, avgScore float64, combinedFeedback, combinedRationale string) (versionID uuid.UUID, err error)
	// ContextID returns the entity ID for stopping conditions (e.g. videoID).
	ContextID() uuid.UUID
}
