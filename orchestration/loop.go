package orchestration

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/behaviorengineering/strop/runreport"
	"github.com/behaviorengineering/strop/streaming"

	"github.com/google/uuid"
)

// perfectScoreForStopping is the weighted score at which refinement treats the run as "perfect" and may stop.
// Must stay aligned with refinement CheckStoppingConditions (score >= 10.0).
const perfectScoreForStopping = 10.0

// RunRefinementLoop runs the generic refinement loop: generate → evaluate → check stop → save or recurse.
// Optional self-healing on score decrease when the strategy implements HealingStrategy and the policy allows it.
// Returns the selected version ID (last saved when continuing, or previous when score decreased / max versions).
func RunRefinementLoop(
	ctx context.Context,
	entityID uuid.UUID,
	strategy RefinementStrategy,
	policy StoppingPolicy,
	maxVersions int,
	eventChan streaming.EventChannel,
) (selectedVersionID uuid.UUID, err error) {
	cfg := runreport.ConfigFromContext(ctx)
	loopCtx, err := strategy.LoadContext(ctx, entityID)
	if err != nil {
		return uuid.Nil, err
	}
	meta := runreport.ResolveMeta(strategy, entityID.String(), loopCtx.NextVersion)
	ctx, finishReport := runreport.StartSession(ctx, cfg, meta)
	defer finishReport(err)

	previousScore := -1.0
	if loopCtx.InitialPreviousScore != nil {
		previousScore = *loopCtx.InitialPreviousScore
	}
	selectedID := uuid.Nil
	if loopCtx.InitialSelectedID != nil {
		selectedID = *loopCtx.InitialSelectedID
	}
	state := loopState{
		version:          loopCtx.NextVersion,
		previousScore:    previousScore,
		previousFeedback: loopCtx.PreviousFeedback,
		selectedID:       selectedID,
		state:            loopCtx.State,
		healingAttempts:  0,
	}
	return runRefinementRecursive(ctx, strategy, policy, maxVersions, eventChan, state)
}

type loopState struct {
	version          int
	previousScore    float64
	previousFeedback string
	selectedID       uuid.UUID
	state            interface{}
	healingAttempts  int
}

func runRefinementRecursive(
	ctx context.Context,
	strategy RefinementStrategy,
	policy StoppingPolicy,
	maxVersions int,
	eventChan streaming.EventChannel,
	state loopState,
) (uuid.UUID, error) {
	if state.version > maxVersions {
		return state.selectedID, nil
	}

	out, err := strategy.GenerateAndEvaluate(ctx, state.version, state.previousFeedback, state.state, eventChan)
	if err != nil {
		return uuid.Nil, err
	}
	if c := runreport.CollectorFromContext(ctx); c != nil {
		c.RecordRefinement(state.version, out.Score, truncateFeedback(out.Feedback, 200))
	}

	shouldStop, returnID := policy.CheckStoppingConditions(
		out.Score, state.previousScore, state.version,
		strategy.ContextID(), state.selectedID, out.Feedback,
	)
	scoreDecreased := state.previousScore >= 0.0 && out.Score < state.previousScore

	if shouldStop && returnID == uuid.Nil && !scoreDecreased {
		id, err := strategy.SaveVersion(ctx, state.version, out.OutputState, out)
		if err != nil {
			return uuid.Nil, err
		}
		sendEvent(eventChan, "Perfect score achieved, stopping refinement")
		return id, nil
	}

	if shouldStop && (returnID != uuid.Nil || scoreDecreased) {
		if state.healingAttempts < policy.MaxHealingAttempts() {
			if healing, ok := strategy.(HealingStrategy); ok {
				diagnosis := healing.DiagnoseForHealing(state.state, out.OutputState, state.previousFeedback)
				healingResult, attemptErr := policy.AttemptHealing(ctx, diagnosis, state.previousFeedback, state.previousScore, out.Score)
				if attemptErr == nil && healingResult != nil && healingResult.ShouldRetry {
					sendEvent(eventChan, fmt.Sprintf("Score decreased from %.1f to %.1f. Self-healing: retrying with corrected feedback.", state.previousScore, out.Score))
					if c := runreport.CollectorFromContext(ctx); c != nil {
						c.RecordHealing(state.previousScore, out.Score, "self-healing retry after score decrease")
					}
					return runRefinementRecursive(ctx, strategy, policy, maxVersions, eventChan, loopState{
						version:          state.version + 1,
						previousScore:    state.previousScore,
						previousFeedback: healingResult.CorrectiveFeedback,
						selectedID:       state.selectedID,
						state:            state.state,
						healingAttempts:  state.healingAttempts + 1,
					})
				}
			}
		}
		sendEvent(eventChan, fmt.Sprintf("Score decreased from %.1f to %.1f. Stopping refinement.", state.previousScore, out.Score))
		return state.selectedID, nil
	}

	id, err := strategy.SaveVersion(ctx, state.version, out.OutputState, out)
	if err != nil {
		return uuid.Nil, err
	}
	return runRefinementRecursive(ctx, strategy, policy, maxVersions, eventChan, loopState{
		version:          state.version + 1,
		previousScore:    out.Score,
		previousFeedback: out.Feedback,
		selectedID:       id,
		state:            out.OutputState,
		healingAttempts:  0,
	})
}

