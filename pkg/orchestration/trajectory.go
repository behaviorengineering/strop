package orchestration

import (
	"context"
	"sync"
	"time"
)

// FailureClass tells hosts and agents how to recover without guessing from HTTP text alone.
type FailureClass string

const (
	// FailureTransientInfra covers rate limits, breakers, brief upstream 502/503, and timeouts.
	// Recover with mechanical pacing/retry (host); never LLM PlanRepair.
	FailureTransientInfra FailureClass = "transient_infra"
	// FailureSemanticGate covers evaluator/gate rejection after a model produced output.
	FailureSemanticGate FailureClass = "semantic_gate"
	// FailureHard covers misconfiguration, license, or non-retryable logic errors.
	FailureHard FailureClass = "hard"
	// FailureCancelled covers context cancellation.
	FailureCancelled FailureClass = "cancelled"
)

// CompensationPolicy configures mechanical retry vs agentic phase compensation for one scope.
type CompensationPolicy struct {
	// MaxMechanicalAttempts is the normal generate+gate budget before compensate (0 = loop default).
	MaxMechanicalAttempts int
	// CompensateAttempts enables PhaseCompensator after mechanical exhaust (0 = skip).
	CompensateAttempts int
}

// PipelineAttempt is one recorded try in a pipeline run (composition phase, stepplan step, or refine round).
type PipelineAttempt struct {
	At           time.Time
	Scope        string // e.g. "composition:outline" or "stepplan:evidence"
	Action       string // e.g. generate_gate, compensate_plan, step_run
	Attempt      int
	FailureClass FailureClass
	Passed       bool
	Score        float64
	Feedback     string
	Err          string
}

// PipelineTrajectory is an ordered ops log for one entity+job session. Hosts attach via WithPipelineTrajectory.
// ACE playbook credit remains on dspy/ace.Manager; this trajectory is the resume/diagnose view for agents.
type PipelineTrajectory struct {
	EntityID string
	Job      string
	mu       sync.Mutex
	Attempts []PipelineAttempt
}

// NewPipelineTrajectory creates an empty trajectory for a pipeline session.
func NewPipelineTrajectory(entityID, job string) *PipelineTrajectory {
	return &PipelineTrajectory{EntityID: entityID, Job: job}
}

// Append adds an attempt record (thread-safe).
func (t *PipelineTrajectory) Append(rec PipelineAttempt) {
	if t == nil {
		return
	}
	if rec.At.IsZero() {
		rec.At = time.Now().UTC()
	}
	t.mu.Lock()
	t.Attempts = append(t.Attempts, rec)
	t.mu.Unlock()
}

// Snapshot returns a copy of attempts for read-only export (resume JSON, agent prompts).
func (t *PipelineTrajectory) Snapshot() []PipelineAttempt {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	out := append([]PipelineAttempt(nil), t.Attempts...)
	t.mu.Unlock()
	return out
}

type trajectoryKey struct{}

// WithPipelineTrajectory attaches a trajectory to ctx for orchestration loops to append into.
func WithPipelineTrajectory(ctx context.Context, t *PipelineTrajectory) context.Context {
	if ctx == nil || t == nil {
		return ctx
	}
	return context.WithValue(ctx, trajectoryKey{}, t)
}

// TrajectoryFromContext returns the trajectory when present.
func TrajectoryFromContext(ctx context.Context) *PipelineTrajectory {
	if ctx == nil {
		return nil
	}
	t, _ := ctx.Value(trajectoryKey{}).(*PipelineTrajectory)
	return t
}

func stepPlanScope(planID, stepID string) string {
	return "stepplan:" + planID + "/" + stepID
}

// RecordPipelineAttempt appends when a trajectory is on ctx. No-op otherwise.
func RecordPipelineAttempt(ctx context.Context, rec PipelineAttempt) {
	tr := TrajectoryFromContext(ctx)
	if tr == nil {
		return
	}
	tr.Append(rec)
	recordACEPipelineStep(ctx, rec)
}
