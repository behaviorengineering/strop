package dspy_test

import (
	"testing"
	"time"

	stropdspy "github.com/behaviorengineering/strop/pkg/dspy"
)

func TestRLMDefaults(t *testing.T) {
	cfg := stropdspy.RLMDefaults()
	if cfg.MaxIterations != 12 {
		t.Fatalf("MaxIterations=%d", cfg.MaxIterations)
	}
	if cfg.Timeout != 3*time.Minute {
		t.Fatalf("Timeout=%v", cfg.Timeout)
	}
	if cfg.MaxFullContextQueryChars != 32_000 {
		t.Fatalf("MaxFullContextQueryChars=%d", cfg.MaxFullContextQueryChars)
	}
	if cfg.SubRLMMaxDepth != 2 {
		t.Fatalf("SubRLMMaxDepth=%d", cfg.SubRLMMaxDepth)
	}
}

func TestCreateRLMModuleRequiresLLM(t *testing.T) {
	_, err := stropdspy.CreateRLMModule(nil, stropdspy.RLMDefaults())
	if err == nil {
		t.Fatal("expected error for nil llm")
	}
}

func TestRLMConfigCreateModuleRequiresLLM(t *testing.T) {
	cfg := stropdspy.RLMDefaults()
	_, err := cfg.CreateModule()
	if err == nil {
		t.Fatal("expected error for nil cfg.LLM")
	}
}

func TestRLMCompleteRequiresModule(t *testing.T) {
	_, _, err := stropdspy.RLMComplete(nil, nil, "ctx", "query")
	if err == nil {
		t.Fatal("expected error for nil module")
	}
}
