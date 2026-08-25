package orchestration

import (
	"context"
	"fmt"
	"strings"

	"github.com/behaviorengineering/strop/runreport"
	"github.com/behaviorengineering/strop/streaming"
)

// PhaseID identifies one step in a multi-phase document composition recipe.
type PhaseID string

// PhaseDef describes one composition phase: which fields it owns and retry budget.
type PhaseDef struct {
	ID          PhaseID
	DisplayName string
	MaxAttempts int
}

// PhaseResult is the outcome of one generate+gate attempt within a phase.
type PhaseResult struct {
	Fields   map[string]string
	Score    float64
	Feedback string
	Passed   bool
}

// FieldDemoUse is one section/phase demo pair recorded during composition.
type FieldDemoUse struct {
	FieldID    string
	NearID     string
	ContrastID string
}

// CompositionResult is the portable handoff after a successful RunCompositionLoop.
// OutputState and EvalPayload are job-specific (opaque to the loop).
type CompositionResult struct {
	Score          float64
	Feedback       string
	OutputState    interface{} // Job-specific assembled draft.
	EvalPayload    interface{} // Optional; e.g. *evaluation.AggregatedEvaluation.
	DemoNearID     string      // Last non-empty near demo (compat); prefer DemoUses.
	DemoContrastID string
	DemoUses       []FieldDemoUse // All section/phase demos from this compose.
}

// CompositionStrategy executes an ordered recipe of phases for one document build.
// Implementations hold draft state (locked upstream fields) across phases.
//
// Nest under RefinementStrategy.GenerateAndEvaluate when a version is assembled
// progressively (e.g. LinkedIn post skim → warmth → depth → teaser).
// Phases() may be a fixed arc or built at runtime (essay of any length = more phases).
type CompositionStrategy interface {
	Phases() []PhaseDef
	// RunPhase generates and gates one phase. feedback is evaluator/alignment text from the prior attempt on this phase.
	RunPhase(ctx context.Context, phase PhaseDef, feedback string, eventChan streaming.EventChannel) (*PhaseResult, error)
	// Result is valid after RunCompositionLoop succeeds.
	Result() (*CompositionResult, error)
}

// RunCompositionLoop runs each phase in order until it passes or exhausts MaxAttempts.
// On full success it returns strategy.Result(). A failed phase returns an error without calling Result;
// upstream locked phases are left intact on the strategy.
func RunCompositionLoop(
	ctx context.Context,
	strategy CompositionStrategy,
	eventChan streaming.EventChannel,
) (*CompositionResult, error) {
	if strategy == nil {
		return nil, fmt.Errorf("composition strategy is nil")
	}
	phases := strategy.Phases()
	if len(phases) == 0 {
		return nil, fmt.Errorf("composition recipe has no phases")
	}

	for _, phase := range phases {
		maxAttempts := phase.MaxAttempts
		if maxAttempts <= 0 {
			maxAttempts = 1
		}
		feedback := ""
		var lastFeedback string

		for attempt := 1; attempt <= maxAttempts; attempt++ {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}

			sendCompositionEvent(eventChan, fmt.Sprintf(
				"Composition phase %s (attempt %d/%d)",
				phaseLabel(phase),
				attempt,
				maxAttempts,
			))

			result, err := strategy.RunPhase(ctx, phase, feedback, eventChan)
			if err != nil {
				if c := runreport.CollectorFromContext(ctx); c != nil {
					c.RecordPhase(string(phase.ID), attempt, false, 0, truncateFeedback(err.Error(), 200))
				}
				sendCompositionEvent(eventChan, fmt.Sprintf(
					"Composition phase %s error: %s",
					phaseLabel(phase),
					truncateFeedback(err.Error(), 300),
				))
				return nil, err
			}
			if result.Passed {
				if c := runreport.CollectorFromContext(ctx); c != nil {
					c.RecordPhase(string(phase.ID), attempt, true, result.Score, truncateFeedback(result.Feedback, 200))
				}
				sendCompositionEvent(eventChan, fmt.Sprintf(
					"Composition phase %s passed (score %.1f)",
					phaseLabel(phase),
					result.Score,
				))
				break
			}

			lastFeedback = result.Feedback
			feedback = result.Feedback
			if c := runreport.CollectorFromContext(ctx); c != nil {
				c.RecordPhase(string(phase.ID), attempt, false, result.Score, truncateFeedback(result.Feedback, 200))
			}
			if attempt == maxAttempts {
				return nil, fmt.Errorf(
					"composition phase %s failed after %d attempts: %s",
					phase.ID,
					maxAttempts,
					truncateFeedback(lastFeedback, 500),
				)
			}
			retryMsg := fmt.Sprintf(
				"Composition phase %s retrying (score %.1f)",
				phaseLabel(phase),
				result.Score,
			)
			if snippet := truncateFeedback(result.Feedback, 200); snippet != "" {
				retryMsg += " — " + snippet
			}
			sendCompositionEvent(eventChan, retryMsg)
		}
	}

	sendCompositionEvent(eventChan, "Composition completed for all phases")
	return strategy.Result()
}

func phaseLabel(phase PhaseDef) string {
	if phase.DisplayName != "" {
		return phase.DisplayName
	}
	return string(phase.ID)
}

func sendCompositionEvent(eventChan streaming.EventChannel, content string) {
	streaming.SendInfo(eventChan, content)
}

func truncateFeedback(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
