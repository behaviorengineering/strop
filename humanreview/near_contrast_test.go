package humanreview

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelectNearAndContrast_empty(t *testing.T) {
	near, contrast := SelectNearAndContrast(nil)
	assert.Nil(t, near)
	assert.Nil(t, contrast)
}

func TestSelectNearAndContrast_oneHit(t *testing.T) {
	near, contrast := SelectNearAndContrast([]RetrievalHit{
		{ID: "1", DistinctiveMove: "pun in title"},
	})
	require.NotNil(t, near)
	assert.Equal(t, "1", near.ID)
	assert.Nil(t, contrast)
}

func TestSelectNearAndContrast_picksDifferentMove(t *testing.T) {
	near, contrast := SelectNearAndContrast([]RetrievalHit{
		{ID: "1", DistinctiveMove: "pun in title"},
		{ID: "2", DistinctiveMove: "pun in title"},
		{ID: "3", DistinctiveMove: "role title twist"},
	})
	require.NotNil(t, near)
	require.NotNil(t, contrast)
	assert.Equal(t, "1", near.ID)
	assert.Equal(t, "3", contrast.ID)
}

func TestSelectNearAndContrast_skipsEmptyMoveForContrast(t *testing.T) {
	near, contrast := SelectNearAndContrast([]RetrievalHit{
		{ID: "1", DistinctiveMove: "pun in title"},
		{ID: "2", DistinctiveMove: ""},
		{ID: "3", DistinctiveMove: "  "},
		{ID: "4", DistinctiveMove: "other move"},
	})
	require.NotNil(t, near)
	require.NotNil(t, contrast)
	assert.Equal(t, "4", contrast.ID)
}

func TestSelectNearAndContrast_caseInsensitiveSameMove(t *testing.T) {
	near, contrast := SelectNearAndContrast([]RetrievalHit{
		{ID: "1", DistinctiveMove: "Pun In Title"},
		{ID: "2", DistinctiveMove: "pun in title"},
	})
	require.NotNil(t, near)
	assert.Nil(t, contrast)
}
