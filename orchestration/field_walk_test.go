package orchestration

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/behaviorengineering/strop/evaluation"
	"github.com/behaviorengineering/strop/streaming"
)

type fakeFieldRunner struct {
	t            *testing.T
	calls        []FieldPhaseRequest
	failFirst    map[string]bool
	seenAttempts map[string]int
}

func (f *fakeFieldRunner) Run(_ context.Context, req FieldPhaseRequest, _ streaming.EventChannel) (*FieldPhaseResponse, error) {
	f.calls = append(f.calls, req)
	if f.seenAttempts == nil {
		f.seenAttempts = map[string]int{}
	}
	f.seenAttempts[req.FieldID]++
	attempt := f.seenAttempts[req.FieldID]

	if req.FieldID == "b" {
		_, hasC := req.LockedOutput["c"]
		assert.False(f.t, hasC, "upcoming field must not be locked")
	}

	score := 8.0
	out := "out-" + req.FieldID
	fb := "ok-" + req.FieldID
	if f.failFirst[req.FieldID] && attempt == 1 {
		score = 3.0
		out = "weak-" + req.FieldID
		fb = "retry-" + req.FieldID
	}
	if attempt > 1 {
		assert.Equal(f.t, "weak-"+req.FieldID, req.PreviousFailedOutput)
		assert.Contains(f.t, req.PreviousFeedback, "retry-")
	}

	return &FieldPhaseResponse{
		OutputText: out,
		Rationale:  "r-" + req.FieldID,
		Eval: &evaluation.AggregatedEvaluation{
			WeightedScore:        score,
			CriterionScores:      map[string]float64{"clarity": score},
			ConsolidatedFeedback: fb,
		},
	}, nil
}

func testPhases() []PhaseDef {
	return []PhaseDef{
		{ID: "a", DisplayName: "a", MaxAttempts: 2},
		{ID: "b", DisplayName: "b", MaxAttempts: 2},
		{ID: "c", DisplayName: "c", MaxAttempts: 2},
	}
}

func TestFieldWalkStrategy_emptyThenAggregate(t *testing.T) {
	t.Parallel()
	runner := &fakeFieldRunner{t: t, failFirst: map[string]bool{"b": true}}
	strat := NewFieldWalkStrategy(FieldWalkConfig{
		Phases:          testPhases(),
		MinPassScore:    7.0,
		Runner:          runner,
		EmptyResultErr:  fmt.Errorf("empty"),
		Version:         1,
		VersionFeedback: "outer-feedback",
		Seed:            map[string]string{"b": "src-b", "c": "src-c"},
	})
	result, err := RunCompositionLoop(context.Background(), strat, nil)
	require.NoError(t, err)
	require.NotNil(t, result)

	st, ok := result.OutputState.(*FieldWalkState)
	require.True(t, ok)
	assert.Equal(t, "", st.Draft["a"])
	assert.Equal(t, "out-b", st.Draft["b"])

	eval, ok := result.EvalPayload.(*evaluation.AggregatedEvaluation)
	require.True(t, ok)
	require.NotNil(t, eval)
	assert.Len(t, st.Scores, 2)
	assert.Equal(t, result.Score, eval.WeightedScore)
	assert.Contains(t, eval.ConsolidatedFeedback, "b:")

	require.NotEmpty(t, runner.calls)
	assert.Equal(t, "outer-feedback", runner.calls[0].PreviousFeedback)
	assert.Equal(t, "b", runner.calls[0].FieldID)
}

func TestFieldWalkStrategy_sourceTextFunc(t *testing.T) {
	t.Parallel()
	source := map[string]string{"a": "EN-a", "b": "EN-b", "c": "EN-c"}
	runner := &fakeFieldRunner{t: t}
	strat := NewFieldWalkStrategy(FieldWalkConfig{
		Phases:         testPhases(),
		MinPassScore:   7.0,
		EmptyResultErr: fmt.Errorf("empty"),
		Version:        1,
		Seed:           map[string]string{},
		SourceText: func(_ map[string]string, fieldID string) string {
			return source[fieldID]
		},
		Runner: runner,
	})
	result, err := RunCompositionLoop(context.Background(), strat, nil)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, runner.calls, 3)
	assert.Equal(t, "EN-a", runner.calls[0].SourceText)
	st := result.OutputState.(*FieldWalkState)
	assert.Equal(t, "out-a", st.Draft["a"])
}
