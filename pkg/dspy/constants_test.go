package dspy

import (
	"strings"
	"testing"
)

func TestRationaleDescriptionWithContext(t *testing.T) {
	s := RationaleDescriptionWithContext("  chapter splits  ")
	if !strings.Contains(s, "chapter splits") {
		t.Fatalf("expected focus in description: %q", s)
	}
	if !strings.Contains(s, "VOICE:") || !strings.Contains(s, "MUST:") || !strings.Contains(s, "ANTI_PATTERN:") {
		t.Fatalf("expected objective recitation labels: %q", s)
	}
	if !strings.Contains(s, "action-chain") {
		t.Fatalf("expected action-chain guidance: %q", s)
	}
	if !strings.Contains(s, "introspective") {
		t.Fatalf("expected anti-essay guidance: %q", s)
	}
}

func TestEvaluatorRationaleDescriptionWithContext(t *testing.T) {
	s := EvaluatorRationaleDescriptionWithContext("style checks")
	if !strings.Contains(s, "style checks") {
		t.Fatalf("expected focus in description: %q", s)
	}
	if strings.Contains(s, "VOICE:") || strings.Contains(s, "ANTI_PATTERN:") {
		t.Fatalf("evaluator rationale must not require objective recitation: %q", s)
	}
	if !strings.Contains(s, "angle brackets") {
		t.Fatalf("expected no-angle-bracket rule: %q", s)
	}
	if !strings.Contains(s, "3–5 lines") {
		t.Fatalf("expected line cap: %q", s)
	}
}

func TestRationaleDescriptionWithContext_EmptyFocus(t *testing.T) {
	s := RationaleDescriptionWithContext("")
	if !strings.Contains(s, "structured output") {
		t.Fatalf("expected default focus phrase: %q", s)
	}
}

func TestRationaleDescriptionWithExtra(t *testing.T) {
	s := RationaleDescriptionWithExtra("Boundaries", "After </chapters> only.")
	if !strings.Contains(s, "Boundaries") || !strings.Contains(s, "</chapters>") {
		t.Fatalf("expected base + extra: %q", s)
	}
	if RationaleDescriptionWithExtra("x", "  ") != RationaleDescriptionWithContext("x") {
		t.Fatal("whitespace-only extra should equal base")
	}
}
