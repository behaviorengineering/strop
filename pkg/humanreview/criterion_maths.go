package humanreview

import (
	"encoding/json"
	"fmt"
)

// ParseFeedbackCriterionScores extracts criterion_scores from a history entry's Feedback JSON.
// Returns nil map if feedback has no criterion_scores or parse fails (caller can treat as no data).
func ParseFeedbackCriterionScores(feedbackJSON string) (map[string]float64, error) {
	if feedbackJSON == "" {
		return nil, nil
	}
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(feedbackJSON), &raw); err != nil {
		return nil, fmt.Errorf("parse feedback JSON: %w", err)
	}
	v, ok := raw["criterion_scores"]
	if !ok || v == nil {
		return nil, nil
	}
	switch m := v.(type) {
	case map[string]interface{}:
		out := make(map[string]float64, len(m))
		for k, val := range m {
			switch n := val.(type) {
			case float64:
				out[k] = n
			default:
				continue
			}
		}
		return out, nil
	default:
		return nil, nil
	}
}

// CriterionMathsResult holds the computed score and performance metrics for one criterion.
type CriterionMathsResult struct {
	ProposedScore           float64
	FirstVersionAtMax       *int
	VersionsWithImprovement int
	SmellPassedAtV1         bool
	PerformanceSummary      string
}

// ComputeCriterionFromVersionScores computes proposed score and performance metrics from per-version scores.
// versionScores[i] is the criterion_scores map for version i+1 (1-based).
func ComputeCriterionFromVersionScores(criterionID string, maxPoints float64, versionScores []map[string]float64) CriterionMathsResult {
	var result CriterionMathsResult
	if maxPoints <= 0 {
		maxPoints = 2.0
	}
	if len(versionScores) == 0 {
		result.PerformanceSummary = "No version data"
		return result
	}

	last := versionScores[len(versionScores)-1]
	if score, ok := last[criterionID]; ok {
		result.ProposedScore = clampScore(score, 0, maxPoints)
	}

	var firstAtMax *int
	for i, m := range versionScores {
		score, ok := m[criterionID]
		if !ok {
			continue
		}
		if score >= maxPoints {
			v := i + 1
			firstAtMax = &v
			break
		}
	}

	result.FirstVersionAtMax = firstAtMax
	if firstAtMax != nil {
		result.VersionsWithImprovement = *firstAtMax - 1
		if *firstAtMax <= 1 {
			result.SmellPassedAtV1 = true
		}
	}

	switch {
	case result.SmellPassedAtV1:
		result.PerformanceSummary = "Passed at v1 (possible rubric smell)"
	case firstAtMax != nil && result.VersionsWithImprovement > 0:
		result.PerformanceSummary = fmt.Sprintf("Met at v%d, improved over %d versions", *firstAtMax, result.VersionsWithImprovement)
	case firstAtMax != nil:
		result.PerformanceSummary = fmt.Sprintf("Met at v%d", *firstAtMax)
	default:
		result.PerformanceSummary = "Never reached max in pipeline"
	}

	return result
}

func clampScore(score, min, max float64) float64 {
	if score < min {
		return min
	}
	if score > max {
		return max
	}
	return score
}