// normalizePerItemIndices returns which 0-based item indices to process.
// If requested is nil or empty, returns [0, 1, ..., n-1]. Otherwise returns unique valid indices in ascending order.
func normalizePerItemIndices(requested []int, n int) []int {
	if n <= 0 {
		return nil
	}
	if len(requested) == 0 {
		out := make([]int, n)
		for i := 0; i < n; i++ {
			out[i] = i
		}
		return out
	}
	seen := make(map[int]struct{})
	for _, idx := range requested {
		if idx >= 0 && idx < n {
			seen[idx] = struct{}{}
		}
	}
	out := make([]int, 0, len(seen))
	for idx := range seen {
		out = append(out, idx)
	}
	sort.Ints(out)
	return out
}

// RunPerItemRefinementLoop runs one refinement loop per item (generate → evaluate → refine until accepted or max),
// then saves once at the end. Use for pipelines with N items (e.g. chapters) where each item has its own sub-loop.
func RunPerItemRefinementLoop(
	ctx context.Context,
	entityID uuid.UUID,
	strategy PerItemRefinementStrategy,
	policy StoppingPolicy,
	maxVersions int,
	eventChan streaming.EventChannel,
) (versionID uuid.UUID, err error) {
	return RunPerItemRefinementLoopWithIndices(ctx, entityID, strategy, policy, maxVersions, eventChan, nil, 0)
}

