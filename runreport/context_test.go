package runreport

import (
	"context"
	"testing"
)

func TestConfigFromContext_UsesDefaultWhenUnset(t *testing.T) {
	SetDefaultConfig(Config{Enabled: true, Dir: "logs/test-runs"})
	t.Cleanup(func() { SetDefaultConfig(Config{}) })

	cfg := ConfigFromContext(context.Background())
	if !cfg.Enabled {
		t.Fatal("expected default enabled config")
	}
	if cfg.Dir != "logs/test-runs" {
		t.Fatalf("dir = %q, want logs/test-runs", cfg.Dir)
	}
}

func TestConfigFromContext_OverridesDefault(t *testing.T) {
	SetDefaultConfig(Config{Enabled: true})
	t.Cleanup(func() { SetDefaultConfig(Config{}) })

	ctx := WithConfig(context.Background(), Config{Enabled: false})
	cfg := ConfigFromContext(ctx)
	if cfg.Enabled {
		t.Fatal("ctx override should disable reports")
	}
}
