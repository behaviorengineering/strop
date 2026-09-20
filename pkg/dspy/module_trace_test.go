package dspy

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	dspylogging "github.com/XiaoConstantine/dspy-go/pkg/logging"
)

func TestAttachModuleTraceWritesSessionAndAttachesContext(t *testing.T) {
	dir := t.TempDir()
	ctx, closeFn, err := AttachModuleTrace(context.Background(), dir, map[string]any{"job": "unit"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeFn != nil {
			_ = closeFn()
		}
	}()
	if dspylogging.GetTraceSession(ctx) == nil {
		t.Fatal("expected TraceSession on context")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries=%d", len(entries))
	}
	name := entries[0].Name()
	if !strings.HasPrefix(name, "module_") || !strings.HasSuffix(name, ".jsonl") {
		t.Fatalf("name=%q", name)
	}
	body, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"type":"session"`) {
		t.Fatalf("missing session event: %s", body)
	}
}

func TestAttachModuleTraceRequiresDir(t *testing.T) {
	_, _, err := AttachModuleTrace(context.Background(), "  ", nil)
	if err == nil {
		t.Fatal("expected error")
	}
}
