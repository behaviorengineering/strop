package validation_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/behaviorengineering/strop/dspy/validation"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
)

// TestValidationInterceptor_EmptyFeedbackFails ensures empty mandatory feedback is a validation error.
// Callers must rely on RetryModuleInterceptor to re-run the module; do not inject synthetic feedback.
func TestValidationInterceptor_EmptyFeedbackFails(t *testing.T) {
	t.Helper()
	validator := validation.ValidateMandatoryFields([]string{"feedback"})
	interceptor := validation.ValidationInterceptor(validator, nil)
	handler := func(ctx context.Context, inputs map[string]any, opts ...core.Option) (map[string]any, error) {
		return map[string]any{"feedback": ""}, nil
	}
	info := &core.ModuleInfo{ModuleName: "process_evaluator - Feedback Analysis"}
	_, err := interceptor(context.Background(), nil, info, handler)
	if err == nil {
		t.Fatal("expected validation error for empty feedback")
	}
	if !strings.Contains(err.Error(), "mandatory field validation failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidationInterceptor_PreservesResultOnHandlerError(t *testing.T) {
	interceptor := validation.ValidationInterceptor(nil, nil)
	raw := map[string]any{"__raw_response": "<response><broken"}
	handlerErr := errors.New("XML parsing failed")
	handler := func(ctx context.Context, inputs map[string]any, opts ...core.Option) (map[string]any, error) {
		return raw, handlerErr
	}
	info := &core.ModuleInfo{ModuleName: "TestModule"}

	outputs, err := interceptor(context.Background(), nil, info, handler)
	if !errors.Is(err, handlerErr) {
		t.Fatalf("expected handler error, got: %v", err)
	}
	if outputs == nil {
		t.Fatal("expected raw outputs preserved for tracing")
	}
	if outputs["__raw_response"] != raw["__raw_response"] {
		t.Fatalf("raw response not preserved: %#v", outputs)
	}
}

// TestValidateMandatoryFields_DedupesDuplicateOutputNames ensures duplicate field names
// in Signature.Outputs only produce one validation pass per map key (no "rationale rationale" in errors).
func TestValidateMandatoryFields_DedupesDuplicateOutputNames(t *testing.T) {
	t.Helper()
	sig := core.NewSignature(nil, []core.OutputField{
		{Field: core.NewField("rationale")},
		{Field: core.NewField("chapter_titles")},
		{Field: core.NewField("rationale")},
	})
	info := core.NewModuleInfo("Chapter Detector", "DirectivesCoT", sig)
	validator := validation.ValidateMandatoryFields(nil)
	err := validator(context.Background(), nil, map[string]any{
		"rationale":      "",
		"chapter_titles": "x",
	}, info)
	if err == nil {
		t.Fatal("expected validation error")
	}
	msg := err.Error()
	if strings.Contains(msg, "[rationale rationale]") {
		t.Fatalf("expected deduped empty-fields list, got: %s", msg)
	}
	if !strings.Contains(msg, "empty fields") || !strings.Contains(msg, "rationale") {
		t.Fatalf("expected rationale in empty-fields message, got: %s", msg)
	}
}
