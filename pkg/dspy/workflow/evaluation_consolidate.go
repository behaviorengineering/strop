package workflow

import (
	"context"
	"fmt"
	"strings"

	"github.com/behaviorengineering/strop/pkg/evaluation"
	"github.com/behaviorengineering/strop/pkg/streaming"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
)

func extractContentVersionFromInputs(inputs map[string]interface{}, fieldIterationVersion string) int {
	if inputs == nil {
		return 0
	}

	// Helper function to extract version from a value (handles multiple types).
	extractVersionFromValue := func(version interface{}) int {
		if version == nil {
			return 0
		}
		// Try int first.
		if v, ok := version.(int); ok {
			return v
		}
		// Handle float64 case (JSON unmarshaling might convert int to float64).
		if v, ok := version.(float64); ok {
			return int(v)
		}
		// Handle string case (in case it's stored as string).
		if vStr, ok := version.(string); ok {
			var v int
			if _, err := fmt.Sscanf(vStr, "%d", &v); err == nil {
				return v
			}
		}
		return 0
	}

	// Check for consistent iterationVersion field at top level.
	if iterationVersion, ok := inputs[fieldIterationVersion]; ok {
		return extractVersionFromValue(iterationVersion)
	}

	return 0
}

// buildConsolidatorInputs builds the input map for the consolidator module.
func (w *ParallelEvaluationWorkflow) buildConsolidatorInputs(
	individualEvals []*evaluation.IndividualEvaluation,
	agentScores map[string]float64,
	weightedScore float64,
	contentVersion int,
) map[string]interface{} {
	var feedbacksBuilder strings.Builder
	for _, eval := range individualEvals {
		fmt.Fprintf(&feedbacksBuilder, "=== %s (%.1f/10) ===\n", eval.AgentName, eval.Score)
		feedbacksBuilder.WriteString(eval.Feedback)
		feedbacksBuilder.WriteString("\n\n")
	}

	var scoresBuilder strings.Builder
	for agent, score := range agentScores {
		fmt.Fprintf(&scoresBuilder, "- %s: %.1f/10\n", agent, score)
	}

	consolidatorInputs := map[string]interface{}{
		w.fieldNames.IndividualFeedbacks: feedbacksBuilder.String(),
		w.fieldNames.AgentScores:         scoresBuilder.String(),
		w.fieldNames.WeightedScore:       weightedScore,
	}

	if contentVersion > 0 {
		consolidatorInputs[w.fieldNames.IterationVersion] = contentVersion
	}

	return consolidatorInputs
}

// consolidateFeedbacks uses consolidator to merge all feedbacks.
func (w *ParallelEvaluationWorkflow) consolidateFeedbacks(
	ctx context.Context,
	individualEvals []*evaluation.IndividualEvaluation,
	agentScores map[string]float64,
	weightedScore float64,
	contentVersion int,
) (string, error) {
	if w.consolidator.Module == nil {
		return "", fmt.Errorf("consolidator not initialized")
	}

	consolidatorInputs := w.buildConsolidatorInputs(individualEvals, agentScores, weightedScore, contentVersion)

	result, err := w.consolidator.Process(ctx, consolidatorInputs)
	if err != nil {
		return "", err
	}

	consolidatedFeedback, ok := result[w.fieldNames.ConsolidatedFeedback].(string)
	if !ok || consolidatedFeedback == "" {
		return "", fmt.Errorf("%s", "consolidator returned empty or invalid feedback")
	}

	return consolidatedFeedback, nil
}

// consolidateFeedbacksStream uses consolidator to merge all feedbacks with streaming support.
func (w *ParallelEvaluationWorkflow) consolidateFeedbacksStream(
	ctx context.Context,
	individualEvals []*evaluation.IndividualEvaluation,
	agentScores map[string]float64,
	weightedScore float64,
	contentVersion int,
	eventChan streaming.EventChannel,
) (string, error) {
	if w.consolidator.Module == nil {
		return "", fmt.Errorf("%s", "consolidator not initialized")
	}

	if err := emitInferenceEvent(ctx, eventChan, w.consolidator.StartEvent()); err != nil {
		return "", err
	}

	consolidatorInputs := w.buildConsolidatorInputs(individualEvals, agentScores, weightedScore, contentVersion)

	handler := w.consolidator.StreamHandler(eventChan)

	result, err := w.consolidator.Process(
		ctx,
		consolidatorInputs,
		core.WithStreamHandler(handler),
	)

	_ = emitInferenceEvent(ctx, eventChan, w.consolidator.EndEvent(result, err))

	if err != nil {
		return "", err
	}

	consolidatedFeedback, ok := result[w.fieldNames.ConsolidatedFeedback].(string)
	if !ok || consolidatedFeedback == "" {
		return "", fmt.Errorf("%s", "consolidator returned empty or invalid feedback")
	}

	return consolidatedFeedback, nil
}
