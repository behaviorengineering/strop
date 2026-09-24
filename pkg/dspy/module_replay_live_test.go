package dspy_test

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	stropdspy "github.com/behaviorengineering/strop/pkg/dspy"
	"github.com/behaviorengineering/strop/pkg/dspy/factory"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
	"github.com/XiaoConstantine/dspy-go/pkg/modules"
)

func TestLiveModuleReplay_generator(t *testing.T) {
	if os.Getenv("STROP_LIVE_MODULE_REPLAY") != "1" {
		t.Skip("set STROP_LIVE_MODULE_REPLAY=1 for live OpenAI-compatible generator replay")
	}
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("STROP_LIVE_BASE_URL")), "/")
	if baseURL == "" {
		baseURL = "http://127.0.0.1:1320/v1"
	}
	model := strings.TrimSpace(os.Getenv("STROP_LIVE_MODEL"))
	if model == "" {
		t.Fatal("STROP_LIVE_MODEL is required for live replay")
	}
	apiKey := os.Getenv("STROP_LIVE_API_KEY")
	if apiKey == "" {
		apiKey = "dummy"
	}
	apiSchema := os.Getenv("STROP_LIVE_API_SCHEMA")
	if apiSchema == "" {
		apiSchema = "openai"
	}

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(baseURL + "/models")
	if err != nil {
		t.Fatalf("provider not reachable at %s/models: %v", baseURL, err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		t.Fatalf("provider %s/models returned HTTP %d", baseURL, resp.StatusCode)
	}

	span, err := stropdspy.LoadModuleReplaySpan(filepath.Join(moduleReplayDir(t), "generator_span.json"))
	if err != nil {
		t.Fatal(err)
	}

	llmFactory := factory.NewLLMFactory(nil, 60*time.Second)
	llm, err := llmFactory.CreateLLM(context.Background(), stropdspy.ProviderConfig{
		APISchema: apiSchema,
		BaseURL:   baseURL,
		APIKey:    apiKey,
		Model:     model,
	})
	if err != nil {
		t.Fatalf("CreateLLM: %v", err)
	}

	sig := core.NewSignature(
		[]core.InputField{{Field: core.NewField(stropdspy.FieldOriginalText)}},
		[]core.OutputField{
			{Field: core.NewField(stropdspy.FieldDirectivesAck)},
			{Field: core.NewField("summary")},
		},
	).WithInstruction("Rewrite the original_text as a one-sentence summary. Fill directives_ack briefly.")
	predict := modules.NewPredict(sig)
	predict.SetLLM(llm)

	traceDir := t.TempDir()
	ctx, closeTrace, err := stropdspy.AttachModuleTrace(context.Background(), traceDir, map[string]any{
		"pipeline": "strop-module-replay-live",
		"task":     span.Task,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = closeTrace() }()

	inputs := map[string]any{}
	for k, v := range span.Fields {
		inputs[k] = v
	}
	outs, err := stropdspy.RunModule(ctx, predict, inputs, nil)
	if err != nil {
		t.Fatalf("RunModule: %v", err)
	}
	if outs == nil || strings.TrimSpace(stringify(outs["summary"])) == "" {
		t.Fatalf("expected non-empty summary, got %#v", outs)
	}
	t.Logf("module-trace dir=%s summary=%q", traceDir, stringify(outs["summary"]))
}

func stringify(v any) string {
	switch t := v.(type) {
	case string:
		return t
	default:
		return ""
	}
}
