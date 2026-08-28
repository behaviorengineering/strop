// Package skimmarkup provides portable helpers for LLM skim emphasis markup.
// Allowed changes: wrap existing spans in **bold** or *italic* only; fidelity checks
// require stripped output to match source words after whitespace normalization.
package skimmarkup

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	boldRe   = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	italicRe = regexp.MustCompile(`\*([^*]+)\*`)
)

// Strip removes **bold** and *italic* markers, leaving plain words.
func Strip(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	s = boldRe.ReplaceAllString(s, "$1")
	s = italicRe.ReplaceAllString(s, "$1")
	return strings.TrimSpace(s)
}

// NormalizePlain strips markup and collapses whitespace for fidelity comparison.
func NormalizePlain(s string) string {
	return strings.Join(strings.Fields(Strip(s)), " ")
}

// FidelityOK reports whether marked text matches source after strip + normalize.
func FidelityOK(source, marked string) bool {
	want := NormalizePlain(source)
	got := NormalizePlain(marked)
	if want == "" {
		return false
	}
	return got == want
}

// ValidateFidelity returns an error when marked text rewrites source words.
func ValidateFidelity(source, marked string) error {
	if strings.TrimSpace(marked) == "" {
		return fmt.Errorf("skimmarkup: empty marked text")
	}
	if strings.TrimSpace(source) == "" {
		return fmt.Errorf("skimmarkup: empty source text")
	}
	if !FidelityOK(source, marked) {
		return fmt.Errorf("skimmarkup: fidelity failed (stripped output must match input words)")
	}
	return nil
}
