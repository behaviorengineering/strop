package orchestration

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/behaviorengineering/strop/pkg/evaluation"
	"github.com/behaviorengineering/strop/pkg/streaming"
)

// fakePhaseRunner is a test double for PhaseWalkRunner.
type fakePhaseRunner struct {
	t            *testing.T
	calls        []PhaseWalkRequest
	failFirst    map[string]bool
	seenAttempts map[string]int
}

func (f *fakePhaseRunner) Run(_ context.Context, req PhaseWalkRequest, _ streaming.EventChannel) (*PhaseWalkResponse, error) {
	f.calls = append(f.calls, req)
	if f.seenAttempts == nil {
		f.seenAttempts = map[string]int{}
	}
	f.seenAttempts[req.PhaseID]++
	attempt := f.seenAttempts[req.PhaseID]

	score := 8.0
	if f.failFirst[req.PhaseID] && attempt == 1 {
		score = 3.0
		fields := map[string]string{}
		for _, fid := range req.OwnedFields {
			fields[fid] = "weak-" + fid
		}
		return &PhaseWalkResponse{
			OutputFields: fields,
			Passed:       false,
			Score:        score,
			Feedback:     "retry-" + req.PhaseID,
		}, nil
	}

	fields := map[string]string{}
	for _, fid := range req.OwnedFields {
		fields[fid] = "out-" + fid
	}
	return &PhaseWalkResponse{
		OutputFields: fields,
		Passed:       true,
		Score:        score,
		Feedback:     "ok-" + req.PhaseID,
		Rationale:    "r-" + req.PhaseID,
		Eval: &evaluation.AggregatedEvaluation{
			WeightedScore:        score,
			CriterionScores:      map[string]float64{"quality": score},
			ConsolidatedFeedback: "ok-" + req.PhaseID,
		},
	}, nil
}

// multiPhaseWalkDefs returns three phases with two owned fields each.
func multiPhaseWalkDefs() []PhaseDef {
	return []PhaseDef{
		{ID: "intro", DisplayName: "intro", MaxAttempts: 2},
		{ID: "body", DisplayName: "body", MaxAttempts: 2},
		{ID: "close", DisplayName: "close", MaxAttempts: 2},
	}
}

func multiPhaseWalkOwned() PhaseWalkOwnedFields {
	return PhaseWalkOwnedFields{
		"intro": {"intro_title", "intro_summary"},
		"body":  {"body_main", "body_detail"},
		"close": {"close_cta"},
	}
}

func TestPhaseWalkStrategy_happyPath(t *testing.T) {
	t.Parallel()
	runner := &fakePhaseRunner{t: t}
	strategy := NewPhaseWalkStrategy(PhaseWalkConfig{
		Phases:      multiPhaseWalkDefs(),
		OwnedFields: multiPhaseWalkOwned(),
		Runner:      runner,
	})
	result, err := RunCompositionLoop(context.Background(), strategy, nil)
	require.NoError(t, err)
	require.NotNil(t, result)

	st, ok := result.OutputState.(*PhaseWalkState)
	require.True(t, ok)
	assert.Equal(t, "out-intro_title", st.Draft["intro_title"])
	assert.Equal(t, "out-intro_summary", st.Draft["intro_summary"])
	assert.Equal(t, "out-body_main", st.Draft["body_main"])
	assert.Equal(t, "out-close_cta", st.Draft["close_cta"])
	assert.Len(t, st.Scores, 3)
}

