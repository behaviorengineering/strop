package workflow

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	stropdspy "github.com/behaviorengineering/strop/pkg/dspy"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvaluatorReplayFixture_parseCriterionScores(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	path := filepath.Join(filepath.Dir(file), "..", "testdata", "module-replay", "evaluator_span.json")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var span struct {
		RecordedOutputs map[string]any `json:"recorded_outputs"`
	}
	require.NoError(t, json.Unmarshal(data, &span))
	raw := span.RecordedOutputs[stropdspy.FieldCriterionScores]
	expected := map[string]struct{}{"clarity": {}, "grounding": {}}
	scores, err := parseCriterionScores(raw, expected)
	require.NoError(t, err)
	assert.InDelta(t, 1.5, scores["clarity"], 0.001)
	assert.InDelta(t, 1.0, scores["grounding"], 0.001)
}
