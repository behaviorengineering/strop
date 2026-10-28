package orchestration

import (
	"context"
	"sync"
	"testing"

	"github.com/behaviorengineering/strop/pkg/evaluation"
	"github.com/behaviorengineering/strop/pkg/streaming"
)

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