func TestPhaseWalkStrategy_priorLockedFieldsExcludeLaterPhases(t *testing.T) {
	t.Parallel()
	runner := &fakePhaseRunner{t: t}
	strategy := NewPhaseWalkStrategy(PhaseWalkConfig{
		Phases:      multiPhaseWalkDefs(),
		OwnedFields: multiPhaseWalkOwned(),
		Runner:      runner,
	})
	_, err := RunCompositionLoop(context.Background(), strategy, nil)
	require.NoError(t, err)

	// body call: should have intro fields locked, but not close fields.
	var bodyCall *PhaseWalkRequest
	for i := range runner.calls {
		if runner.calls[i].PhaseID == "body" {
			bodyCall = &runner.calls[i]
			break
		}
	}
	require.NotNil(t, bodyCall)
	assert.Equal(t, "out-intro_title", bodyCall.LockedOutput["intro_title"])
	assert.Equal(t, "out-intro_summary", bodyCall.LockedOutput["intro_summary"])
	_, hasClose := bodyCall.LockedOutput["close_cta"]
	assert.False(t, hasClose, "later phase fields must not appear in locked output")
}

func TestPhaseWalkStrategy_retryWiresPreviousFailedOutput(t *testing.T) {
	t.Parallel()
	runner := &fakePhaseRunner{t: t, failFirst: map[string]bool{"body": true}}
	strategy := NewPhaseWalkStrategy(PhaseWalkConfig{
		Phases:      multiPhaseWalkDefs(),
		OwnedFields: multiPhaseWalkOwned(),
		Runner:      runner,
	})
	_, err := RunCompositionLoop(context.Background(), strategy, nil)
	require.NoError(t, err)

	// There should be two calls for "body": one fail then one pass.
	bodyCalls := []PhaseWalkRequest{}
	for _, c := range runner.calls {
		if c.PhaseID == "body" {
			bodyCalls = append(bodyCalls, c)
		}
	}
	require.Len(t, bodyCalls, 2)
	// Second attempt should have the first attempt's weak output as PreviousFailedOutput.
	assert.Equal(t, "weak-body_main", bodyCalls[1].PreviousFailedOutput["body_main"])
	assert.Equal(t, "weak-body_detail", bodyCalls[1].PreviousFailedOutput["body_detail"])
	assert.Contains(t, bodyCalls[1].PreviousFeedback, "retry-")
}

func TestPhaseWalkStrategy_passedFailedFieldsNotInDraft(t *testing.T) {
	t.Parallel()
	runner := &fakePhaseRunner{t: t, failFirst: map[string]bool{"body": true}}
	strategy := NewPhaseWalkStrategy(PhaseWalkConfig{
		Phases:      multiPhaseWalkDefs(),
		OwnedFields: multiPhaseWalkOwned(),
		Runner:      runner,
	})
	result, err := RunCompositionLoop(context.Background(), strategy, nil)
	require.NoError(t, err)

	st := result.OutputState.(*PhaseWalkState)
	// After the retry the draft should hold the *passing* values, not the weak ones.
	assert.Equal(t, "out-body_main", st.Draft["body_main"])
	assert.Equal(t, "out-body_detail", st.Draft["body_detail"])
}

func TestPhaseWalkStrategy_mergeOnFail_updatesDraftBeforeRetry(t *testing.T) {
	t.Parallel()
	runner := &fakePhaseRunner{t: t, failFirst: map[string]bool{"body": true}}
	strategy := NewPhaseWalkStrategy(PhaseWalkConfig{
		Phases:      multiPhaseWalkDefs(),
		OwnedFields: multiPhaseWalkOwned(),
		Runner:      runner,
		MergeOnFail: true,
	}).(*phaseWalkStrategy)

	ctx := context.Background()
	introPhase := PhaseDef{ID: "intro", DisplayName: "intro", MaxAttempts: 2}
	bodyPhase := PhaseDef{ID: "body", DisplayName: "body", MaxAttempts: 2}

	_, err := strategy.RunPhase(ctx, introPhase, "", nil)
	require.NoError(t, err)

	_, err = strategy.RunPhase(ctx, bodyPhase, "", nil)
	require.NoError(t, err)
	assert.Equal(t, "weak-body_main", strategy.draft["body_main"])
	assert.Equal(t, "weak-body_detail", strategy.draft["body_detail"])

	_, err = strategy.RunPhase(ctx, bodyPhase, "retry-body", nil)
	require.NoError(t, err)
	assert.Equal(t, "out-body_main", strategy.draft["body_main"])
	assert.Equal(t, "out-body_detail", strategy.draft["body_detail"])
}

