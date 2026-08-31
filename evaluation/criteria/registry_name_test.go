package criteria

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCriterionRegistry_GetByName(t *testing.T) {
	reg := NewCriterionRegistry()

	got, err := reg.GetByName("Instruction Compliance")
	require.NoError(t, err)
	require.Equal(t, CriterionIDInstructionCompliance, got.ID)

	_, err = reg.GetByName("Not A Real Criterion")
	require.Error(t, err)
}

func TestCriterionRegistry_GetByName_afterRegister(t *testing.T) {
	reg := NewCriterionRegistry()
	reg.Register(CriterionDescription{
		ID:        CriterionIDInsightNovelty,
		Name:      "Insight Novelty",
		MaxPoints: 2.0,
		Category:  CriterionCategoryOutputQuality,
	})

	got, err := reg.GetByName("Insight Novelty")
	require.NoError(t, err)
	require.Equal(t, CriterionIDInsightNovelty, got.ID)
}
