package scopeboundary

import (
	"strings"
	"testing"
)

func TestFormat_nil(t *testing.T) {
	if got := Format("post", nil); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestFormat_empty(t *testing.T) {
	b := &Boundary{InDomain: "  ", OutOfDomain: "\n"}
	if got := Format("translation", b); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestAppendToEvaluatorPrompt(t *testing.T) {
	b := &Boundary{InDomain: "- A", OutOfDomain: "- B"}
	got := AppendToEvaluatorPrompt("hello", "chapter", b)
	if !strings.HasPrefix(got, "hello") {
		t.Fatalf("expected base first: %q", got)
	}
	if !strings.Contains(got, "EVALUATOR SCOPE BOUNDARY") {
		t.Fatalf("missing appendix: %q", got)
	}
}
