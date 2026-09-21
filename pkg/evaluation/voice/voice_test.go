package voice

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/behaviorengineering/strop/pkg/evaluation"
	"github.com/behaviorengineering/strop/pkg/evaluation/criteria"
)

func TestCriterionRegistered(t *testing.T) {
	t.Parallel()
	desc, err := criteria.DefaultRegistry().Get(criteria.CriterionIDVoiceFidelity)
	require.NoError(t, err)
	require.Equal(t, "Voice Fidelity", desc.Name)
	require.Contains(t, desc.Description, "voice profile")
	require.Contains(t, desc.Scoring, "0 points")
}

func TestCriterionIDs(t *testing.T) {
	t.Parallel()
	require.Equal(t, []criteria.CriterionID{criteria.CriterionIDVoiceFidelity}, CriterionIDs())
	require.Equal(t, evaluation.EvaluatorKey("voice_fidelity"), EvaluatorKey)
}

func TestPromptsMentionProfile(t *testing.T) {
	t.Parallel()
	p := Profile{
		BannedPatterns: []string{"Look,"},
		RequiredVerbs:  []string{"collapse"},
		ToneNotes:      "structural diagnosis",
	}
	fa := FeedbackAnalysisPrompt(p)
	require.Contains(t, fa, "voice_fidelity")
	require.Contains(t, fa, "Look,")
	require.Contains(t, fa, "collapse")
	require.Contains(t, fa, "structural diagnosis")
	sg := ScoreGenerationPrompt(p)
	require.Contains(t, sg, "voice_fidelity")
}

func TestHeuristicAuditPassesVariedProse(t *testing.T) {
	t.Parallel()
	prose := "A single sentence cannot carry an entire mind. When you speak, you collapse a dense landscape of memories, private associations, and immediate mood into a flat sequence of words. The listener rebuilds that meaning from a different history."
	result := HeuristicAudit(Profile{RequiredVerbs: []string{"collapse", "rebuild"}}, prose)
	require.True(t, result.Pass, "%+v", result.Violations)
	require.Empty(t, HeuristicFeedback(Profile{RequiredVerbs: []string{"collapse"}}, prose))
}

func TestHeuristicAuditFlagsStaccatoAndBannedPhrase(t *testing.T) {
	t.Parallel()
	prose := "Look, people hide the real fact today. Folks avoid the hard talk at night. We all just nod and stay quiet."
	p := Profile{BannedPatterns: []string{"Look,"}, MaxStaccatoRun: 2}
	result := HeuristicAudit(p, prose)
	require.False(t, result.Pass)
	require.Greater(t, result.LongestStaccatoRun, 2)
	fb := HeuristicFeedback(p, prose)
	require.Contains(t, fb, "voice_fidelity")
	require.Contains(t, fb, "banned_phrase")
	require.Contains(t, fb, "staccato_run")
}

func TestHeuristicAuditFlagsMissingVerb(t *testing.T) {
	t.Parallel()
	prose := "The shared record expires when nobody names the shift in the room between them today."
	result := HeuristicAudit(Profile{RequiredVerbs: []string{"collapse"}}, prose)
	require.False(t, result.Pass)
	require.Equal(t, 1, result.ParagraphsMissingVerbs)
}

func TestEmptyProsePasses(t *testing.T) {
	t.Parallel()
	require.True(t, HeuristicAudit(Profile{}, "").Pass)
	require.True(t, HeuristicAudit(Profile{}, "   ").Pass)
}
