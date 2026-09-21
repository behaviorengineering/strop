package humanreview

import (
	"fmt"

	"github.com/google/uuid"
)

// Error is a humanreview failure with a stable code and operation.
// Error text stays the historical message so callers can keep matching it.
type Error struct {
	Code string
	Op   string
	Msg  string
}

// Error returns the historical message.
func (e *Error) Error() string {
	if e == nil {
		return "humanreview: error"
	}
	return e.Msg
}

func newError(code, op, msg string) error {
	return &Error{Code: code, Op: op, Msg: msg}
}

// ErrUnknownJob returns an error for an unregistered job.
func ErrUnknownJob(job Job) error {
	return newError("humanreview.unknown_job", "humanreview.Job", fmt.Sprintf("unknown job: %s", job))
}

// ErrEvaluationNotFound returns an error when an evaluation id is missing.
func ErrEvaluationNotFound(id uuid.UUID) error {
	return newError("humanreview.not_found", "humanreview.Evaluation", fmt.Sprintf("evaluation not found: %s", id))
}

// ErrEmptyPipelineType returns an error when pipeline type is missing.
func ErrEmptyPipelineType() error {
	return newError("humanreview.empty_pipeline", "humanreview.PipelineType", "pipeline type cannot be empty")
}

// ErrUnknownEvaluationStatus returns an error for an unexpected evaluation status.
func ErrUnknownEvaluationStatus(status string) error {
	return newError("humanreview.unknown_status", "humanreview.Evaluation", fmt.Sprintf("cannot start evaluation: existing evaluation has unknown status %s", status))
}

// ErrResetNotRejected returns an error when ResetRejected is called on a non-rejected evaluation.
func ErrResetNotRejected(status string) error {
	return newError("humanreview.reset_not_rejected", "humanreview.Evaluation", fmt.Sprintf("cannot reset evaluation with status %s (only rejected evaluations can be reset)", status))
}

// ErrBuilderNotFound returns an error when no history builder is registered for a job.
func ErrBuilderNotFound(job Job) error {
	return newError("humanreview.builder_not_found", "humanreview.HistoryBuilder", fmt.Sprintf("builder not found for job: %s", job))
}

// ErrUnknownLearningPack returns an error when no learning pack is registered for a pipeline.
func ErrUnknownLearningPack(pipelineType PipelineType) error {
	return newError("humanreview.unknown_pack", "humanreview.LearningPack", fmt.Sprintf("unknown learning pack for pipeline: %s", pipelineType))
}

// ErrMergeIdentityConflict returns an error when distinctive moves do not match (or either is empty).
func ErrMergeIdentityConflict() error {
	return newError("humanreview.merge_conflict", "humanreview.Merge", "learning merge blocked: distinctive moves conflict or are empty")
}

// ErrMergeTargetMissing returns an error when MergeIntoExisting cannot find the approved row.
func ErrMergeTargetMissing() error {
	return newError("humanreview.merge_target_missing", "humanreview.Merge", "learning merge blocked: existing artifact not found")
}

// ErrCriterionNotFound returns an error when a criterion row is missing.
func ErrCriterionNotFound(criterionName string) error {
	return newError("humanreview.criterion_not_found", "humanreview.Criterion", fmt.Sprintf("criterion evaluation not found: %s", criterionName))
}
