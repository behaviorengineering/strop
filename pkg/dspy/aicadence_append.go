package dspy

import (
	"github.com/behaviorengineering/strop/pkg/evaluation"
	"github.com/behaviorengineering/strop/pkg/evaluation/aicadence"
	"github.com/behaviorengineering/strop/pkg/evaluation/criteria"
)

// AppendAICadenceEvaluator adds the portable ai_cadence chained role to config.
// Reader-facing prose jobs SHOULD call this; vernacular-aphorism and structural
// jobs SHOULD NOT. Prefer a cheap model via CreateChainedEvaluatorsFromConfig roleProviders.
func AppendAICadenceEvaluator(config *ChainedEvaluatorConfig) {
	if config == nil {
		return
	}
	if config.RolePrompts == nil {
		config.RolePrompts = make(map[evaluation.EvaluatorKey]ChainedEvaluatorRolePrompts)
	}
	config.RolePrompts[aicadence.EvaluatorKey] = ChainedEvaluatorRolePrompts{
		FeedbackAnalysisPrompt: aicadence.FeedbackAnalysisPrompt(),
		ScoreGenerationPrompt:  aicadence.ScoreGenerationPrompt(),
	}
	ids := aicadence.CriterionIDs()
	config.CriterionIDs = mergeCriterionIDs(config.CriterionIDs, ids)
}

// AICadenceRoleToCriterionIDs returns the role→criteria map entry for ai_cadence.
func AICadenceRoleToCriterionIDs() map[evaluation.EvaluatorKey][]criteria.CriterionID {
	return map[evaluation.EvaluatorKey][]criteria.CriterionID{
		aicadence.EvaluatorKey: aicadence.CriterionIDs(),
	}
}

func mergeCriterionIDs(base, extra []criteria.CriterionID) []criteria.CriterionID {
	seen := make(map[criteria.CriterionID]struct{}, len(base)+len(extra))
	out := make([]criteria.CriterionID, 0, len(base)+len(extra))
	for _, id := range base {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	for _, id := range extra {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
