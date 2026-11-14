package criteria

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShouldRankLeaves_skipsAtMax(t *testing.T) {
	t.Parallel()
	assert.False(t, ShouldRankLeaves(2.0, 2.0))
	assert.False(t, ShouldRankLeaves(2.0, 0))
	assert.True(t, ShouldRankLeaves(1.41, 2.0))
	assert.True(t, ShouldRankLeaves(1.999, 2.0))
	assert.True(t, ShouldRankLeaves(0, 2.0))
}

func TestRankLeaves_highestFirstThenID(t *testing.T) {
	t.Parallel()
	mods := ParentModifiers{
		Parent: "parent_quality",
		Leaves: []Modifier{
			{ID: "leaf_b", Name: "B"},
			{ID: "leaf_a", Name: "A"},
			{ID: "leaf_c", Name: "C"},
		},
	}
	got := RankLeaves(mods, map[string]float64{
		"leaf_b": 0.8,
		"leaf_a": 1.9,
		"leaf_c": 1.9,
	})
	require.Len(t, got, 3)
	assert.Equal(t, CriterionID("leaf_a"), got[0].ID)
	assert.Equal(t, CriterionID("leaf_c"), got[1].ID)
	assert.Equal(t, CriterionID("leaf_b"), got[2].ID)
}

func TestRankLeaves_missingScoreIsZero(t *testing.T) {
	t.Parallel()
	mods := ParentModifiers{
		Parent: "parent_quality",
		Leaves: []Modifier{{ID: "leaf_keep"}, {ID: "leaf_missing"}},
	}
	got := RankLeaves(mods, map[string]float64{"leaf_keep": 1.2})
	require.Len(t, got, 2)
	assert.Equal(t, CriterionID("leaf_keep"), got[0].ID)
	assert.Equal(t, CriterionID("leaf_missing"), got[1].ID)
	assert.InDelta(t, 0.0, got[1].Score, 1e-9)
}

func TestParentModifiers_LeafIDsSkipsEmpty(t *testing.T) {
	t.Parallel()
	mods := ParentModifiers{
		Parent: "parent_quality",
		Leaves: []Modifier{{ID: "leaf_keep"}, {ID: ""}},
	}
	assert.Equal(t, []CriterionID{"leaf_keep"}, mods.LeafIDs())
	assert.Empty(t, ParentModifiers{}.LeafIDs())
}
