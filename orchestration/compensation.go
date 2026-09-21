package orchestration

import (
	"context"

	"github.com/behaviorengineering/strop/streaming"
)

// CompensationAttempt records one generate+gate try during normal phase retries.
type CompensationAttempt struct {
	Attempt  int
	Score    float64
	Feedback string
	Passed   bool
}

// CompensationEvidence is a product-neutral bundle the host fills after phase retries exhaust.
// Snapshot holds host-defined extras (criterion fails, dump_path, and similar).
type CompensationEvidence struct {
	PhaseID        string
	AttemptHistory []CompensationAttempt
	LockedOutput   map[string]string
	FailedOutput   map[string]string
	Feedback       string
	Snapshot       map[string]any
}

// RepairPlan is the structured output of plan inference.
// Hosts may embed richer JSON or markdown in Body for ApplyAndGate.
type RepairPlan struct {
	Summary string
	Steps   []string
	Body    string
}

// PhaseCompensator is optional. When RunCompositionLoop exhausts MaxAttempts for a phase,
// it runs CollectEvidence → PlanRepair → ApplyAndGate before returning a hard-fail error.
//
// Do not overload refinement HealingStrategy for this: that path is score-decrease on
// version refine; this path is phase-exhaust recovery with an explicit repair plan.
type PhaseCompensator interface {
	// CompensateAttempts returns how many compensate tries for this phase (0 skips).
	CompensateAttempts(phase PhaseDef) int
	// CollectEvidence builds the diagnostic bundle after normal retries exhaust.
	CollectEvidence(ctx context.Context, phase PhaseDef, feedback string, failedOutput map[string]string) (*CompensationEvidence, error)
	// PlanRepair runs inference (RLM or structured Predict) over evidence → plan.
	PlanRepair(ctx context.Context, evidence *CompensationEvidence, eventChan streaming.EventChannel) (*RepairPlan, error)
	// ApplyAndGate applies the plan and re-gates; same PhaseResult contract as RunPhase.
	ApplyAndGate(ctx context.Context, phase PhaseDef, evidence *CompensationEvidence, plan *RepairPlan, eventChan streaming.EventChannel) (*PhaseResult, error)
}
