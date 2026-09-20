// Package scopeboundary provides a shared appendix for evaluator prompts:
// IN DOMAIN / OUT OF DOMAIN plus a single overlap rule. Pipelines supply
// job-specific Boundary text; this package stays product-agnostic.
package scopeboundary

import "strings"

// Boundary documents in-domain versus out-of-domain feedback for one evaluator in one job context.
type Boundary struct {
	InDomain    string // What this evaluator should address in its checklist items.
	OutOfDomain string // What parallel evaluators own, or what lies outside the rubric.
}

// Format renders Boundary into a fixed English appendix. jobContext is a short job key
// (e.g. "translation", "topic", "post"). Returns empty if b is nil or both fields are empty after trim.
func Format(jobContext string, b *Boundary) string {
	if b == nil {
		return ""
	}
	in := strings.TrimSpace(b.InDomain)
	out := strings.TrimSpace(b.OutOfDomain)
	if in == "" && out == "" {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("EVALUATOR SCOPE BOUNDARY (job: ")
	sb.WriteString(jobContext)
	sb.WriteString(")\n")
	if in != "" {
		sb.WriteString("IN DOMAIN (your checklist items only):\n")
		sb.WriteString(in)
		sb.WriteString("\n\n")
	}
	if out != "" {
		sb.WriteString("OUT OF DOMAIN (parallel evaluators in this job own these; do not duplicate their topics):\n")
		sb.WriteString(out)
		sb.WriteString("\n\n")
	}
	sb.WriteString("OVERLAP RULE: For one quoted span, at most one role should lead the primary fix; stay inside IN DOMAIN above.")
	return sb.String()
}

// AppendToEvaluatorPrompt appends the formatted boundary after basePrompt when non-empty.
// If basePrompt is empty, returns only the appendix (or empty when boundary is empty).
func AppendToEvaluatorPrompt(basePrompt, jobContext string, b *Boundary) string {
	formatted := Format(jobContext, b)
	if formatted == "" {
		return strings.TrimSpace(basePrompt)
	}
	base := strings.TrimSpace(basePrompt)
	if base == "" {
		return formatted
	}
	return base + "\n\n" + formatted
}
