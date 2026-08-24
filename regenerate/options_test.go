package regenerate

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFromContext_includesContext(t *testing.T) {
	t.Parallel()
	opts := RegenerateOptions{Force: true, Message: "too stiff", Context: "edited proposal"}
	got := FromContext(WithOptions(context.Background(), opts))
	assert.True(t, got.Force)
	assert.Equal(t, "too stiff", got.Message)
	assert.Equal(t, "edited proposal", got.Context)
}

func TestFromContext_zeroWhenUnset(t *testing.T) {
	t.Parallel()
	got := FromContext(context.Background())
	assert.False(t, got.Force)
	assert.Empty(t, got.Message)
	assert.Empty(t, got.Context)
	assert.False(t, got.Research)
}

func TestResearchModeFromContext(t *testing.T) {
	t.Parallel()
	assert.False(t, ResearchModeFromContext(context.Background()))
	assert.False(t, ResearchModeFromContext(nil))
	assert.True(t, ResearchModeFromContext(WithResearchMode(context.Background(), true)))
	assert.False(t, ResearchModeFromContext(WithResearchMode(context.Background(), false)))
}
