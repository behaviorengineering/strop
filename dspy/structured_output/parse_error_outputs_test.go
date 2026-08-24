package structured_output_test

import (
	"context"
	"strings"
	"testing"

	"github.com/behaviorengineering/strop/dspy/rawresponse"
	"github.com/behaviorengineering/strop/dspy/structured_output"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
)

func TestParseInterceptor_PreservesRawOutputsOnParseError(t *testing.T) {
	cfg := structured_output.DefaultConfig().
		WithStrictParsing(true).
		WithFallback(false).
		WithValidation(false)
	parser, err := structured_output.GetParser(structured_output.FormatXML, cfg)
	if err != nil {
		t.Fatalf("GetParser: %v", err)
	}

	raw := `<response><broken`
	interceptor := structured_output.ParseInterceptor(parser, cfg)
	handler := func(ctx context.Context, inputs map[string]any, opts ...core.Option) (map[string]any, error) {
		return map[string]any{rawresponse.CanonicalKey: raw}, nil
	}
	info := core.NewModuleInfo("TestModule", "Predict", core.NewSignature(
		[]core.InputField{{Field: core.NewField("text")}},
		[]core.OutputField{{Field: core.NewField("score")}},
	))

	outputs, err := interceptor(context.Background(), map[string]any{"text": "x"}, info, handler)
	if err == nil {
		t.Fatal("expected parse error")
	}
	if !strings.Contains(err.Error(), "XML") {
		t.Fatalf("unexpected error: %v", err)
	}
	if outputs == nil {
		t.Fatal("expected raw outputs to be preserved with parse error")
	}
	got, key := rawresponse.TextFrom(outputs)
	if key == "" || got != raw {
		t.Fatalf("expected raw response %q, got key=%q text=%q", raw, key, got)
	}
}

func TestParseInterceptor_FallbackStillClearsError(t *testing.T) {
	cfg := structured_output.DefaultConfig().
		WithStrictParsing(true).
		WithFallback(true).
		WithValidation(false)
	parser, err := structured_output.GetParser(structured_output.FormatXML, cfg)
	if err != nil {
		t.Fatalf("GetParser: %v", err)
	}

	raw := `<response><broken`
	interceptor := structured_output.ParseInterceptor(parser, cfg)
	handler := func(ctx context.Context, inputs map[string]any, opts ...core.Option) (map[string]any, error) {
		return map[string]any{rawresponse.CanonicalKey: raw}, nil
	}
	info := core.NewModuleInfo("TestModule", "Predict", core.NewSignature(
		[]core.InputField{{Field: core.NewField("text")}},
		[]core.OutputField{{Field: core.NewField("score")}},
	))

	outputs, err := interceptor(context.Background(), map[string]any{"text": "x"}, info, handler)
	if err != nil {
		t.Fatalf("fallback should clear parse error, got: %v", err)
	}
	if outputs == nil {
		t.Fatal("expected outputs")
	}
}
