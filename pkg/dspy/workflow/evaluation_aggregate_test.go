package workflow

import (
	"testing"

	"github.com/behaviorengineering/strop/pkg/evaluation"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type weightRoleInfo struct {
	weights map[evaluation.EvaluatorKey]float64
}

func (w weightRoleInfo) EvaluatorName(key evaluation.EvaluatorKey) string { return string(key) }
func (w weightRoleInfo) HasEvaluator(key evaluation.EvaluatorKey) bool {
	_, ok := w.weights[key]
	return ok
}
func (w weightRoleInfo) EvaluatorWeight(key evaluation.EvaluatorKey) float64 {
	if v, ok := w.weights[key]; ok {
		return v
	}
	return 1
}
func (w weightRoleInfo) ConsolidatorKey() evaluation.ConsolidatorKey { return "consolidator" }
func (w weightRoleInfo) ConsolidatorName() string                    { return "Consolidator" }

func TestCalculateWeightedScore_equalWeights(t *testing.T) {
	w := &ParallelEvaluationWorkflow{
		roleInfo: weightRoleInfo{weights: map[evaluation.EvaluatorKey]float64{
			"a": 1, "b": 1,
		}},
	}
	score, agents, err := w.calculateWeightedScore([]*evaluation.IndividualEvaluation{
		{AgentID: "a", AgentName: "A", Score: 0.8},
		{AgentID: "b", AgentName: "B", Score: 0.4},
	})
	require.NoError(t, err)
	assert.InDelta(t, 0.6, score, 0.001)
	assert.Equal(t, 0.8, agents["A"])
	assert.Equal(t, 0.4, agents["B"])
}

func TestCalculateWeightedScore_respectsRoleWeights(t *testing.T) {
	w := &ParallelEvaluationWorkflow{
		roleInfo: weightRoleInfo{weights: map[evaluation.EvaluatorKey]float64{
			"a": 3, "b": 1,
		}},
	}
	score, _, err := w.calculateWeightedScore([]*evaluation.IndividualEvaluation{
		{AgentID: "a", AgentName: "A", Score: 1.0},
		{AgentID: "b", AgentName: "B", Score: 0.0},
	})
	require.NoError(t, err)
	assert.InDelta(t, 0.75, score, 0.001)
}
