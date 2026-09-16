package validation_test

import (
	"context"
	"strings"
	"testing"

	"github.com/behaviorengineering/strop/dspy/validation"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
)

func TestValidateRequiredInputs_MissingAndEmpty(t *testing.T) {
	t.Parallel()
	proc := validation.ValidateRequiredInputs([]string{"repo_id", "slice_objective_ledger"})
	err := proc(context.Background(), map[string]any{
		"repo_id": "gitboard",
	}, nil)
	if err == nil {
		t.Fatal("expected error for missing ledger")
	}
	if !strings.Contains(err.Error(), "required input validation failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "slice_objective_ledger") {
		t.Fatalf("expected ledger in error: %v", err)
	}

	err = proc(context.Background(), map[string]any{
		"repo_id":                "gitboard",
		"slice_objective_ledger": "   ",
	}, nil)
	if err == nil {
		t.Fatal("expected error for empty ledger")
	}
	if !strings.Contains(err.Error(), "empty fields") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateRequiredInputs_HappyPath(t *testing.T) {
	t.Parallel()
	proc := validation.ValidateRequiredInputs([]string{"repo_id", "slice_objective_ledger"})
	err := proc(context.Background(), map[string]any{
		"repo_id":                "gitboard",
		"slice_objective_ledger": "slices:\n  - id: board\n",
		"optional_draft":         "",
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateRequiredInputs_NilFieldsUsesSignatureInputs(t *testing.T) {
	t.Parallel()
	proc := validation.ValidateRequiredInputs(nil)
	info := &core.ModuleInfo{
		ModuleName: "demo",
		Signature: core.Signature{
			Inputs: []core.InputField{
				{Field: core.NewField("repo_id")},
				{Field: core.NewField("readme_snapshot")},
			},
		},
	}
	err := proc(context.Background(), map[string]any{"repo_id": "x"}, info)
	if err == nil {
		t.Fatal("expected missing readme_snapshot")
	}
	err = proc(context.Background(), map[string]any{
		"repo_id":         "x",
		"readme_snapshot": "hello",
	}, info)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInputProcessingInterceptor_RequiredInputsRejectsBeforeHandler(t *testing.T) {
	t.Parallel()
	proc := validation.ValidateRequiredInputs([]string{"evidence"})
	interceptor := validation.InputProcessingInterceptor(proc, nil)
	called := false
	handler := func(ctx context.Context, inputs map[string]any, opts ...core.Option) (map[string]any, error) {
		called = true
		return map[string]any{"ok": true}, nil
	}
	_, err := interceptor(context.Background(), map[string]any{}, &core.ModuleInfo{ModuleName: "m"}, handler)
	if err == nil {
		t.Fatal("expected input processing error")
	}
	if called {
		t.Fatal("handler must not run when required inputs fail")
	}
	if !strings.Contains(err.Error(), "input processing failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}
