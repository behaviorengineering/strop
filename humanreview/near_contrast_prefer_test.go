package humanreview_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	strophr "github.com/behaviorengineering/strop/humanreview"
)

func TestSelectNearAndContrast_Unchanged(t *testing.T) {
	hits := []strophr.RetrievalHit{
		{ID: "n", DistinctiveMove: "a"},
		{ID: "c", DistinctiveMove: "b"},
	}
	near, contrast := strophr.SelectNearAndContrast(hits)
	require.NotNil(t, near)
	require.NotNil(t, contrast)
	assert.Equal(t, "n", near.ID)
	assert.Equal(t, "c", contrast.ID)
}
