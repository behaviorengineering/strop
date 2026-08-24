package dspy

import (
	"fmt"

	"github.com/behaviorengineering/strop/evaluation"
	"github.com/behaviorengineering/strop/evaluation/criteria"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
)

// ChainedEvaluatorRolePrompts holds the two prompts for a chained evaluator (feedback analysis then score generation).
type ChainedEvaluatorRolePrompts struct {
	FeedbackAnalysisPrompt string
	ScoreGenerationPrompt  string
}

// ChainedEvaluatorConfig is the shared configuration for chained evaluators (feedback -> score).
// Pipelines fill this and use CreateChainedModulesFromConfig to build modules.
type ChainedEvaluatorConfig struct {
	Signature    core.Signature
	RolePrompts  map[evaluation.EvaluatorKey]ChainedEvaluatorRolePrompts
	Persona      string                 // Optional: how the evaluator should behave; rendered first in both feedback and score when non-empty
	CriterionIDs []criteria.CriterionID // Optional; used by workflows for role-to-criteria mapping. Can be nil.
}

// CreateChainedModulesFromConfig builds a chained evaluator module for each role in config.RolePrompts.
// Modules are not yet configured (no LLM/interceptors); use the factory's CreateChainedEvaluatorsFromConfig to create and setup.
func CreateChainedModulesFromConfig(
	config *ChainedEvaluatorConfig,
	formatter EvaluatorSignatureFormatter,
) (map[evaluation.EvaluatorKey]core.Module, error) {
	if config == nil {
		return nil, fmt.Errorf("ChainedEvaluatorConfig is required")
	}
	if len(config.RolePrompts) == 0 {
		return nil, fmt.Errorf("ChainedEvaluatorConfig.RolePrompts cannot be empty")
	}
	out := make(map[evaluation.EvaluatorKey]core.Module, len(config.RolePrompts))
	for role, prompts := range config.RolePrompts {
		mod, err := CreateChainedEvaluatorModule(
			config.Signature,
			role.String(),
			prompts.FeedbackAnalysisPrompt,
			prompts.ScoreGenerationPrompt,
			config.Persona,
			formatter,
		)
		if err != nil {
			return nil, fmt.Errorf("create chained evaluator for role %q: %w", role, err)
		}
		out[role] = mod
	}
	return out, nil
}
