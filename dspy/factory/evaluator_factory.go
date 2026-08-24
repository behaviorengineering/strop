package factory

import (
	"context"
	"errors"
	"fmt"

	stropdspy "github.com/behaviorengineering/strop/dspy"
	dspymodules "github.com/behaviorengineering/strop/dspy/modules"
	"github.com/behaviorengineering/strop/evaluation"
	"github.com/behaviorengineering/strop/evaluation/criteria"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
	"github.com/XiaoConstantine/dspy-go/pkg/modules"
)

// EvaluatorFactory creates evaluator modules with any role type using Go generics.
// This allows the same factory to work with sayings, videos, or any future pipeline's role types.
type EvaluatorFactory struct {
	configurator *ModuleConfigurator
}

// NewEvaluatorFactory creates a new evaluator factory.
func NewEvaluatorFactory(configurator *ModuleConfigurator) *EvaluatorFactory {
	if configurator == nil {
		panic("configurator cannot be nil")
	}
	return &EvaluatorFactory{
		configurator: configurator,
	}
}

// CreateEvaluators creates evaluator modules for multiple roles.
// Returns a map of role to module.
//
// R is a generic role type that must be comparable (for use as map key).
// The pipeline provides:
//   - roles: List of roles to create modules for
//   - getRoleName: Function to get display name for a role
//   - rolePrompts: Map of role to system prompt
//   - createModule: Function to create a module for a role name and prompt
func CreateEvaluators[R comparable](
	f *EvaluatorFactory,
	ctx context.Context,
	provider stropdspy.ProviderConfig,
	roles []R,
	getRoleName func(R) string,
	rolePrompts map[R]string,
	createModule func(roleName string, systemPrompt string) (core.Module, error),
	errorPrefix string,
) (map[R]core.Module, error) {
	modulesMap := make(map[R]core.Module)

	for _, role := range roles {
		// Get role name using pipeline-provided function.
		roleName := getRoleName(role)

		// Get prompt from pipeline-provided map.
		prompt, ok := rolePrompts[role]
		if !ok {
			return nil, fmt.Errorf(ErrNoPromptForRole, roleName)
		}

		// Create module using pipeline-provided function.
		module, err := createModule(roleName, prompt)
		if err != nil {
			return nil, fmt.Errorf(ErrModuleCreationForRole, roleName, err)
		}

		// Note: All evaluators for a task share the same LLM instance.
		// Setup module (LLM, XML output, interceptors).
		if err := f.configurator.SetupModule(ctx, provider, module, errorPrefix); err != nil {
			return nil, err
		}

		modulesMap[role] = module
	}

	return modulesMap, nil
}

// ChainedEvaluatorPrompts contains prompts for both feedback analysis and score generation.
type ChainedEvaluatorPrompts struct {
	FeedbackAnalysisPrompt string
	ScoreGenerationPrompt  string
}

// CreateChainedEvaluators creates chained evaluator modules for multiple roles.
// Returns a map of role to module (core.Module to support chained modules).
// If roleProviders is provided, it overrides the default provider for specific roles.
//
// R is a generic role type that must be comparable (for use as map key).
// The pipeline provides:
//   - roles: List of roles to create modules for
//   - getRoleName: Function to get display name for a role
//   - rolePrompts: Map of role to prompts (feedback + score)
//   - createChainedModule: Function to create a chained module
func CreateChainedEvaluators[R comparable](
	f *EvaluatorFactory,
	ctx context.Context,
	provider stropdspy.ProviderConfig, // Default provider
	roleProviders map[R]stropdspy.ProviderConfig, // Optional per-role overrides
	roles []R,
	getRoleName func(R) string,
	rolePrompts map[R]ChainedEvaluatorPrompts,
	createChainedModule func(signature core.Signature, roleName string, feedbackPrompt string, scorePrompt string) (core.Module, error),
	signature core.Signature,
	criterionIDs []criteria.CriterionID,
	errorPrefix string,
) (map[R]core.Module, error) {
	modulesMap := make(map[R]core.Module)

	for _, role := range roles {
		// Get role name using pipeline-provided function.
		roleName := getRoleName(role)

		// Get prompts from pipeline-provided map.
		prompts, ok := rolePrompts[role]
		if !ok {
			return nil, fmt.Errorf(ErrNoPromptForRole, roleName)
		}

		// Create chained module using pipeline-provided function.
		chainedModule, err := createChainedModule(signature, roleName, prompts.FeedbackAnalysisPrompt, prompts.ScoreGenerationPrompt)
		if err != nil {
			return nil, fmt.Errorf(ErrModuleCreationForRole, roleName, err)
		}

		// Use role-specific provider if available, otherwise use default.
		roleProvider := provider
		if roleProviders != nil {
			if overrideProvider, hasOverride := roleProviders[role]; hasOverride {
				roleProvider = overrideProvider
			}
		}

		// Setup chained module with role-specific provider.
		if err := f.configurator.SetupChainedModule(ctx, roleProvider, chainedModule, errorPrefix); err != nil {
			return nil, err
		}

		modulesMap[role] = chainedModule
	}

	return modulesMap, nil
}

