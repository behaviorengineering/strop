package dspy

import (
	"github.com/behaviorengineering/strop/pkg/evaluation"
	"github.com/behaviorengineering/strop/pkg/evaluation/criteria"
	"github.com/behaviorengineering/strop/pkg/evaluation/voice"
)

// AppendVoiceEvaluator adds the portable voice_fidelity chained role to config.
// Polish jobs that modulate voice SHOULD call this after a neutral composition.
// Prefer a cheap model via CreateChainedEvaluatorsFromConfig roleProviders.
// An empty profile still registers the role and scores staccato runs.
func AppendVoiceEvaluator(config *ChainedEvaluatorConfig, profile voice.Profile) {
	if config == nil {
		return
	}
	if config.RolePrompts == nil {
		config.RolePrompts = make(map[evaluation.EvaluatorKey]ChainedEvaluatorRolePrompts)
	}
	config.RolePrompts[voice.EvaluatorKey] = ChainedEvaluatorRolePrompts{
		FeedbackAnalysisPrompt: voice.FeedbackAnalysisPrompt(profile),
		ScoreGenerationPrompt:  voice.ScoreGenerationPrompt(profile),
	}
	config.CriterionIDs = mergeCriterionIDs(config.CriterionIDs, voice.CriterionIDs())
}

// VoiceRoleToCriterionIDs returns the role to criteria map entry for voice fidelity.
func VoiceRoleToCriterionIDs() map[evaluation.EvaluatorKey][]criteria.CriterionID {
	return map[evaluation.EvaluatorKey][]criteria.CriterionID{
		voice.EvaluatorKey: voice.CriterionIDs(),
	}
}
