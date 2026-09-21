package voice

import (
	"strconv"
	"strings"

	"github.com/behaviorengineering/strop/pkg/evaluation/criteria"
)

// FeedbackAnalysisPrompt is the chained feedback prompt for the voice role.
// The registered workflow reads generator_input.voice_profile at evaluation time.
// A caller-supplied profile is appended only as a fallback when that JSON field is empty.
func FeedbackAnalysisPrompt(p Profile) string {
	return DynamicFeedbackAnalysisPrompt() + bakedProfileFallback(p)
}

// DynamicFeedbackAnalysisPrompt tells the voice role to read the per-piece profile
// from generator_input and to stay neutral when that field is absent.
func DynamicFeedbackAnalysisPrompt() string {
	var b strings.Builder
	b.WriteString("Evaluate VOICE FIDELITY only on the owned reader-facing prose.\n\n")
	b.WriteString("Score only: voice_fidelity.\n")
	b.WriteString("Do NOT score process, meta-discourse, show-then-tell, contrastive negation, phantom critics, or paradox rescue.\n\n")
	b.WriteString("Read generator_input.voice_profile when it is present.\n")
	b.WriteString("It is a JSON object with id, label, banned_patterns, required_verb_classes, max_staccato_run, and tone_notes.\n")
	b.WriteString("Apply only the rules in that object.\n\n")
	b.WriteString("When voice_profile is empty or absent:\n")
	b.WriteString("- Do not invent bans, required verbs, tone notes, or a house style.\n")
	b.WriteString("- Pass voice_fidelity unless a run of similar short sentences is longer than 2.\n\n")
	b.WriteString("When voice_profile is present, fail when:\n")
	b.WriteString("- The prose uses a phrase listed in banned_patterns.\n")
	b.WriteString("- A body paragraph contains none of required_verb_classes, when that list is non-empty.\n")
	b.WriteString("- A run of similar short sentences is longer than max_staccato_run (use 2 when the field is missing or 0).\n")
	b.WriteString("- tone_notes is non-empty and the prose misses that tone.\n")
	b.WriteString("Do not add rules that are not in the profile.\n\n")
	b.WriteString("Quote the offending span in feedback.\n\n")
	b.WriteString("Provide checklist-based feedback. Always output at least one line in the feedback field\n")
	b.WriteString("(use [✓] / [ ] lines). Never leave feedback empty. Scores must be plain decimals.\n")
	return b.String()
}

func bakedProfileFallback(p Profile) string {
	if len(p.BannedPatterns) == 0 && len(p.RequiredVerbs) == 0 && strings.TrimSpace(p.ToneNotes) == "" && p.MaxStaccatoRun == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\nIf generator_input.voice_profile is empty, use this caller profile instead:\n")
	if len(p.BannedPatterns) > 0 {
		b.WriteString("- banned phrases: ")
		b.WriteString(strings.Join(p.BannedPatterns, "; "))
		b.WriteString(".\n")
	}
	if len(p.RequiredVerbs) > 0 {
		b.WriteString("- required verb classes: ")
		b.WriteString(strings.Join(p.RequiredVerbs, ", "))
		b.WriteString(".\n")
	}
	if p.MaxStaccatoRun > 0 {
		b.WriteString("- max staccato run: ")
		b.WriteString(strconv.Itoa(p.MaxStaccatoRun))
		b.WriteString(".\n")
	}
	if notes := strings.TrimSpace(p.ToneNotes); notes != "" {
		b.WriteString("- tone notes: ")
		b.WriteString(notes)
		b.WriteString(".\n")
	}
	return b.String()
}

// ScoreGenerationPrompt builds the score prompt for the voice role.
func ScoreGenerationPrompt(p Profile) string {
	base := criteria.NewEvaluatorScorePromptBuilder("Voice fidelity", CriterionIDs()).Build()
	return base + "\n" + FeedbackAnalysisPrompt(p)
}