// CreateChainedEvaluatorsFromConfig creates and configures chained evaluator modules from a shared config.
// roleProviders can be nil; otherwise it overrides the default provider for specific evaluator keys.
func CreateChainedEvaluatorsFromConfig(
	f *EvaluatorFactory,
	ctx context.Context,
	provider stropdspy.ProviderConfig,
	roleProviders map[evaluation.EvaluatorKey]stropdspy.ProviderConfig,
	config *stropdspy.ChainedEvaluatorConfig,
	formatter stropdspy.EvaluatorSignatureFormatter,
	errorPrefix string,
) (map[evaluation.EvaluatorKey]core.Module, error) {
	if config == nil {
		return nil, errors.New("ChainedEvaluatorConfig is required")
	}
	modulesMap, err := stropdspy.CreateChainedModulesFromConfig(config, formatter)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", errorPrefix, err)
	}
	for role, chainedModule := range modulesMap {
		roleProvider := provider
		if roleProviders != nil {
			if override, ok := roleProviders[role]; ok {
				roleProvider = override
			}
		}
		if err := f.configurator.SetupChainedModule(ctx, roleProvider, chainedModule, errorPrefix); err != nil {
			return nil, fmt.Errorf("setup chained evaluator %q: %w", role, err)
		}
	}
	return modulesMap, nil
}

// SetupChainedModule configures a chained evaluator module (LLM, XML output, interceptors)
// for both internal modules. Use this when creating chained modules outside of
// CreateChainedEvaluators (e.g. YouTube structural evaluators).
func (f *EvaluatorFactory) SetupChainedModule(
	ctx context.Context,
	provider stropdspy.ProviderConfig,
	chainedModule core.Module,
	errorPrefix string,
) error {
	if f == nil || f.configurator == nil {
		return errors.New("evaluator factory or configurator is nil")
	}
	return f.configurator.SetupChainedModule(ctx, provider, chainedModule, errorPrefix)
}

// CreateConsolidator creates a consolidator module for feedback merging.
//
// R is a generic role type that must be comparable.
// The pipeline provides:
//   - consolidatorRole: The role to use for the consolidator
//   - getRoleName: Function to get display name for a role
//   - consolidatorPrompt: System prompt for the consolidator
//   - persona: Optional; how the consolidator should behave, rendered first when non-empty
//   - createModule: Function to create a module for a role name, prompt, and persona
func CreateConsolidator[R comparable](
	f *EvaluatorFactory,
	ctx context.Context,
	provider stropdspy.ProviderConfig,
	consolidatorRole R,
	getRoleName func(R) string,
	consolidatorPrompt string,
	persona string,
	createModule func(roleName string, systemPrompt string, persona string) (core.Module, error),
	errorPrefix string,
) (*modules.Predict, error) {
	if consolidatorPrompt == "" {
		return nil, errors.New(ErrConsolidatorPromptRequired)
	}

	roleName := getRoleName(consolidatorRole)

	module, err := createModule(roleName, consolidatorPrompt, persona)
	if err != nil {
		return nil, fmt.Errorf(ErrModuleCreationFailed, errorPrefix, err)
	}

	// Setup module (LLM, XML output, interceptors).
	if err := f.configurator.SetupModule(ctx, provider, module, errorPrefix); err != nil {
		return nil, err
	}

	predict, err := dspymodules.PredictOf(module)
	if err != nil {
		return nil, fmt.Errorf(ErrModuleCreationFailed, errorPrefix, err)
	}

	return predict, nil
}