func TestPhaseWalkStrategy_mergeOnFail_falseLeavesDraftUnchangedOnFail(t *testing.T) {
	t.Parallel()
	runner := &fakePhaseRunner{t: t, failFirst: map[string]bool{"body": true}}
	strategy := NewPhaseWalkStrategy(PhaseWalkConfig{
		Phases:      multiPhaseWalkDefs(),
		OwnedFields: multiPhaseWalkOwned(),
		Runner:      runner,
	}).(*phaseWalkStrategy)

	ctx := context.Background()
	introPhase := PhaseDef{ID: "intro", DisplayName: "intro", MaxAttempts: 2}
	bodyPhase := PhaseDef{ID: "body", DisplayName: "body", MaxAttempts: 2}

	_, err := strategy.RunPhase(ctx, introPhase, "", nil)
	require.NoError(t, err)

	_, err = strategy.RunPhase(ctx, bodyPhase, "", nil)
	require.NoError(t, err)
	_, hasMain := strategy.draft["body_main"]
	_, hasDetail := strategy.draft["body_detail"]
	assert.False(t, hasMain)
	assert.False(t, hasDetail)
}

func TestPhaseWalkStrategy_pluggableFinalize(t *testing.T) {
	t.Parallel()
	runner := &fakePhaseRunner{t: t}
	finalizeCalled := false
	strategy := NewPhaseWalkStrategy(PhaseWalkConfig{
		Phases:      multiPhaseWalkDefs(),
		OwnedFields: multiPhaseWalkOwned(),
		Runner:      runner,
		Finalize: func(draft map[string]string, evals []evaluation.LabeledEval, scores []float64) (*CompositionResult, error) {
			finalizeCalled = true
			// Teaser-authoritative style: return last phase's eval as the authoritative result.
			var lastEval *evaluation.AggregatedEvaluation
			if len(evals) > 0 {
				lastEval = evals[len(evals)-1].Eval
			}
			if lastEval == nil {
				return nil, fmt.Errorf("finalize: no eval available")
			}
			return &CompositionResult{
				Score:       lastEval.WeightedScore,
				Feedback:    lastEval.ConsolidatedFeedback,
				OutputState: draft,
				EvalPayload: lastEval,
			}, nil
		},
	})
	result, err := RunCompositionLoop(context.Background(), strategy, nil)
	require.NoError(t, err)
	require.True(t, finalizeCalled)
	require.NotNil(t, result)
	// The payload should be the last phase's eval.
	agg, ok := result.EvalPayload.(*evaluation.AggregatedEvaluation)
	require.True(t, ok)
	assert.Equal(t, "ok-close", agg.ConsolidatedFeedback)
}

func TestPhaseWalkStrategy_noOwnedFieldsAcceptsAllReturned(t *testing.T) {
	t.Parallel()
	runner := &fakePhaseRunner{t: t}
	// No ownership declared — runner can return any fields.
	phases := []PhaseDef{
		{ID: "alpha", MaxAttempts: 1},
	}
	strategy := NewPhaseWalkStrategy(PhaseWalkConfig{
		Phases: phases,
		Runner: runner,
	})
	result, err := RunCompositionLoop(context.Background(), strategy, nil)
	require.NoError(t, err)
	require.NotNil(t, result)
}

func TestPhaseWalkStrategy_RewindDraftTo(t *testing.T) {
	t.Parallel()
	runner := &fakePhaseRunner{t: t}
	strategy := NewPhaseWalkStrategy(PhaseWalkConfig{
		Phases:      multiPhaseWalkDefs(),
		OwnedFields: multiPhaseWalkOwned(),
		Runner:      runner,
	})
	_, err := RunCompositionLoop(context.Background(), strategy, nil)
	require.NoError(t, err)
	dr, ok := strategy.(DraftRewinder)
	require.True(t, ok)
	require.NoError(t, dr.RewindDraftTo("body"))
	st, err := strategy.Result()
	require.NoError(t, err)
	draft := st.OutputState.(*PhaseWalkState).Draft
	assert.Equal(t, "out-intro_title", draft["intro_title"])
	assert.Empty(t, draft["body_main"])
	assert.Empty(t, draft["close_cta"])
}

