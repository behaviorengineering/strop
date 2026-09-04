package runner

import (
	"context"
	"errors"
	"testing"

	"github.com/behaviorengineering/strop/dspy/registry"
)

type testGeneratorInput struct{}

func (testGeneratorInput) ToMap() map[string]interface{} {
	return map[string]interface{}{}
}

func (testGeneratorInput) GetVersion() int {
	return 1
}

func TestOperationErrorPreservesCodeOperationAndCause(t *testing.T) {
	t.Parallel()

	cause := errors.New("connection refused")
	err := newOperationError("POST_FAILED", "generate.post", "generation failed", cause)

	var operationErr *OperationError
	if !errors.As(err, &operationErr) {
		t.Fatal("expected OperationError in error chain")
	}
	if operationErr.Code() != "POST_FAILED" {
		t.Fatalf("code = %q, want %q", operationErr.Code(), "POST_FAILED")
	}
	if operationErr.Operation() != "generate.post" {
		t.Fatalf("operation = %q, want %q", operationErr.Operation(), "generate.post")
	}
	if !errors.Is(err, cause) {
		t.Fatal("expected cause to remain discoverable")
	}
}

func TestNewGenerationErrorUsesConfiguredCode(t *testing.T) {
	t.Parallel()

	err := newGenerationError(
		GenerationConfig{ModuleName: "post", ErrorCode: "POST_FAILED"},
		"generation failed",
		nil,
	)

	var operationErr *OperationError
	if !errors.As(err, &operationErr) {
		t.Fatal("expected OperationError in error chain")
	}
	if operationErr.Code() != "POST_FAILED" {
		t.Fatalf("code = %q, want %q", operationErr.Code(), "POST_FAILED")
	}
	if operationErr.Operation() != "generate.post" {
		t.Fatalf("operation = %q, want %q", operationErr.Operation(), "generate.post")
	}
}

func TestNewGenerationErrorUsesDefaultCode(t *testing.T) {
	t.Parallel()

	err := newGenerationError(GenerationConfig{ModuleName: "post"}, "generation failed", nil)

	var operationErr *OperationError
	if !errors.As(err, &operationErr) {
		t.Fatal("expected OperationError in error chain")
	}
	if operationErr.Code() != ErrGenerationFailed {
		t.Fatalf("code = %q, want %q", operationErr.Code(), ErrGenerationFailed)
	}
}

func TestGenerateResultUsesConfiguredErrorCode(t *testing.T) {
	t.Parallel()

	runner := NewJobRunner(registry.NewModuleRegistry(), nil, nil, nil)
	_, err := runner.GenerateResult(
		context.Background(),
		GenerationConfig{
			ModuleName:   "post",
			ErrorCode:    "POST_FAILED",
			ErrorMessage: "post generator",
		},
		testGeneratorInput{},
		nil,
	)

	var operationErr *OperationError
	if !errors.As(err, &operationErr) {
		t.Fatal("expected OperationError in error chain")
	}
	if operationErr.Code() != "POST_FAILED" {
		t.Fatalf("code = %q, want %q", operationErr.Code(), "POST_FAILED")
	}
}
