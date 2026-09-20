package evaluation

import (
	"fmt"
	"strings"
	"time"
)

// LabeledEval is one sub-unit evaluation (section, field, item) for aggregation.
type LabeledEval struct {
	Label string
	Eval  *AggregatedEvaluation
}

// AggregateLabeledEvals builds a single AggregatedEvaluation from labeled sub-evals.
// WeightedScore is the mean of scores (or emptyDefaultScore when both inputs are empty).
// CriterionScores and AgentScores are averaged per key; string maps and feedback are joined.
func AggregateLabeledEvals(scores []float64, items []LabeledEval, emptyDefaultScore float64) *AggregatedEvaluation {
	if len(scores) == 0 && len(items) == 0 {
		return &AggregatedEvaluation{
			WeightedScore:   emptyDefaultScore,
			CriterionScores: map[string]float64{},
			AgentScores:     map[string]float64{},
			AgentFeedback:   map[string]string{},
			AgentRationale:  map[string]string{},
		}
	}

	out := &AggregatedEvaluation{
		WeightedScore:   meanFloats(scores),
		CriterionScores: meanFloatMaps(collectCriterionScores(items)),
		AgentScores:     meanFloatMaps(collectAgentScores(items)),
		AgentFeedback:   joinStringMaps(collectAgentFeedback(items)),
		AgentRationale:  joinStringMaps(collectAgentRationale(items)),
	}

	var feedbackParts []string
	var totalTime time.Duration
	for _, s := range items {
		if s.Eval == nil {
			continue
		}
		totalTime += s.Eval.EvaluationTime
		fb := strings.TrimSpace(s.Eval.ConsolidatedFeedback)
		if fb == "" {
			continue
		}
		if id := strings.TrimSpace(s.Label); id != "" {
			feedbackParts = append(feedbackParts, fmt.Sprintf("%s: %s", id, fb))
		} else {
			feedbackParts = append(feedbackParts, fb)
		}
	}
	out.ConsolidatedFeedback = strings.Join(feedbackParts, "\n\n")
	out.EvaluationTime = totalTime
	return out
}

func meanFloats(scores []float64) float64 {
	if len(scores) == 0 {
		return 0
	}
	var sum float64
	for _, s := range scores {
		sum += s
	}
	return sum / float64(len(scores))
}

func collectCriterionScores(items []LabeledEval) []map[string]float64 {
	out := make([]map[string]float64, 0, len(items))
	for _, s := range items {
		if s.Eval != nil && s.Eval.CriterionScores != nil {
			out = append(out, s.Eval.CriterionScores)
		}
	}
	return out
}

func collectAgentScores(items []LabeledEval) []map[string]float64 {
	out := make([]map[string]float64, 0, len(items))
	for _, s := range items {
		if s.Eval != nil && s.Eval.AgentScores != nil {
			out = append(out, s.Eval.AgentScores)
		}
	}
	return out
}

func collectAgentFeedback(items []LabeledEval) []map[string]string {
	out := make([]map[string]string, 0, len(items))
	for _, s := range items {
		if s.Eval != nil && s.Eval.AgentFeedback != nil {
			out = append(out, s.Eval.AgentFeedback)
		}
	}
	return out
}

func collectAgentRationale(items []LabeledEval) []map[string]string {
	out := make([]map[string]string, 0, len(items))
	for _, s := range items {
		if s.Eval != nil && s.Eval.AgentRationale != nil {
			out = append(out, s.Eval.AgentRationale)
		}
	}
	return out
}

func meanFloatMaps(maps []map[string]float64) map[string]float64 {
	sums := make(map[string]float64)
	counts := make(map[string]int)
	for _, m := range maps {
		for k, v := range m {
			sums[k] += v
			counts[k]++
		}
	}
	out := make(map[string]float64, len(sums))
	for k, sum := range sums {
		out[k] = sum / float64(counts[k])
	}
	return out
}

func joinStringMaps(maps []map[string]string) map[string]string {
	parts := make(map[string][]string)
	for _, m := range maps {
		for k, v := range m {
			v = strings.TrimSpace(v)
			if v == "" {
				continue
			}
			parts[k] = append(parts[k], v)
		}
	}
	out := make(map[string]string, len(parts))
	for k, vals := range parts {
		out[k] = strings.Join(vals, "\n")
	}
	return out
}
