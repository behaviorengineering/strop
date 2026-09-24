package dspy_test

import (
	"path/filepath"
	"runtime"
	"testing"

	stropdspy "github.com/behaviorengineering/strop/pkg/dspy"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func moduleReplayDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Join(filepath.Dir(file), "testdata", "module-replay")
}

func TestModuleReplayFixturesOffline_generator(t *testing.T) {
	span, err := stropdspy.LoadModuleReplaySpan(filepath.Join(moduleReplayDir(t), "generator_span.json"))
	require.NoError(t, err)
	require.NotEmpty(t, span.Fields)
	require.NotEmpty(t, span.RecordedOutputs)
	assert.Contains(t, span.RecordedOutputs, "summary")
	assert.Contains(t, span.Fields, stropdspy.FieldOriginalText)
}

func TestModuleReplayFixturesOffline_evaluatorHasCriterionScores(t *testing.T) {
	span, err := stropdspy.LoadModuleReplaySpan(filepath.Join(moduleReplayDir(t), "evaluator_span.json"))
	require.NoError(t, err)
	raw, ok := span.RecordedOutputs[stropdspy.FieldCriterionScores]
	require.True(t, ok)
	scores, ok := raw.(map[string]any)
	require.True(t, ok)
	assert.EqualValues(t, 1.5, scores["clarity"])
	assert.EqualValues(t, 1.0, scores["grounding"])
}
