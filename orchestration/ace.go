package orchestration

import (
	"context"
	"fmt"

	"github.com/behaviorengineering/strop/dspy/ace"
)

// beginACEAttempt starts a trajectory when a Manager is on ctx and attaches the recorder.
// finish(outcome) ends the trajectory once. Callers should defer finish(OutcomeFailure) for early returns.
func beginACEAttempt(ctx context.Context, entityID, job string, version int, query string) (context.Context, func(ace.Outcome)) {
	m := ace.FromContext(ctx)
	if m == nil {
		return ctx, func(ace.Outcome) {}
	}
	if query == "" {
		query = fmt.Sprintf("version %d", version)
	}
	rec := m.StartTrajectory(entityID, job, query)
	ctx = ace.WithRecorder(ctx, rec)
	finished := false
	finish := func(outcome ace.Outcome) {
		if finished {
			return
		}
		finished = true
		m.EndTrajectory(ctx, rec, outcome)
	}
	return ctx, finish
}

func recordACERefinementStep(ctx context.Context, version int, score float64, feedback, rationale string) {
	rec := ace.RecorderFromContext(ctx)
	if rec == nil {
		return
	}
	reasoning := fmt.Sprintf("version %d score %.1f: %s", version, score, truncateFeedback(feedback, 200))
	if rationale != "" {
		reasoning = reasoning + " | " + truncateFeedback(rationale, 200)
	}
	rec.RecordStep("refine", "", reasoning, map[string]any{
		"version": version,
		"score":   score,
	}, nil, nil)
}

func recordACEItemStep(ctx context.Context, itemIndex, round int, score float64, feedback, rationale string) {
	rec := ace.RecorderFromContext(ctx)
	if rec == nil {
		return
	}
	reasoning := fmt.Sprintf("item %d round %d score %.1f: %s", itemIndex, round, score, truncateFeedback(feedback, 200))
	if rationale != "" {
		reasoning = reasoning + " | " + truncateFeedback(rationale, 200)
	}
	rec.RecordStep("refine_item", "", reasoning, map[string]any{
		"item":  itemIndex,
		"round": round,
		"score": score,
	}, nil, nil)
}

func assertACEBinding(ctx context.Context, entityID, job string) error {
	m := ace.FromContext(ctx)
	if m == nil {
		return nil
	}
	return m.CheckBinding(entityID, job)
}
