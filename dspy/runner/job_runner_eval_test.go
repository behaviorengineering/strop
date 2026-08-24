package runner

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	kitdspy "github.com/behaviorengineering/strop/dspy"
)

type stubEvaluationInput struct {
	eval map[string]interface{}
	ver  int
}

func (s stubEvaluationInput) EvaluationMap() map[string]interface{} {
	return s.eval
}

func (s stubEvaluationInput) GetVersion() int {
	return s.ver
}

func TestBuildEvaluationInputs_usesEvaluationMapAndVersion(t *testing.T) {
	t.Parallel()
	in := stubEvaluationInput{
		eval: map[string]interface{}{
			"original_text": "decir",
			"version":       9,
		},
		ver: 2,
	}
	out := map[string]interface{}{"literal_translation": "to say"}

	got := buildEvaluationInputs(in, out)

	require.Equal(t, in.eval, got[kitdspy.FieldGeneratorInput])
	require.Equal(t, out, got[kitdspy.FieldGeneratorOutput])
	assert.Equal(t, 2, got[kitdspy.FieldIterationVersion], "iterationVersion comes from GetVersion, not the inner map")
}
