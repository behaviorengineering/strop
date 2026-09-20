package stepplan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrNotFound is returned when a plan or checkpoint is missing.
var ErrNotFound = errors.New("stepplan: not found")

// Store persists plans and per-step checkpoints.
// Implementations must not require a strop-level transaction type.
type Store interface {
	// SavePlan writes the plan artifact. Call after NewPlan / before RunStepPlan.
	SavePlan(ctx context.Context, plan *Plan) error
	// LoadPlan reads the plan artifact.
	LoadPlan(ctx context.Context, planID string) (*Plan, error)

	// SaveStep writes a step checkpoint (complete or failed).
	SaveStep(ctx context.Context, cp *Checkpoint) error
	// LoadStep loads a checkpoint by plan id and checkpoint key.
	// Returns ErrNotFound when the step has not been checkpointed.
	LoadStep(ctx context.Context, planID, checkpointKey string) (*Checkpoint, error)
	// ListCompleted returns checkpoint keys with Status == complete for the plan.
	ListCompleted(ctx context.Context, planID string) ([]string, error)
}

// FingerprintInputs builds a stable fingerprint from ordered InputRefs.
// Hosts MAY use this as Checkpoint.InputFingerprint; they MAY also supply their own digest.
func FingerprintInputs(refs []InputRef) string {
	if len(refs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(refs))
	for _, r := range refs {
		parts = append(parts, strings.TrimSpace(r.Key)+"="+strings.TrimSpace(r.Hash))
	}
	return strings.Join(parts, "|")
}

// NewCompleteCheckpoint builds a complete checkpoint for a plan step.
func NewCompleteCheckpoint(planID string, step Step, output json.RawMessage, usage *TokenUsage) (*Checkpoint, error) {
	return newCheckpoint(planID, step, StepStatusComplete, output, "", usage)
}

// NewFailedCheckpoint builds a failed checkpoint for a plan step.
func NewFailedCheckpoint(planID string, step Step, errMsg string) (*Checkpoint, error) {
	return newCheckpoint(planID, step, StepStatusFailed, nil, errMsg, nil)
}

func newCheckpoint(planID string, step Step, status string, output json.RawMessage, errMsg string, usage *TokenUsage) (*Checkpoint, error) {
	planID = strings.TrimSpace(planID)
	if err := validateID(planID, "plan id"); err != nil {
		return nil, err
	}
	if err := validateStep(step, 0); err != nil {
		return nil, err
	}
	key := step.EffectiveCheckpointKey()
	if err := validateID(key, "checkpoint key"); err != nil {
		return nil, err
	}
	switch status {
	case StepStatusComplete, StepStatusFailed:
	default:
		return nil, fmt.Errorf("stepplan: invalid checkpoint status %q", status)
	}
	if status == StepStatusFailed && strings.TrimSpace(errMsg) == "" {
		return nil, fmt.Errorf("stepplan: failed checkpoint requires error message")
	}
	cp := &Checkpoint{
		PlanID:           planID,
		StepID:           strings.TrimSpace(step.ID),
		CheckpointKey:    key,
		Status:           status,
		InputFingerprint: FingerprintInputs(step.InputRefs),
		Output:           output,
		Error:            strings.TrimSpace(errMsg),
		Usage:            usage,
		CompletedAt:      time.Now().UTC(),
	}
	return cp, nil
}
