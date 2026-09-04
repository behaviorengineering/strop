package runner

import (
	"strings"
)

const (
	// ErrGenerationFailed identifies a generator execution failure without an application-specific code.
	ErrGenerationFailed = "DSPY_GENERATION_FAILED"
	// ErrEvaluationFailed identifies an evaluation workflow failure.
	ErrEvaluationFailed = "DSPY_EVALUATION_FAILED"
)

// OperationError identifies the strop operation that failed while preserving its cause.
type OperationError struct {
	code      string
	operation string
	message   string
	cause     error
}

// Error returns the stable code, operation, message, and cause.
func (e *OperationError) Error() string {
	if e == nil {
		return ""
	}

	parts := make([]string, 0, 3)
	if e.code != "" {
		parts = append(parts, e.code)
	}
	if e.operation != "" {
		parts = append(parts, e.operation)
	}
	if e.message != "" {
		parts = append(parts, e.message)
	}
	result := strings.Join(parts, ": ")
	if e.cause == nil {
		return result
	}
	if result == "" {
		return e.cause.Error()
	}
	return result + ": " + e.cause.Error()
}

// Unwrap returns the underlying cause for errors.Is and errors.As.
func (e *OperationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

// Code returns the stable error code.
func (e *OperationError) Code() string {
	if e == nil {
		return ""
	}
	return e.code
}

// Operation returns the strop operation that failed.
func (e *OperationError) Operation() string {
	if e == nil {
		return ""
	}
	return e.operation
}

func newOperationError(code, operation, message string, cause error) error {
	return &OperationError{
		code:      code,
		operation: operation,
		message:   message,
		cause:     cause,
	}
}

func newGenerationError(config GenerationConfig, message string, cause error) error {
	code := strings.TrimSpace(config.ErrorCode)
	if code == "" {
		code = ErrGenerationFailed
	}
	operation := "generate"
	if moduleName := strings.TrimSpace(config.ModuleName); moduleName != "" {
		operation += "." + moduleName
	}
	return newOperationError(code, operation, message, cause)
}

func newEvaluationError(job, message string, cause error) error {
	operation := "evaluate"
	if job = strings.TrimSpace(job); job != "" {
		operation += "." + job
	}
	return newOperationError(ErrEvaluationFailed, operation, message, cause)
}
