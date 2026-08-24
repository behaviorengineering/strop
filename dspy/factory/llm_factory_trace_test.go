package factory

import "testing"

func TestProviderNameForTrace_polypus(t *testing.T) {
	if got := providerNameForTrace("openai", "http://127.0.0.1:1320/v1"); got != "polypus" {
		t.Fatalf("got %s", got)
	}
	if got := providerNameForTrace("openai", "https://openrouter.ai/api/v1"); got != "openai" {
		t.Fatalf("got %s", got)
	}
	if got := providerNameForTrace("google", "http://127.0.0.1:1320/v1"); got != "google" {
		t.Fatalf("got %s", got)
	}
}
