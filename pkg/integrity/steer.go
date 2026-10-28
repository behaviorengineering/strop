package integrity

import "context"

// MissAction chooses how the Shell reacts to a validation miss before the budget exhausts.
type MissAction int

const (
	// MissContinue appends the failed assistant turn and a complete-note user turn.
	MissContinue MissAction = iota
	// MissRestart drops the failed turn and retries from the seed transcript.
	MissRestart
)

// AfterMissFunc decides continue vs restart after a non-terminal validation failure.
type AfterMissFunc func(validateErr error) MissAction

// SteerAction is the Compensate decision after a budget exhausts.
type SteerAction int

const (
	// SteerStop ends the run with ErrExhausted.
	SteerStop SteerAction = iota
	// SteerReenter runs another chat budget (optionally with a different ChatFunc).
	SteerReenter
)

// Steer is the Compensate outcome: stop or reenter the same loop procedure.
type Steer struct {
	Action   SteerAction
	Chat     ChatFunc // nil keeps the current ChatFunc.
	Restart  bool     // true starts the next budget from seed instead of the current transcript.
	Attempts int      // 0 uses Shell.MaxAttempts for the reenter budget.
}

// CompensateFunc runs once per Run/Resume when a budget exhausts validation.
// messages is the thread at exhaust and does not include lastRaw.
// Nested reenter must not invoke Compensate again.
type CompensateFunc func(ctx context.Context, messages []Message, lastRaw string, lastErr error) (Steer, error)
