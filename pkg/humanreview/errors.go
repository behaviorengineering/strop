package humanreview

import (
	"fmt"

	"github.com/google/uuid"
)

// Pure error helpers (no app DomainError dependency). Apps may wrap these.

// ErrUnknownJob returns an error for an unregistered job.
func ErrUnknownJob(job Job) error {
	return fmt.Errorf("unknown job: %s", job)
}

// ErrEvaluationNotFound returns an error when an evaluation id is missing.
func ErrEvaluationNotFound(id uuid.UUID) error {
	return fmt.Errorf("evaluation not found: %s", id)
}

// ErrEmptyPipelineType returns an error when pipeline type is missing.
func ErrEmptyPipelineType() error {
	return fmt.Errorf("pipeline type cannot be empty")
}

// ErrUnknownEvaluationStatus returns an error for an unexpected evaluation status.
func ErrUnknownEvaluationStatus(status string) error {
	return fmt.Errorf("cannot start evaluation: existing evaluation has unknown status %s", status)
}

// ErrResetNotRejected returns an error when ResetRejected is called on a non-rejected evaluation.
func ErrResetNotRejected(status string) error {
	return fmt.Errorf("cannot reset evaluation with status %s (only rejected evaluations can be reset)", status)
}

// ErrBuilderNotFound returns an error when no history builder is registered for a job.
func ErrBuilderNotFound(job Job) error {
	return fmt.Errorf("builder not found for job: %s", job)
}

// ErrUnknownLearningPack returns an error when no learning pack is registered for a pipeline.
func ErrUnknownLearningPack(pipelineType PipelineType) error {
	return fmt.Errorf("unknown learning pack for pipeline: %s", pipelineType)
}

// ErrMergeIdentityConflict returns an error when distinctive moves do not match (or either is empty).
func ErrMergeIdentityConflict() error {
	return fmt.Errorf("learning merge blocked: distinctive moves conflict or are empty")
}

// ErrMergeTargetMissing returns an error when MergeIntoExisting cannot find the approved row.
func ErrMergeTargetMissing() error {
	return fmt.Errorf("learning merge blocked: existing artifact not found")
}

// ErrCriterionNotFound returns an error when a criterion row is missing.
func ErrCriterionNotFound(criterionName string) error {
	return fmt.Errorf("criterion evaluation not found: %s", criterionName)
}
