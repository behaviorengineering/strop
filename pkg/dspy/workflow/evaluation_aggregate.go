package workflow

import (
	"fmt"
	"math"

	"github.com/behaviorengineering/strop/pkg/evaluation"
)

func (w *ParallelEvaluationWorkflow) calculateWeightedScore(
	individualEvals []*evaluation.IndividualEvaluation,
) (float64, map[string]float64, error) {
	var weightedSum float64
	var totalWeight float64
	agentScores := make(map[string]float64)

	for _, eval := range individualEvals {
		weight := 1.0
		if w.roleInfo != nil {
			weight = w.roleInfo.EvaluatorWeight(evaluation.EvaluatorKey(eval.AgentID))
		}
		weightedSum += eval.Score * weight
		totalWeight += weight
		agentScores[eval.AgentName] = eval.Score
	}

	// Validate we have weights before division.
	if totalWeight == 0 {
		return 0, nil, fmt.Errorf("no valid evaluations with weights - cannot calculate weighted score")
	}

	// Calculate weighted average.
	weightedScoreRaw := weightedSum / totalWeight

	// This ensures the stored score matches the feedback string exactly.
	weightedScore := math.Round(weightedScoreRaw*100) / 100

	return weightedScore, agentScores, nil
}

// Validates that ALL configured criteria have scores from ALL evaluators - missing scores are system errors requiring retry.
func (w *ParallelEvaluationWorkflow) calculateWeightedCriterionScores(
	individualEvals []*evaluation.IndividualEvaluation,
) (map[string]float64, error) {
	// Collect all unique criterion IDs across all evaluators.
	criterionScoresMap := make(map[string]map[string]float64) // criterionID -> agentName -> score.
	agentWeights := make(map[string]float64)                  // agentName -> weight.

	for _, eval := range individualEvals {
		weight := 1.0
		if w.roleInfo != nil {
			weight = w.roleInfo.EvaluatorWeight(evaluation.EvaluatorKey(eval.AgentID))
		}
		agentWeights[eval.AgentName] = weight

		expectedCriterionIDs, ok := w.roleToCriterionIDs[evaluation.EvaluatorKey(eval.AgentID)]
		if !ok {
			return nil, fmt.Errorf("no criterion IDs configured for role %s", eval.AgentID)
		}

		// Validate that evaluator has scores for ALL expected criteria.
		for _, expectedCriterionID := range expectedCriterionIDs {
			expectedCriterionIDStr := string(expectedCriterionID)
			score, hasScore := eval.CriterionScores[expectedCriterionIDStr]
			if !hasScore {
				return nil, fmt.Errorf("evaluator %s (role %s) is missing score for configured criterion %s - system error requiring retry", eval.AgentName, eval.AgentID, expectedCriterionID)
			}
			// Validate score is not negative (should be 0 or positive).
			if score < 0 {
				return nil, fmt.Errorf("evaluator %s (role %s) has negative score for criterion %s: %f - invalid score", eval.AgentName, eval.AgentID, expectedCriterionID, score)
			}
		}

		// Collect criterion scores for this evaluator.
		for criterionID, score := range eval.CriterionScores {
			if criterionScoresMap[criterionID] == nil {
				criterionScoresMap[criterionID] = make(map[string]float64)
			}
			criterionScoresMap[criterionID][eval.AgentName] = score
		}
	}

	// Validate we have weights.
	if len(agentWeights) == 0 {
		return nil, fmt.Errorf("no valid evaluations with weights - cannot calculate weighted criterion scores")
	}

	allExpectedCriterionIDs := make(map[string]bool)
	for _, eval := range individualEvals {
		expectedCriterionIDs, ok := w.roleToCriterionIDs[evaluation.EvaluatorKey(eval.AgentID)]
		if !ok {
			continue
		}
		for _, criterionID := range expectedCriterionIDs {
			allExpectedCriterionIDs[string(criterionID)] = true
		}
	}

	// Second pass: calculate weighted average for each expected criterion.
	result := make(map[string]float64)
	for expectedCriterionIDStr := range allExpectedCriterionIDs {
		agentScores, hasScores := criterionScoresMap[expectedCriterionIDStr]
		if !hasScores {
			return nil, fmt.Errorf("configured criterion %s has no scores from any evaluator - system error requiring retry", expectedCriterionIDStr)
		}

		var weightedSum float64
		var totalWeight float64

		for agentName, score := range agentScores {
			weight, ok := agentWeights[agentName]
			if !ok {
				return nil, fmt.Errorf("weight not found for agent %s - cannot calculate weighted average for criterion %s", agentName, expectedCriterionIDStr)
			}
			weightedSum += score * weight
			totalWeight += weight
		}

		if totalWeight == 0 {
			return nil, fmt.Errorf("total weight is 0 for criterion %s - cannot calculate weighted average", expectedCriterionIDStr)
		}

		// Calculate weighted average and round to 2 decimal places.
		weightedAvg := math.Round((weightedSum/totalWeight)*100) / 100
		result[expectedCriterionIDStr] = weightedAvg
	}

	return result, nil
}
