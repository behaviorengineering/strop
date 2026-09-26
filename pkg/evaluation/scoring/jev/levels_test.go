package jev

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDiscreteLevelCount_matchesRegistryMax(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 3, discreteLevelCount(2))
	assert.Equal(t, 2, discreteLevelCount(1))
	assert.Equal(t, 10, discreteLevelCount(10))
}
