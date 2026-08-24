package humanreview_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	kithr "github.com/behaviorengineering/strop/humanreview"
)

func TestSelectNearAndContrast_Unchanged(t *testing.T) {
	hits := []kithr.RetrievalHit{
		{ID: "n", DistinctiveMove: "a"},
		{ID: "c", DistinctiveMove: "b"},
	}
	near, contrast := kithr.SelectNearAndContrast(hits)
	require.NotNil(t, near)
	require.NotNil(t, contrast)
	assert.Equal(t, "n", near.ID)
	assert.Equal(t, "c", contrast.ID)
}
