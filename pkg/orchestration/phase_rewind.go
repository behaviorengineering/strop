package orchestration

import (
	"context"
	"fmt"
	"strings"

	"github.com/behaviorengineering/strop/pkg/streaming"
)

// PhaseRewindVerdict is the outcome of an on-pass review. Empty RewindTo means continue.
type PhaseRewindVerdict struct {
	RewindTo PhaseID
	Feedback string
	Reason   string
}

// PhasePassRewinder is optional. When RunCompositionLoop enables it (RewindBudget > 0),
// AfterPhasePass runs after a phase passes (including after successful compensation).
type PhasePassRewinder interface {
	RewindBudget() int
	AfterPhasePass(ctx context.Context, phase PhaseDef, result *PhaseResult, eventChan streaming.EventChannel) (*PhaseRewindVerdict, error)
}

// DraftRewinder drops owned fields from rewindTo inclusive through later phases in the walk draft.
type DraftRewinder interface {
	RewindDraftTo(phaseID PhaseID) error
}

func phaseIndex(phases []PhaseDef, id PhaseID) int {
	for i, p := range phases {
		if p.ID == id {
			return i
		}
	}
	return -1
}

func applyPhaseRewind(
	ctx context.Context,
	strategy CompositionStrategy,
	phases []PhaseDef,
	currentIndex int,
	verdict *PhaseRewindVerdict,
	rewindsUsed int,
	budget int,
	eventChan streaming.EventChannel,
) (nextIndex int, nextFeedback string, newRewindsUsed int, err error) {
	if verdict == nil || verdict.RewindTo == "" {
		return currentIndex + 1, "", rewindsUsed, nil
	}
	target := phaseIndex(phases, verdict.RewindTo)
	if target < 0 || target > currentIndex {
		RecordPipelineAttempt(ctx, PipelineAttempt{
			Scope:    compositionScope(phases[currentIndex]),
			Action:   "phase_rewind_ignored",
			Attempt:  rewindsUsed + 1,
			Passed:   true,
			Feedback: truncateFeedback("invalid rewind target: "+string(verdict.RewindTo), 400),
		})
		sendCompositionProgress(ctx, eventChan, phases[currentIndex], 0, 0, fmt.Sprintf(
			"Composition phase %s ignored rewind to %s",
			phaseLabel(phases[currentIndex]),
			string(verdict.RewindTo),
		))
		return currentIndex + 1, "", rewindsUsed, nil
	}
	if rewindsUsed >= budget {
		reason := strings.TrimSpace(verdict.Reason)
		if reason == "" {
			reason = "rewind budget exhausted"
		}
		RecordPipelineAttempt(ctx, PipelineAttempt{
			Scope:        compositionScope(phases[currentIndex]),
			Action:       "phase_rewind_exhausted",
			Attempt:      rewindsUsed + 1,
			FailureClass: FailureHard,
			Passed:       false,
			Feedback:     truncateFeedback(reason, 400),
		})
		return currentIndex, "", rewindsUsed, fmt.Errorf(
			"composition phase %s rewind budget exhausted: %s",
			phases[currentIndex].ID,
			truncateFeedback(reason, 500),
		)
	}
	if dr, ok := strategy.(DraftRewinder); ok {
		if err := dr.RewindDraftTo(verdict.RewindTo); err != nil {
			return currentIndex, "", rewindsUsed, fmt.Errorf("composition rewind draft: %w", err)
		}
	}
	rewindsUsed++
	RecordPipelineAttempt(ctx, PipelineAttempt{
		Scope:    compositionScope(phases[currentIndex]),
		Action:   "phase_rewind",
		Attempt:  rewindsUsed,
		Passed:   true,
		Feedback: truncateFeedback(string(verdict.RewindTo)+": "+verdict.Feedback, 400),
	})
	sendCompositionProgress(ctx, eventChan, phases[currentIndex], 0, 0, fmt.Sprintf(
		"Composition phase %s rewind to %s",
		phaseLabel(phases[currentIndex]),
		string(verdict.RewindTo),
	))
	return target, verdict.Feedback, rewindsUsed, nil
}

func runAfterPhasePass(
	ctx context.Context,
	strategy CompositionStrategy,
	phases []PhaseDef,
	phase PhaseDef,
	phaseIndex int,
	result *PhaseResult,
	rewindsUsed int,
	budget int,
	eventChan streaming.EventChannel,
) (nextIndex int, nextFeedback string, newRewindsUsed int, err error) {
	rewinder, ok := strategy.(PhasePassRewinder)
	if !ok || budget <= 0 {
		return phaseIndex + 1, "", rewindsUsed, nil
	}
	verdict, err := rewinder.AfterPhasePass(ctx, phase, result, eventChan)
	if err != nil {
		return phaseIndex, "", rewindsUsed, err
	}
	return applyPhaseRewind(ctx, strategy, phases, phaseIndex, verdict, rewindsUsed, budget, eventChan)
}
