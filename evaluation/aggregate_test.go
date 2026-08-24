package evaluation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAggregateLabeledEvals_empty(t *testing.T) {
	t.Parallel()
	got := AggregateLabeledEvals(nil, nil, 7.0)
	require.NotNil(t, got)
	assert.Equal(t, 7.0, got.WeightedScore)
	assert.Empty(t, got.CriterionScores)
}

func TestAggregateLabeledEvals_meansAndJoins(t *testing.T) {
	t.Parallel()
	got := AggregateLabeledEvals([]float64{8, 6}, []LabeledEval{
		{
			Label: "teaser",
			Eval: &AggregatedEvaluation{
				WeightedScore:        8,
				CriterionScores:      map[string]float64{"clarity": 9, "voice": 7},
				ConsolidatedFeedback: "tighten hook",
				AgentScores:          map[string]float64{"a": 8},
				AgentFeedback:        map[string]string{"a": "ok"},
				AgentRationale:       map[string]string{"a": "r1"},
				EvaluationTime:       time.Second,
			},
		},
		{
			Label: "tldr",
			Eval: &AggregatedEvaluation{
				WeightedScore:        6,
				CriterionScores:      map[string]float64{"clarity": 5, "flow": 6},
				ConsolidatedFeedback: "cut fluff",
				AgentScores:          map[string]float64{"a": 6},
				AgentFeedback:        map[string]string{"a": "more"},
				AgentRationale:       map[string]string{"a": "r2"},
				EvaluationTime:       2 * time.Second,
			},
		},
	}, 0)
	require.NotNil(t, got)
	assert.Equal(t, 7.0, got.WeightedScore)
	assert.Equal(t, 7.0, got.CriterionScores["clarity"])
	assert.Equal(t, 7.0, got.CriterionScores["voice"])
	assert.Equal(t, 6.0, got.CriterionScores["flow"])
	assert.Equal(t, 7.0, got.AgentScores["a"])
	assert.Equal(t, "ok\nmore", got.AgentFeedback["a"])
	assert.Equal(t, "r1\nr2", got.AgentRationale["a"])
	assert.Contains(t, got.ConsolidatedFeedback, "teaser: tighten hook")
	assert.Contains(t, got.ConsolidatedFeedback, "tldr: cut fluff")
	assert.Equal(t, 3*time.Second, got.EvaluationTime)
}
