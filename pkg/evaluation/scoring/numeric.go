package scoring

import (
	"fmt"
	"strconv"
	"strings"
)

// FloatMapFromCriterionScores coerces a criterion_scores map to float64 values.
func FloatMapFromCriterionScores(scores map[string]interface{}) (map[string]float64, error) {
	if len(scores) == 0 {
		return nil, fmt.Errorf("criterion_scores map is empty")
	}
	out := make(map[string]float64, len(scores))
	for criterionID, scoreValue := range scores {
		score, err := parseNumericCriterionScore(scoreValue)
		if err != nil {
			return nil, fmt.Errorf("invalid score for criterion %s: %w", criterionID, err)
		}
		out[criterionID] = score
	}
	return out, nil
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
			return 0, fmt.Errorf("%q (scores must be plain decimals)", v)
		}
		score, err := strconv.ParseFloat(trimmed, 64)
		if err != nil {
			return 0, fmt.Errorf("%q (scores must be plain decimals)", v)
		}
		return score, nil
	default:
		return 0, fmt.Errorf("score has invalid type %T", scoreValue)
	}
}
