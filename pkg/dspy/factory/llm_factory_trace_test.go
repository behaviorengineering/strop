package factory

import "testing"

func TestProviderNameForTrace_polypus(t *testing.T) {
	if got := providerNameForTrace(apiSchemaOpenAI, "http://127.0.0.1:1320/v1"); got != "polypus" {
		t.Fatalf("got %s", got)
	}
	if got := providerNameForTrace(apiSchemaOpenAI, "https://openrouter.ai/api/v1"); got != apiSchemaOpenAI {
		t.Fatalf("got %s", got)
	}
	if got := providerNameForTrace(apiSchemaGoogle, "http://127.0.0.1:1320/v1"); got != apiSchemaGoogle {
		t.Fatalf("got %s", got)
	}
}
