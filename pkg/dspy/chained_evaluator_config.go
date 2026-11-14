package dspy

import (
	"fmt"

	"github.com/behaviorengineering/strop/pkg/evaluation"
	"github.com/behaviorengineering/strop/pkg/evaluation/criteria"
	"github.com/behaviorengineering/strop/pkg/evaluation/scoring"

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
	// ScoreBackendFactory optionally replaces the LLM score step for matching roles (e.g. JEV). Nil keeps LLM scoring.
	ScoreBackendFactory func(role evaluation.EvaluatorKey, criterionIDs []criteria.CriterionID, scorePrompt string) (scoring.Backend, error)
	// ScoreThenFeedback runs score generation before feedback analysis (JEV rubric first, then LLM checklist from zeros). Default false.
	ScoreThenFeedback bool
	// ParentModifiers are host-owned parent/leaf packs. Ranked only when ScoreThenFeedback is set and the parent is below max.
	ParentModifiers []criteria.ParentModifiers
	// LeafScoreBackendFactory builds a score backend for dinged-parent leaves. Nil skips leaf ranking.
	LeafScoreBackendFactory func() (scoring.Backend, error)
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
		criterionIDs := criteria.ParseCriterionIDsFromMappingPrompt(prompts.ScoreGenerationPrompt)
		var scoreBackend scoring.Backend
		if config.ScoreBackendFactory != nil {
			backend, factoryErr := config.ScoreBackendFactory(role, criterionIDs, prompts.ScoreGenerationPrompt)
			if factoryErr != nil {
				return nil, fmt.Errorf("create score backend for role %q: %w", role, factoryErr)
			}
			scoreBackend = backend
		}
		mod, err := CreateChainedEvaluatorModuleWithScoreBackend(
			config.Signature,
			role.String(),
			prompts.FeedbackAnalysisPrompt,
			prompts.ScoreGenerationPrompt,
			config.Persona,
			formatter,
			scoreBackend,
			config.ScoreThenFeedback,
		)
		if err != nil {
			return nil, fmt.Errorf("create chained evaluator for role %q: %w", role, err)
		}
		if config.ScoreThenFeedback && len(config.ParentModifiers) > 0 && config.LeafScoreBackendFactory != nil {
			leafBackend, leafErr := config.LeafScoreBackendFactory()
			if leafErr != nil {
				return nil, fmt.Errorf("create leaf score backend for role %q: %w", role, leafErr)
			}
			mod.SetParentLeafRank(config.ParentModifiers, leafBackend)
		}
		out[role] = mod
	}
	return out, nil
}
