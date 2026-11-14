package criteria

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFloorAnchor_keepsPartialScoreOnLowerBand(t *testing.T) {
	t.Parallel()
	assert.InDelta(t, 1.0, FloorAnchor(1.41, 2), 1e-9)
	assert.InDelta(t, 1.0, FloorAnchor(1.90, 2), 1e-9)
	assert.InDelta(t, 2.0, FloorAnchor(2.00, 2), 1e-9)
	assert.InDelta(t, 0.0, FloorAnchor(0.40, 2), 1e-9)
}

func TestScoringBandText_onePointLine(t *testing.T) {
	t.Parallel()
	crit, err := DefaultRegistry().Get(CriterionIDInstructionCompliance)
	require.NoError(t, err)
	got := ScoringBandText(crit.Scoring, 1)
	assert.Contains(t, strings.ToLower(got), "most instructions followed")
	assert.NotContains(t, got, "2 points:")
	assert.NotContains(t, got, "0 points:")
}

func TestFormatScoreEvidence_lowestScoreFirst(t *testing.T) {
	t.Parallel()
	got := FormatScoreEvidence(DefaultRegistry(), map[string]float64{
		string(CriterionIDCompleteness):          1.80,
		string(CriterionIDInstructionCompliance): 1.41,
	})
	idxLow := strings.Index(got, string(CriterionIDInstructionCompliance))
	idxHigh := strings.Index(got, string(CriterionIDCompleteness))
	require.Greater(t, idxLow, 0)
	require.Greater(t, idxHigh, idxLow)
	assert.Contains(t, got, "1.41 / 2.00 -> 1-point band")
	assert.Contains(t, strings.ToLower(got), "most instructions followed")
}
