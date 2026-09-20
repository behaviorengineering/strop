package dspy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppendRLMInputsDumpWritesFullContext(t *testing.T) {
	dir := t.TempDir()
	longCtx := strings.Repeat("abcdefghij", 80) // 800 chars > dspy-go 500 preview
	if err := appendRLMInputsDump(dir, rlmInputsDumpEntry{
		Type:      "inputs",
		Timestamp: "2026-01-02T03:04:05Z",
		CallID:    "call-1",
		Context:   longCtx,
		Query:     "classify package",
	}); err != nil {
		t.Fatal(err)
	}
	if err := appendRLMInputsDump(dir, rlmInputsDumpEntry{
		Type:        "result",
		Timestamp:   "2026-01-02T03:04:06Z",
		CallID:      "call-1",
		Query:       "classify package",
		FinalAnswer: "role: adapter",
		Iterations:  2,
	}); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(dir, RLMInputsDumpFile)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 jsonl lines, got %d", len(lines))
	}
	var inputs rlmInputsDumpEntry
	if err := json.Unmarshal([]byte(lines[0]), &inputs); err != nil {
		t.Fatal(err)
	}
	ctx, ok := inputs.Context.(string)
	if !ok {
		t.Fatalf("context type %T", inputs.Context)
	}
	if len(ctx) != len(longCtx) {
		t.Fatalf("context truncated: got %d want %d", len(ctx), len(longCtx))
	}
	if !strings.HasSuffix(ctx, "abcdefghij") {
		t.Fatal("context suffix lost")
	}
	var result rlmInputsDumpEntry
	if err := json.Unmarshal([]byte(lines[1]), &result); err != nil {
		t.Fatal(err)
	}
	if result.FinalAnswer != "role: adapter" || result.Iterations != 2 {
		t.Fatalf("result entry=%+v", result)
	}
}
