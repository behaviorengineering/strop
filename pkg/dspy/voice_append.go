package dspy

import (
	"github.com/behaviorengineering/strop/pkg/evaluation"
	"github.com/behaviorengineering/strop/pkg/evaluation/criteria"
	"github.com/behaviorengineering/strop/pkg/evaluation/voice"
)

// AppendVoiceEvaluator adds the portable voice_fidelity chained role to config.
// The role prompt reads generator_input.voice_profile at evaluation time.
// profile is a fallback only when that field is empty. Pass an empty profile
// for jobs that stay voice-neutral unless the caller supplies voice_profile.
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
