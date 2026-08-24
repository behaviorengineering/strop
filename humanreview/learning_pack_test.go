package humanreview

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLearningPackRegistry_RegisterThenGet(t *testing.T) {
	t.Parallel()
	reg := NewLearningPackRegistry()
	pack := NewStaticLearningPack("demo", "compose_job")
	reg.Register(pack)

	got, err := reg.Get("demo")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, PipelineType("demo"), got.PipelineType())
	assert.True(t, got.IsCompositionJob("compose_job"))
	assert.False(t, got.IsCompositionJob("other_job"))
	assert.Equal(t, []Job{"compose_job"}, got.CompositionJobs())
}

func TestLearningPackRegistry_GetMissingIsExplicit(t *testing.T) {
	t.Parallel()
	reg := NewLearningPackRegistry()
	_, err := reg.Get("missing")
	require.Error(t, err)
	assert.Equal(t, ErrUnknownLearningPack("missing").Error(), err.Error())
}

func TestLearningPackRegistry_NilRegisterIgnored(t *testing.T) {
	t.Parallel()
	reg := NewLearningPackRegistry()
	reg.Register(nil)
	_, err := reg.Get("demo")
	require.Error(t, err)
}

func TestLearningPackRegistry_NilReceiverGet(t *testing.T) {
	t.Parallel()
	var reg *LearningPackRegistry
	_, err := reg.Get("demo")
	require.Error(t, err)
}
