package aicadence

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/behaviorengineering/strop/evaluation"
	"github.com/behaviorengineering/strop/evaluation/criteria"
)

func TestCriterionRegistered(t *testing.T) {
	t.Parallel()
	desc, err := criteria.DefaultRegistry().Get(criteria.CriterionIDNoAphorismStack)
	require.NoError(t, err)
	require.Equal(t, "No Aphorism Stack", desc.Name)
	require.Contains(t, desc.Description, "parallel Loud/Quiet")
	require.Contains(t, desc.Examples, "Stress drains")
}

func TestCriterionIDs(t *testing.T) {
	t.Parallel()
	ids := CriterionIDs()
	require.Equal(t, []criteria.CriterionID{criteria.CriterionIDNoAphorismStack}, ids)
}

func TestPromptsMentionCadencePatterns(t *testing.T) {
	t.Parallel()
	require.Equal(t, evaluation.EvaluatorKey("ai_cadence"), EvaluatorKey)
	fa := FeedbackAnalysisPrompt()
	require.Contains(t, fa, "no_aphorism_stack")
	require.Contains(t, fa, "couplet")
	require.Contains(t, fa, "restack")
	sg := ScoreGenerationPrompt()
	require.Contains(t, sg, "no_aphorism_stack")
}

func TestHeuristicFeedback(t *testing.T) {
	t.Parallel()
	require.Empty(t, HeuristicFeedback(""))
	require.Empty(t, HeuristicFeedback("She keeps her voice small and gets called shy. Clinics rarely refer that presentation."))
	fb := HeuristicFeedback("Loud shows up on the checklist. Quiet looks like personality.")
	require.Contains(t, fb, "no_aphorism_stack")
	require.Contains(t, fb, "parallel contrast couplet")
	fb2 := HeuristicFeedback("Stress drains; purpose concentrates.")
	require.Contains(t, fb2, "semicolon aphorism")
}
