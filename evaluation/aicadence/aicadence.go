// Package aicadence is the portable reader-prose AI-cadence judge for strop consumers.
//
// Attach it on human-facing explanatory prose jobs (essay composition/polish, claims
// thoughts, video TL;DW). Do NOT attach on vernacular-aphorism jobs (sayings fluff)
// or structural extractors (chapters/speakers). Prefer a cheap model via roleProviders.
package aicadence

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/behaviorengineering/strop/evaluation"
	"github.com/behaviorengineering/strop/evaluation/criteria"
)

// EvaluatorKey is the parallel evaluation role for the cadence judge.
const EvaluatorKey evaluation.EvaluatorKey = "ai_cadence"

// CriterionIDs returns the criterion set owned by the cadence role.
func CriterionIDs() []criteria.CriterionID {
	return []criteria.CriterionID{criteria.CriterionIDNoAphorismStack}
}

// FeedbackAnalysisPrompt is the chained FA prompt for the cadence role.
func FeedbackAnalysisPrompt() string {
	return `Evaluate AI CADENCE only on the owned reader-facing prose.

Score only: no_aphorism_stack.
Do NOT score process, meta-discourse, show-then-tell, contrastive negation, phantom critics, or paradox rescue.

Fail when:
- Parallel Loud/Quiet (or noise/silence, visible/invisible) couplets that punch antithesis without a new fact.
- Semicolon aphorisms ("Stress drains; purpose concentrates.").
- Metaphor-shell restacks that only relabel the prior claim (map/page, storm/filter, bill-to-pay punches) with no new scene or mechanism.

Prefer scene + fact. Quote the offending span in feedback.

Provide checklist-based feedback. Always output at least one line in the feedback field
(use [✓] / [ ] lines). Never leave feedback empty. Scores must be plain decimals.
`
}

// ScoreGenerationPrompt is built by the consumer via criteria.NewEvaluatorScorePromptBuilder
// when CriterionIDs are wired. This fallback names the single criterion for copy-paste jobs.
func ScoreGenerationPrompt() string {
	return criteria.NewEvaluatorScorePromptBuilder("AI cadence (no aphorism stack)", CriterionIDs()).Build()
}

// HeuristicFeedback returns checklist lines when portable regex hits fire.
// Empty string means no hit. Product phrase banks may wrap this and add more hits.
func HeuristicFeedback(prose string) string {
	hits := HeuristicHits(prose)
	if len(hits) == 0 {
		return ""
	}
	lines := make([]string, 0, len(hits)+1)
	lines = append(lines, "[ ] no_aphorism_stack: replace couplets and metaphor restacks with plain scene or fact")
	for _, h := range hits {
		lines = append(lines, fmt.Sprintf("[ ] aphorism hit: %s", h))
	}
	return strings.Join(lines, "\n")
}

// HeuristicHits returns portable cadence labels (no product-specific phrases).
func HeuristicHits(prose string) []string {
	text := strings.TrimSpace(prose)
	if text == "" {
		return nil
	}
	var hits []string
	seen := map[string]bool{}
	add := func(label string) {
		if seen[label] {
			return
		}
		seen[label] = true
		hits = append(hits, label)
	}
	if semicolonAphorism.MatchString(text) {
		add("semicolon aphorism")
	}
	if parallelContrastCouplet.MatchString(text) {
		add("parallel contrast couplet")
	}
	return hits
}

// Short antithetical clauses joined by semicolon: "Stress drains; purpose concentrates."
var semicolonAphorism = regexp.MustCompile(`(?i)\b([A-Za-z][\w'-]{0,24}(?:\s+[A-Za-z][\w'-]{0,24}){0,4})\s*;\s*([A-Za-z][\w'-]{0,24}(?:\s+[A-Za-z][\w'-]{0,24}){0,4})\.`)

// Adjacent short sentences that open with antonym bookends.
var parallelContrastCouplet = regexp.MustCompile(`(?i)(?:^|[.!?]\s+)(loud|quiet|noise|silence|visible|invisible|overt|silent)\b[^.!?]{0,80}[.!?]\s+(loud|quiet|noise|silence|visible|invisible|overt|silent)\b`)
