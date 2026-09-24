package workflow

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCriterionScores_rejectsInvalidNumeric(t *testing.T) {
	expected := map[string]struct{}{"clarity": {}}
	_, err := parseCriterionScores(map[string]interface{}{"clarity": "not-a-number"}, expected)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "clarity")
}

func TestParseCriterionScores_requiresConfiguredKeys(t *testing.T) {
	expected := map[string]struct{}{"clarity": {}, "depth": {}}
	scores, err := parseCriterionScores(map[string]interface{}{"clarity": 0.8, "extra": 0.1}, expected)
	require.NoError(t, err)
	assert.Equal(t, 0.8, scores["clarity"])
	_, ok := scores["extra"]
	assert.False(t, ok)
}
