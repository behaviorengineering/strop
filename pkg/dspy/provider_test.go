package dspy

import (
	"testing"
	"time"
)

func TestProviderConfig_Validate(t *testing.T) {
	t.Parallel()
	err := ProviderConfig{}.Validate()
	if err == nil {
		t.Fatal("expected error")
	}
	ok := ProviderConfig{
		APIKey:    "k",
		Model:     "m",
		BaseURL:   "http://localhost",
		APISchema: "openai",
	}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestProviderConfig_GetTimeout(t *testing.T) {
	t.Parallel()
	fallback := 30 * time.Second
	p := ProviderConfig{}
	if got := p.GetTimeout(fallback); got != fallback {
		t.Fatalf("got %v", got)
	}
	p.Timeout = "5s"
	if got := p.GetTimeout(fallback); got != 5*time.Second {
		t.Fatalf("got %v", got)
	}
	p.Timeout = "not-a-duration"
	if got := p.GetTimeout(fallback); got != fallback {
		t.Fatalf("got %v", got)
	}
}
