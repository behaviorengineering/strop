package orchestration

import (
	"context"
	"fmt"
	"strings"

	"github.com/behaviorengineering/strop/pkg/runreport"
	"github.com/behaviorengineering/strop/pkg/streaming"
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
// If the strategy implements PhaseCompensator and CompensateAttempts is positive, an exhausted
// phase runs diagnose → plan → apply before hard-fail. On full success it returns strategy.Result().
// A failed phase returns an error without calling Result; upstream locked phases stay intact.
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
		var lastFailedFields map[string]string
		var attemptHistory []CompensationAttempt
		phasePassed := false

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
			attemptHistory = append(attemptHistory, CompensationAttempt{
				Attempt:  attempt,
				Score:    result.Score,
				Feedback: truncateFeedback(result.Feedback, 400),
				Passed:   result.Passed,
			})
			if result.Passed {
				if c := runreport.CollectorFromContext(ctx); c != nil {
					c.RecordPhase(string(phase.ID), attempt, true, result.Score, truncateFeedback(result.Feedback, 200))
				}
				sendCompositionEvent(eventChan, fmt.Sprintf(
					"Composition phase %s passed (score %.1f)",
					phaseLabel(phase),
					result.Score,
				))
				phasePassed = true
				break
			}

			lastFeedback = result.Feedback
			lastFailedFields = result.Fields
			feedback = result.Feedback
			if c := runreport.CollectorFromContext(ctx); c != nil {
				c.RecordPhase(string(phase.ID), attempt, false, result.Score, truncateFeedback(result.Feedback, 200))
			}
			if attempt == maxAttempts {
				break
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

		if phasePassed {
			continue
		}

		if err := runPhaseCompensation(ctx, strategy, phase, lastFeedback, lastFailedFields, attemptHistory, eventChan); err != nil {
			return nil, err
		}
	}

	sendCompositionEvent(eventChan, "Composition completed for all phases")
	return strategy.Result()
}

// runPhaseCompensation runs optional diagnose → plan → apply after normal retries exhaust.
// Strategies that do not implement PhaseCompensator (or return 0 attempts) hard-fail as before.
func runPhaseCompensation(
	ctx context.Context,
	strategy CompositionStrategy,
	phase PhaseDef,
	lastFeedback string,
	lastFailedFields map[string]string,
	attemptHistory []CompensationAttempt,
	eventChan streaming.EventChannel,
) error {
	maxAttempts := phase.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1
	}
	hardFail := func(feedback string) error {
		return fmt.Errorf(
			"composition phase %s failed after %d attempts: %s",
			phase.ID,
			maxAttempts,
			truncateFeedback(feedback, 500),
		)
	}

	comp, ok := strategy.(PhaseCompensator)
	if !ok {
		return hardFail(lastFeedback)
	}
	compBudget := comp.CompensateAttempts(phase)
	if compBudget <= 0 {
		return hardFail(lastFeedback)
	}

	feedback := lastFeedback
	failed := lastFailedFields
	history := append([]CompensationAttempt(nil), attemptHistory...)

	for attempt := 1; attempt <= compBudget; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		sendCompositionEvent(eventChan, fmt.Sprintf(
			"Composition phase %s compensate plan (attempt %d/%d)",
			phaseLabel(phase),
			attempt,
			compBudget,
		))

		evidence, err := comp.CollectEvidence(ctx, phase, feedback, failed)
		if err != nil {
			return fmt.Errorf("composition phase %s compensate collect: %w", phase.ID, err)
		}
		if evidence == nil {
			evidence = &CompensationEvidence{PhaseID: string(phase.ID), Feedback: feedback, FailedOutput: failed}
		}
		if len(evidence.AttemptHistory) == 0 {
			evidence.AttemptHistory = history
		}
		if evidence.PhaseID == "" {
			evidence.PhaseID = string(phase.ID)
		}

		plan, err := comp.PlanRepair(ctx, evidence, eventChan)
		if err != nil {
			return fmt.Errorf("composition phase %s compensate plan: %w", phase.ID, err)
		}

		sendCompositionEvent(eventChan, fmt.Sprintf(
			"Composition phase %s compensate apply (attempt %d/%d)",
			phaseLabel(phase),
			attempt,
			compBudget,
		))

		result, err := comp.ApplyAndGate(ctx, phase, evidence, plan, eventChan)
		if err != nil {
			if c := runreport.CollectorFromContext(ctx); c != nil {
				c.RecordPhase(string(phase.ID)+":compensate", attempt, false, 0, truncateFeedback(err.Error(), 200))
			}
			return fmt.Errorf("composition phase %s compensate apply: %w", phase.ID, err)
		}
		if result == nil {
			return fmt.Errorf("composition phase %s compensate apply: empty result", phase.ID)
		}
		history = append(history, CompensationAttempt{
			Attempt:  maxAttempts + attempt,
			Score:    result.Score,
			Feedback: truncateFeedback(result.Feedback, 400),
			Passed:   result.Passed,
		})
		if result.Passed {
			if c := runreport.CollectorFromContext(ctx); c != nil {
				c.RecordPhase(string(phase.ID)+":compensate", attempt, true, result.Score, truncateFeedback(result.Feedback, 200))
			}
			sendCompositionEvent(eventChan, fmt.Sprintf(
				"Composition phase %s compensate passed (score %.1f)",
				phaseLabel(phase),
				result.Score,
			))
			return nil
		}
		if c := runreport.CollectorFromContext(ctx); c != nil {
			c.RecordPhase(string(phase.ID)+":compensate", attempt, false, result.Score, truncateFeedback(result.Feedback, 200))
		}
		feedback = result.Feedback
		failed = result.Fields
		sendCompositionEvent(eventChan, fmt.Sprintf(
			"Composition phase %s compensate failed (score %.1f)",
			phaseLabel(phase),
			result.Score,
		))
	}

	return fmt.Errorf(
		"composition phase %s failed after %d attempts and %d compensate attempts: %s",
		phase.ID,
		maxAttempts,
		compBudget,
		truncateFeedback(feedback, 500),
	)
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
