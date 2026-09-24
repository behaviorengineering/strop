package workflow

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	stropdspy "github.com/behaviorengineering/strop/pkg/dspy"
	"github.com/behaviorengineering/strop/pkg/dspy/rawresponse"
	"github.com/behaviorengineering/strop/pkg/evaluation"
	"github.com/behaviorengineering/strop/pkg/evaluation/criteria"
)

func getMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// parseCriterionScores parses criterion scores from a map[string]interface{}.
// Returns a map of criterion ID to score.
func parseCriterionScores(value interface{}, expectedCriterionIDs map[string]struct{}) (map[string]float64, error) {
	if value == nil {
		return nil, fmt.Errorf("criterion_scores is nil")
	}

	scoresMap, err := stropdspy.CoerceCriterionScoresMap(value)
	if err != nil {
		return nil, err
	}

	result := make(map[string]float64, len(scoresMap))
	for criterionID, scoreValue := range scoresMap {
		if len(expectedCriterionIDs) > 0 {
			if _, expected := expectedCriterionIDs[criterionID]; !expected {
				// Ignore unexpected keys so malformed extra fields do not fail the whole parse.
				continue
			}
		}

		score, err := parseNumericCriterionScore(scoreValue)
		if err != nil {
			return nil, fmt.Errorf("invalid score format for criterion %s: %w", criterionID, err)
		}
		result[criterionID] = score
	}

	return result, nil
}

func parseNumericCriterionScore(scoreValue interface{}) (float64, error) {
	switch v := scoreValue.(type) {
	case float64:
		return v, nil
	case float32:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case int32:
		return float64(v), nil
	case int:
		return float64(v), nil
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return 0, fmt.Errorf("%q (scores must be plain decimals like 0.0, 1.0, 2.0; not checklist marks)", v)
		}
		score, err := strconv.ParseFloat(trimmed, 64)
		if err != nil {
			return 0, fmt.Errorf("%q (scores must be plain decimals like 0.0, 1.0, 2.0; not checklist marks)", v)
		}
		return score, nil
	default:
		return 0, fmt.Errorf("score has invalid type: %T (expected float64, int64, int, or string)", scoreValue)
	}
}

// normalizeScoreFromCriteria calculates a normalized total score (0-10) from individual criterion scores.
// It uses the criterion registry to determine the maximum possible score for each criterion.
func normalizeScoreFromCriteria(criterionScores map[string]float64, criterionIDs []criteria.CriterionID) (float64, error) {
	if len(criterionScores) == 0 {
		return 0.0, fmt.Errorf("criterion_scores is empty")
	}

	// Calculate actual score and max possible score using shared registry.
	var actualScore float64
	var maxPossibleScore float64

	for _, criterionID := range criterionIDs {
		criterionIDStr := string(criterionID)
		score, hasScore := criterionScores[criterionIDStr]
		if !hasScore {
			// If a criterion is configured, it MUST have a score - this is a system error requiring retry.
			return 0.0, fmt.Errorf("criterion %s is configured but missing from evaluator output - system error requiring retry", criterionID)
		}

		// Get max points for this criterion.
		criterion, err := criterionRegistry.Get(criterionID)
		if err != nil {
			return 0.0, fmt.Errorf("failed to get criterion %s: %w", criterionID, err)
		}

		actualScore += score
		maxPossibleScore += criterion.MaxPoints
	}

	if maxPossibleScore == 0 {
		return 0.0, fmt.Errorf("max possible score is 0")
	}

	// Normalize to 0-10 range: (actual / max) * 10.
	normalizedScore := (actualScore / maxPossibleScore) * 10.0

	// Round to 2 decimal places to match database DECIMAL(4,2) precision.
	return math.Round(normalizedScore*100) / 100, nil
}

// sendOrCancel attempts to send a value to a channel, respecting context cancellation.

func (w *ParallelEvaluationWorkflow) parseEvaluationResult(
	roleKey evaluation.EvaluatorKey,
	result map[string]interface{},
) (*evaluation.IndividualEvaluation, error) {
	if w.roleInfo != nil && !w.roleInfo.HasEvaluator(roleKey) {
		return nil, fmt.Errorf("unknown agent role: %s", roleKey)
	}

	// Extract criterion_scores - must be present at top level.
	criterionScoresValue, exists := result[w.fieldNames.CriterionScores]
	if !exists || criterionScoresValue == nil {
		return nil, fmt.Errorf("criterion_scores field is missing or nil in evaluator result (evaluator: %s)", roleKey)
	}

	criterionIDs, ok := w.roleToCriterionIDs[roleKey]
	if !ok {
		return nil, fmt.Errorf("no criterion IDs configured for role %s", roleKey)
	}
	expectedCriterionIDs := make(map[string]struct{}, len(criterionIDs))
	for _, criterionID := range criterionIDs {
		expectedCriterionIDs[string(criterionID)] = struct{}{}
	}

	criterionScores, err := parseCriterionScores(criterionScoresValue, expectedCriterionIDs)
	if err != nil {
		// Log raw LLM response for debugging when parsing fails (key spelling varies by path).
		if rawResponseStr, rawKey := rawresponse.TextFrom(result); rawKey != "" {
			// Truncate for logging if too long (increased limit for debugging long XML documents).
			if len(rawResponseStr) > 20000 {
				rawResponseStr = rawResponseStr[:20000] + "..."
			}
			w.logger.WithFields(map[string]interface{}{
				"evaluator":              roleKey,
				"criterion_scores":       criterionScoresValue,
				"raw_response_key":       rawKey,
				"__raw_response_preview": rawResponseStr,
			}).Error("Failed to parse criterion_scores - logging raw response for debugging")
		}
		return nil, fmt.Errorf("failed to parse criterion_scores for evaluator %s: %w", roleKey, err)
	}

	// Calculate normalized total score from criterion scores.
	normalizedScore, err := normalizeScoreFromCriteria(criterionScores, criterionIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to normalize score for evaluator %s: %w", roleKey, err)
	}

	// Extract feedback - must be present at top level.
	feedback, ok := result[w.fieldNames.Feedback].(string)
	if !ok || feedback == "" {
		return nil, fmt.Errorf("feedback field is missing or empty in evaluator result (evaluator: %s)", roleKey)
	}

	rationale, err := stropdspy.ExtractRequiredReasoningField(result)
	if err != nil {
		return nil, fmt.Errorf("directives_ack field is missing or empty in evaluator result (evaluator: %s): %w", roleKey, err)
	}

	agentName := roleKey.String()
	if w.roleInfo != nil {
		agentName = w.roleInfo.EvaluatorName(roleKey)
	}
	return &evaluation.IndividualEvaluation{
		AgentID:         roleKey.String(),
		AgentName:       agentName,
		Score:           normalizedScore,
		CriterionScores: criterionScores,
		Feedback:        feedback,
		Rationale:       rationale,
	}, nil
}
