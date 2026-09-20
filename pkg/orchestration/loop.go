package orchestration

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/behaviorengineering/strop/pkg/dspy/ace"
	"github.com/behaviorengineering/strop/pkg/runreport"
	"github.com/behaviorengineering/strop/pkg/streaming"

	"github.com/google/uuid"
)

// perfectScoreForStopping is the weighted score at which refinement treats the run as "perfect" and may stop.
// Must stay aligned with refinement CheckStoppingConditions (score >= 10.0).
const perfectScoreForStopping = 10.0

// RunRefinementLoop runs the generic refinement loop: generate → evaluate → check stop → save or recurse.
// Optional self-healing on score decrease when the strategy implements HealingStrategy and the policy allows it.
// Returns the selected version ID (last saved when continuing, or previous when score decreased / max versions).
// When an ACE Manager is on ctx, each version is one trajectory (Start/RecordStep/End); host owns Manager lifetime.
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
	if err := assertACEBinding(ctx, entityID.String(), meta.Job); err != nil {
		return uuid.Nil, err
	}
	ctx, finishReport := runreport.StartSession(ctx, cfg, meta)
	defer func() {
		finishReport(err)
	}()

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
		aceJob:           meta.Job,
		aceEntityID:      entityID.String(),
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
	aceJob           string
	aceEntityID      string
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

	attemptCtx, finishACE := beginACEAttempt(ctx, state.aceEntityID, state.aceJob, state.version, "")
	defer finishACE(ace.OutcomeFailure)

	out, err := strategy.GenerateAndEvaluate(attemptCtx, state.version, state.previousFeedback, state.state, eventChan)
	if err != nil {
		finishACE(ace.OutcomeFailure)
		return uuid.Nil, err
	}
	if c := runreport.CollectorFromContext(ctx); c != nil {
		c.RecordRefinement(state.version, out.Score, truncateFeedback(out.Feedback, 200))
	}
	recordACERefinementStep(attemptCtx, state.version, out.Score, out.Feedback, firstNonEmpty(out.Rationale, out.EvalRationale))

	shouldStop, returnID := policy.CheckStoppingConditions(
		out.Score, state.previousScore, state.version,
		strategy.ContextID(), state.selectedID, out.Feedback,
	)
	scoreDecreased := state.previousScore >= 0.0 && out.Score < state.previousScore

	if shouldStop && returnID == uuid.Nil && !scoreDecreased {
		id, err := strategy.SaveVersion(ctx, state.version, out.OutputState, out)
		if err != nil {
			finishACE(ace.OutcomeFailure)
			return uuid.Nil, err
		}
		finishACE(ace.OutcomeSuccess)
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
					finishACE(ace.OutcomePartial)
					return runRefinementRecursive(ctx, strategy, policy, maxVersions, eventChan, loopState{
						version:          state.version + 1,
						previousScore:    state.previousScore,
						previousFeedback: healingResult.CorrectiveFeedback,
						selectedID:       state.selectedID,
						state:            state.state,
						healingAttempts:  state.healingAttempts + 1,
						aceJob:           state.aceJob,
						aceEntityID:      state.aceEntityID,
					})
				}
			}
		}
		finishACE(ace.OutcomeFailure)
		sendEvent(eventChan, fmt.Sprintf("Score decreased from %.1f to %.1f. Stopping refinement.", state.previousScore, out.Score))
		return state.selectedID, nil
	}

	id, err := strategy.SaveVersion(ctx, state.version, out.OutputState, out)
	if err != nil {
		finishACE(ace.OutcomeFailure)
		return uuid.Nil, err
	}
	finishACE(ace.OutcomePartial)
	return runRefinementRecursive(ctx, strategy, policy, maxVersions, eventChan, loopState{
		version:          state.version + 1,
		previousScore:    out.Score,
		previousFeedback: out.Feedback,
		selectedID:       id,
		state:            out.OutputState,
		healingAttempts:  0,
		aceJob:           state.aceJob,
		aceEntityID:      state.aceEntityID,
	})
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
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
	if err := assertACEBinding(ctx, entityID.String(), meta.Job); err != nil {
		return uuid.Nil, err
	}
	ctx, finishReport := runreport.StartSession(ctx, cfg, meta)
	defer func() {
		finishReport(err)
	}()

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
	aceEntityID := entityID.String()
	aceJob := meta.Job
	for _, i := range indices {
		itemFeedback := loopCtx.PreviousFeedback // each item gets initial feedback at round 1.
		previousScore := -1.0
		healingAttempts := 0
		for round := 1; round <= maxVersions; round++ {
			var (
				score     float64
				feedback  string
				rationale string
				runErr    error
				stopItem  bool
			)
			func() {
				attemptCtx, finishACE := beginACEAttempt(ctx, aceEntityID, aceJob, round, fmt.Sprintf("item %d round %d", i, round))
				defer finishACE(ace.OutcomeFailure)
				score, feedback, rationale, runErr = strategy.GenerateAndEvaluateOne(attemptCtx, i, round, itemFeedback, state, eventChan)
				if runErr != nil {
					finishACE(ace.OutcomeFailure)
					return
				}
				if c := runreport.CollectorFromContext(ctx); c != nil {
					c.RecordPerItemRefinement(i, round, score, truncateFeedback(feedback, 200))
				}
				recordACEItemStep(attemptCtx, i, round, score, feedback, rationale)
				// Always attempt healing on score decrease before stopping this item.
				// This mirrors single-entity loop behavior and avoids prematurely accepting degraded rounds.
				if previousScore >= 0.0 && score < previousScore && healingAttempts < policy.MaxHealingAttempts() {
					healingResult, healingErr := policy.AttemptHealing(ctx, nil, itemFeedback, previousScore, score)
					if healingErr == nil && healingResult != nil && healingResult.ShouldRetry {
						sendEvent(eventChan, fmt.Sprintf("Item %d/%d: score decreased from %.1f to %.1f. Self-healing: retrying with corrected feedback.", i+1, n, previousScore, score))
						if c := runreport.CollectorFromContext(ctx); c != nil {
							c.RecordHealing(previousScore, score, fmt.Sprintf("item %d self-healing retry", i+1))
						}
						finishACE(ace.OutcomePartial)
						itemFeedback = healingResult.CorrectiveFeedback
						healingAttempts++
						return
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
						finishACE(ace.OutcomeSuccess)
						sendEvent(eventChan, fmt.Sprintf("Item %d/%d: accepted (score %.1f)", i+1, n, score))
					} else if shouldStop {
						finishACE(ace.OutcomeSuccess)
					} else {
						finishACE(ace.OutcomePartial)
					}
					stopItem = true
					return
				}
				finishACE(ace.OutcomePartial)
				itemFeedback = feedback
				previousScore = score
				healingAttempts = 0
			}()
			if runErr != nil {
				return uuid.Nil, runErr
			}
			if stopItem {
				break
			}
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
