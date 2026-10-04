package dspy_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	stropdspy "github.com/behaviorengineering/strop/pkg/dspy"
	"github.com/behaviorengineering/strop/pkg/dspy/factory"
)

func TestLiveCreateRLMPreferBatchProbe(t *testing.T) {
	if os.Getenv("STROP_LIVE_OPENAI_BATCH") != "1" {
		t.Skip("set STROP_LIVE_OPENAI_BATCH=1 for live RLM batch wiring smoke")
	}
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("STROP_LIVE_BASE_URL")), "/")
	if baseURL == "" {
		baseURL = "http://127.0.0.1:1320/v1"
	}
	model := strings.TrimSpace(os.Getenv("STROP_LIVE_MODEL"))
	if model == "" {
		t.Fatal("STROP_LIVE_MODEL is required")
	}
	apiKey := os.Getenv("STROP_LIVE_API_KEY")
	if apiKey == "" {
		apiKey = "dummy"
	}

	llmFactory := factory.NewLLMFactory(nil, 60*time.Second)
	setup := factory.NewInterceptorSetup(factory.InterceptorSetupConfig{})
	configurator := factory.NewModuleConfigurator(llmFactory, setup, nil)
	genFactory := factory.NewGeneratorFactory(configurator)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	module, err := genFactory.CreateRLM(ctx, stropdspy.ProviderConfig{
		APISchema: "openai",
		BaseURL:   baseURL,
		APIKey:    apiKey,
		Model:     model,
	}, stropdspy.RLMDefaults(), "live-rlm-batch")
	if err != nil {
		t.Fatal(err)
	}
	if module == nil {
		t.Fatal("nil module")
	}
}
