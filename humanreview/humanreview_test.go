package humanreview

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/behaviorengineering/strop/evaluation/criteria"
)

func TestJobStepRegistry_RegisterAndLookup(t *testing.T) {
	t.Parallel()
	reg := NewJobStepRegistry()
	reg.Register("my_job", "my_step")
	step, err := reg.StepForJob("my_job")
	require.NoError(t, err)
	assert.Equal(t, Step("my_step"), step)

	_, err = reg.StepForJob("missing")
	require.Error(t, err)
}

func TestComputeCriterionFromVersionScores_metLater(t *testing.T) {
	t.Parallel()
	scores := []map[string]float64{
		{"clarity": 0},
		{"clarity": 1},
		{"clarity": 2},
	}
	got := ComputeCriterionFromVersionScores("clarity", 2.0, scores)
	assert.Equal(t, 2.0, got.ProposedScore)
	require.NotNil(t, got.FirstVersionAtMax)
	assert.Equal(t, 3, *got.FirstVersionAtMax)
	assert.Equal(t, 2, got.VersionsWithImprovement)
	assert.False(t, got.SmellPassedAtV1)
}

func TestDeduplicateCriterionEvaluationsByName(t *testing.T) {
	t.Parallel()
	evals := []CriterionScoreEvaluation{
		{CriterionName: "A", Status: CriterionScoreStatusProposed},
		{CriterionName: "B", Status: CriterionScoreStatusProposed},
		{CriterionName: "A", Status: CriterionScoreStatusProposed},
	}
	deduped, changed := DeduplicateCriterionEvaluationsByName(evals)
	assert.True(t, changed)
	require.Len(t, deduped, 2)
}

func TestGetCriterionDescriptions_deduplicatesInputIDs(t *testing.T) {
	t.Parallel()
	ids := []criteria.CriterionID{
		criteria.CriterionIDInstructionCompliance,
		criteria.CriterionIDInstructionCompliance,
		criteria.CriterionIDCompleteness,
	}
	descriptions := GetCriterionDescriptions(ids)
	require.Len(t, descriptions, 2)
	assert.Equal(t, "Instruction Compliance", descriptions[0].Name)
	assert.Equal(t, "Completeness", descriptions[1].Name)
}

func TestMapHistoryBuilderFactory(t *testing.T) {
	t.Parallel()
	f := NewMapHistoryBuilderFactory()
	_, err := f.GetBuilder("j")
	require.Error(t, err)
}