// RunPerItemRefinementLoopWithIndices is like RunPerItemRefinementLoop but only runs the refinement sub-loop
// for indices listed in itemIndices. If itemIndices is nil or empty, every item from 0 to ItemCount-1 is processed.
// minRoundsBeforePerfectScore, when > 0, prevents stopping on a perfect score until that many rounds have
// completed for the item (so the model must see evaluator feedback at least once more). Pass 0 for default behavior.
func RunPerItemRefinementLoopWithIndices(
	ctx context.Context,
	entityID uuid.UUID,
	strategy PerItemRefinementStrategy,
	policy StoppingPolicy,
	maxVersions int,
	eventChan streaming.EventChannel,
	itemIndices []int,
	minRoundsBeforePerfectScore int,
) (versionID uuid.UUID, err error) {
	cfg := runreport.ConfigFromContext(ctx)
	loopCtx, err := strategy.LoadContext(ctx, entityID)
	if err != nil {
		return uuid.Nil, err
	}
	meta := runreport.ResolveMeta(strategy, entityID.String(), loopCtx.NextVersion)
	ctx, finishReport := runreport.StartSession(ctx, cfg, meta)
	defer finishReport(err)

	state := loopCtx.State
	n := strategy.ItemCount(state)
	if n == 0 {
		sendEvent(eventChan, "Saving version (no items to refine)...")
		return strategy.SaveVersion(ctx, loopCtx.NextVersion, state, 0, "", "")
	}
	indices := normalizePerItemIndices(itemIndices, n)
	if len(indices) == 0 {
		return uuid.Nil, fmt.Errorf("no valid item indices to process (item count %d)", n)
	}
	var allScores []float64
	var allFeedbacks []string
	var allRationales []string
	for _, i := range indices {
		itemFeedback := loopCtx.PreviousFeedback // each item gets initial feedback at round 1.
		previousScore := -1.0
		healingAttempts := 0
		for round := 1; round <= maxVersions; round++ {
			score, feedback, rationale, runErr := strategy.GenerateAndEvaluateOne(ctx, i, round, itemFeedback, state, eventChan)
			if runErr != nil {
				return uuid.Nil, runErr
			}
			if c := runreport.CollectorFromContext(ctx); c != nil {
				c.RecordPerItemRefinement(i, round, score, truncateFeedback(feedback, 200))
			}
			// Always attempt healing on score decrease before stopping this item.
			// This mirrors single-entity loop behavior and avoids prematurely accepting degraded rounds.
			if previousScore >= 0.0 && score < previousScore && healingAttempts < policy.MaxHealingAttempts() {
				healingResult, healingErr := policy.AttemptHealing(ctx, nil, itemFeedback, previousScore, score)
				if healingErr == nil && healingResult != nil && healingResult.ShouldRetry {
					sendEvent(eventChan, fmt.Sprintf("Item %d/%d: score decreased from %.1f to %.1f. Self-healing: retrying with corrected feedback.", i+1, n, previousScore, score))
					if c := runreport.CollectorFromContext(ctx); c != nil {
						c.RecordHealing(previousScore, score, fmt.Sprintf("item %d self-healing retry", i+1))
					}
					itemFeedback = healingResult.CorrectiveFeedback
					healingAttempts++
					continue
				}
			}
			shouldStop, _ := policy.CheckStoppingConditions(score, previousScore, round, strategy.ContextID(), uuid.Nil, feedback)
			if minRoundsBeforePerfectScore > 0 && shouldStop && score >= perfectScoreForStopping && round < minRoundsBeforePerfectScore {
				sendEvent(eventChan, fmt.Sprintf("Item %d/%d: score %.1f — refinement round %d/%d required before accepting perfect score; continuing with feedback",
					i+1, n, score, round, minRoundsBeforePerfectScore))
				shouldStop = false
			}
			// Consolidated feedback uses "[ ]" for unchecked items; do not stop (e.g. on score regression) while issues remain and rounds are left.
			if shouldStop && score < perfectScoreForStopping && strings.Contains(feedback, "[ ]") && round < maxVersions {
				sendEvent(eventChan, fmt.Sprintf("Item %d/%d: score %.1f — feedback still lists unchecked items; continuing refinement (round %d/%d)",
					i+1, n, score, round, maxVersions))
				shouldStop = false
			}
			if shouldStop || round == maxVersions {
				allScores = append(allScores, score)
				allFeedbacks = append(allFeedbacks, feedback)
				allRationales = append(allRationales, rationale)
				if shouldStop && round < maxVersions {
					sendEvent(eventChan, fmt.Sprintf("Item %d/%d: accepted (score %.1f)", i+1, n, score))
				}
				break
			}
			itemFeedback = feedback
			previousScore = score
			healingAttempts = 0
		}
	}
	avgScore := 0.0
	for _, s := range allScores {
		avgScore += s
	}
	if len(allScores) > 0 {
		avgScore /= float64(len(allScores))
	}
	combinedFeedback := strings.Join(allFeedbacks, "\n\n")
	combinedRationale := strings.Join(allRationales, "\n\n")
	sendEvent(eventChan, "Saving version (aggregating per-item evaluations)...")
	return strategy.SaveVersion(ctx, loopCtx.NextVersion, state, avgScore, combinedFeedback, combinedRationale)
}

func sendEvent(ch streaming.EventChannel, content string) {
	streaming.SendInfo(ch, content)
}
