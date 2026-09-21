package voice

import (
	"strings"

	"github.com/behaviorengineering/strop/pkg/evaluation/criteria"
)

// FeedbackAnalysisPrompt is the chained feedback prompt for the voice role.
// Phrase banks come from the profile. An empty profile still names staccato runs.
func FeedbackAnalysisPrompt(p Profile) string {
	var b strings.Builder
	b.WriteString("Evaluate VOICE FIDELITY only on the owned reader-facing prose.\n\n")
	b.WriteString("Score only: voice_fidelity.\n")
	b.WriteString("Do NOT score process, meta-discourse, show-then-tell, contrastive negation, phantom critics, or paradox rescue.\n\n")
	b.WriteString("Fail when:\n")
	b.WriteString("- A run of similar short sentences is longer than the profile limit (default: more than 2).\n")
	if len(p.BannedPatterns) > 0 {
		b.WriteString("- The prose uses a banned phrase from this profile: ")
		b.WriteString(strings.Join(p.BannedPatterns, "; "))
		b.WriteString(".\n")
	} else {
		b.WriteString("- The prose uses a banned phrase when the caller supplied a phrase list. With no list, do not invent bans.\n")
	}
	if len(p.RequiredVerbs) > 0 {
		b.WriteString("- A body paragraph contains none of the required verb classes: ")
		b.WriteString(strings.Join(p.RequiredVerbs, ", "))
		b.WriteString(".\n")
	}
	if notes := strings.TrimSpace(p.ToneNotes); notes != "" {
		b.WriteString("- The tone misses this note: ")
		b.WriteString(notes)
		b.WriteString(".\n")
	}
	b.WriteString("\nQuote the offending span in feedback.\n\n")
	b.WriteString("Provide checklist-based feedback. Always output at least one line in the feedback field\n")
	b.WriteString("(use [✓] / [ ] lines). Never leave feedback empty. Scores must be plain decimals.\n")
	return b.String()
}

// ScoreGenerationPrompt builds the score prompt for the voice role.
func ScoreGenerationPrompt(p Profile) string {
	base := criteria.NewEvaluatorScorePromptBuilder("Voice fidelity", CriterionIDs()).Build()
	return base + "\n" + FeedbackAnalysisPrompt(p)
}