func TestPhaseWalkStrategy_seedPopulatesDraft(t *testing.T) {
	t.Parallel()
	runner := &fakePhaseRunner{t: t}
	strategy := NewPhaseWalkStrategy(PhaseWalkConfig{
		Phases:      multiPhaseWalkDefs(),
		OwnedFields: multiPhaseWalkOwned(),
		Runner:      runner,
		Seed:        map[string]string{"extra": "seeded"},
	})
	result, err := RunCompositionLoop(context.Background(), strategy, nil)
	require.NoError(t, err)
	st := result.OutputState.(*PhaseWalkState)
	assert.Equal(t, "seeded", st.Draft["extra"])
}

type phaseWalkCountRunner struct {
	mu    sync.Mutex
	count map[string]int
}

func (r *phaseWalkCountRunner) Run(_ context.Context, req PhaseWalkRequest, _ streaming.EventChannel) (*PhaseWalkResponse, error) {
	r.mu.Lock()
	r.count[req.PhaseID]++
	n := r.count[req.PhaseID]
	r.mu.Unlock()

	switch req.PhaseID {
	case "p1":
		return &PhaseWalkResponse{
			OutputFields: map[string]string{"a": "a1"},
			Passed:       true,
			Score:        1,
		}, nil
	case "p2":
		if n == 1 {
			return &PhaseWalkResponse{
				OutputFields: map[string]string{"b": "b1"},
				Passed:       true,
				Score:        1,
			}, nil
		}
		return &PhaseWalkResponse{
			OutputFields: map[string]string{"b": "b2"},
			Passed:       true,
			Score:        1,
		}, nil
	case "p3":
		if n == 1 {
			return &PhaseWalkResponse{
				OutputFields: map[string]string{"c": "c1"},
				Passed:       false,
				Feedback:     "semantic fail",
				RestartAt:    "p2",
			}, nil
		}
		return &PhaseWalkResponse{
			OutputFields: map[string]string{"c": "c2"},
			Passed:       true,
			Score:        1,
		}, nil
	default:
		return &PhaseWalkResponse{Passed: true}, nil
	}
}

func TestRunCompositionLoop_phaseWalkRewindsDownstream(t *testing.T) {
	runner := &phaseWalkCountRunner{count: map[string]int{}}
	strategy := NewPhaseWalkStrategy(PhaseWalkConfig{
		Phases: []PhaseDef{
			{ID: "p1", MaxAttempts: 2},
			{ID: "p2", MaxAttempts: 2},
			{ID: "p3", MaxAttempts: 2},
		},
		OwnedFields: PhaseWalkOwnedFields{
			"p1": {"a"},
			"p2": {"b"},
			"p3": {"c"},
		},
		Runner: runner,
		Finalize: func(draft map[string]string, _ []evaluation.LabeledEval, _ []float64) (*CompositionResult, error) {
			cp := make(map[string]string, len(draft))
			for k, v := range draft {
				cp[k] = v
			}
			return &CompositionResult{OutputState: &PhaseWalkState{Draft: cp}}, nil
		},
	})

	out, err := RunCompositionLoop(context.Background(), strategy, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out == nil {
		t.Fatal("nil result")
	}
	st, ok := out.OutputState.(*PhaseWalkState)
	if !ok || st.Draft["b"] != "b2" || st.Draft["c"] != "c2" {
		t.Fatalf("draft %v ok=%v", st, ok)
	}
	if runner.count["p2"] < 2 || runner.count["p3"] < 2 {
		t.Fatalf("counts p2=%d p3=%d", runner.count["p2"], runner.count["p3"])
	}
}
