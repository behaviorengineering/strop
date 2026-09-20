package humanreview

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPassthroughNormalizer_trimsWhitespace(t *testing.T) {
	t.Parallel()
	n := PassthroughNormalizer{}
	got, err := n.Normalize(context.Background(), "  too long  \n")
	require.NoError(t, err)
	assert.Equal(t, "too long", got)
}

func TestPassthroughNormalizer_emptyComment(t *testing.T) {
	t.Parallel()
	n := PassthroughNormalizer{}
	got, err := n.Normalize(context.Background(), "   ")
	require.NoError(t, err)
	assert.Equal(t, "", got)
}

func TestRegenOptionsFromComment_passthroughForceAndMessage(t *testing.T) {
	t.Parallel()
	opts, err := RegenOptionsFromComment(context.Background(), PassthroughNormalizer{}, "  rewrite title  ")
	require.NoError(t, err)
	assert.True(t, opts.Force)
	assert.Equal(t, "rewrite title", opts.Message)
	assert.Empty(t, opts.Context)
}

func TestRegenOptionsFromReview_setsMessageAndContext(t *testing.T) {
	t.Parallel()
	opts, err := RegenOptionsFromReview(context.Background(), nil, "  too stiff  ", "  ## Semantic translation\nHello\n")
	require.NoError(t, err)
	assert.True(t, opts.Force)
	assert.Equal(t, "too stiff", opts.Message)
	assert.Equal(t, "## Semantic translation\nHello", opts.Context)
}

func TestRegenOptionsFromComment_nilNormalizerUsesPassthrough(t *testing.T) {
	t.Parallel()
	opts, err := RegenOptionsFromComment(context.Background(), nil, "  keep speakers  ")
	require.NoError(t, err)
	assert.True(t, opts.Force)
	assert.Equal(t, "keep speakers", opts.Message)
}

func TestRegenOptionsFromComment_emptyCommentStillForces(t *testing.T) {
	t.Parallel()
	opts, err := RegenOptionsFromComment(context.Background(), nil, "")
	require.NoError(t, err)
	assert.True(t, opts.Force)
	assert.Equal(t, "", opts.Message)
}

type stubNormalizer struct {
	out string
	err error
}

func (s stubNormalizer) Normalize(_ context.Context, _ string) (string, error) {
	return s.out, s.err
}

func TestRegenOptionsFromComment_customNormalizerMessage(t *testing.T) {
	t.Parallel()
	opts, err := RegenOptionsFromComment(context.Background(), stubNormalizer{out: "structured"}, "raw")
	require.NoError(t, err)
	assert.True(t, opts.Force)
	assert.Equal(t, "structured", opts.Message)
}

func TestRegenOptionsFromComment_customNormalizerError(t *testing.T) {
	t.Parallel()
	want := fmt.Errorf("llm failed")
	_, err := RegenOptionsFromComment(context.Background(), stubNormalizer{err: want}, "raw")
	require.Error(t, err)
	assert.Equal(t, want, err)
}
