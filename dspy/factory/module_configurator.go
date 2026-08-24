package factory

import (
	"context"
	"fmt"
	stroplog "github.com/behaviorengineering/strop/log"

	stropdspy "github.com/behaviorengineering/strop/dspy"
	dspymodules "github.com/behaviorengineering/strop/dspy/modules"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
)

// ModuleConfigurator encapsulates generic module setup patterns (LLM, interceptors, XML output).
// This is the foundation that all other factories use for module configuration.
type ModuleConfigurator struct {
	llmFactory       *LLMFactory
	interceptorSetup *InterceptorSetup
	logger           stroplog.Logger
}

// NewModuleConfigurator creates a new module configurator.
func NewModuleConfigurator(
	llmFactory *LLMFactory,
	interceptorSetup *InterceptorSetup,
	logger stroplog.Logger,
) *ModuleConfigurator {
	if llmFactory == nil {
		panic("llmFactory cannot be nil")
	}
	if interceptorSetup == nil {
		panic("interceptorSetup cannot be nil")
	}
	return &ModuleConfigurator{
		llmFactory:       llmFactory,
		interceptorSetup: interceptorSetup,
		logger:           logger,
	}
}

// SetupModule configures a module with LLM, XML output, and interceptors.
// Accepts DirectivesCoT or bare Predict.
func (c *ModuleConfigurator) SetupModule(
	ctx context.Context,
	provider stropdspy.ProviderConfig,
	module core.Module,
	errorPrefix string,
) error {
	if err := provider.Validate(); err != nil {
		return fmt.Errorf(ErrInvalidProviderConfig, err)
	}

	predict, err := dspymodules.PredictOf(module)
	if err != nil {
		return fmt.Errorf("%s: %w", errorPrefix, err)
	}
	interceptable, err := dspymodules.AsInterceptable(module)
	if err != nil {
		return fmt.Errorf("%s: %w", errorPrefix, err)
	}

	// Create LLM instance.
	llmInstance, err := c.llmFactory.CreateLLM(ctx, provider)
	if err != nil {
		return fmt.Errorf(ErrLLMSetupFailed, errorPrefix, err)
	}

	switch m := module.(type) {
	case *dspymodules.DirectivesCoT:
		m.SetLLM(llmInstance)
	default:
		predict.LLM = llmInstance
	}

	if c.logger != nil {
		c.logger.WithFields(map[string]interface{}{
			"module":   errorPrefix,
			"llm_type": fmt.Sprintf("%T", llmInstance),
		}).Debug("🔧 Module setup: LLM type set on Predict.LLM")
	}

	existingInterceptors := interceptable.GetInterceptors()
	xmlAlreadyEnabled := predict.IsXMLModeEnabled()

	var xmlInterceptor core.ModuleInterceptor
	if xmlAlreadyEnabled && len(existingInterceptors) > 0 {
		xmlInterceptor = existingInterceptors[len(existingInterceptors)-1]
		existingInterceptors = existingInterceptors[:len(existingInterceptors)-1]
		interceptable.SetInterceptors(existingInterceptors)
	}

	c.interceptorSetup.AddInterceptors(interceptable, provider)

	if xmlAlreadyEnabled && xmlInterceptor != nil {
		c.interceptorSetup.EnableStructuredOutput(interceptable, predict)
	} else if !xmlAlreadyEnabled {
		c.interceptorSetup.EnableStructuredOutput(interceptable, predict)
	}

	if c.interceptorSetup.logger != nil {
		interceptors := interceptable.GetInterceptors()
		c.interceptorSetup.logger.WithFields(map[string]interface{}{
			"module":            errorPrefix,
			"interceptor_count": len(interceptors),
			"xml_enabled":       predict.IsXMLModeEnabled(),
		}).Debug("Module setup complete - interceptors configured")
	}

	return nil
}

// SetupChainedModule configures a chained module by setting up both internal modules.
// Chained modules contain two internal modules (e.g., feedback analysis + score generation).
func (c *ModuleConfigurator) SetupChainedModule(
	ctx context.Context,
	provider stropdspy.ProviderConfig,
	chainedModule core.Module,
	errorPrefix string,
) error {
	// Try to cast to interface that exposes internal modules.
	if cm, ok := chainedModule.(interface {
		GetFeedbackAnalysisModule() core.Module
		GetScoreGenerationModule() core.Module
	}); ok {
		feedbackAnalysisModule := cm.GetFeedbackAnalysisModule()
		if err := c.SetupModule(ctx, provider, feedbackAnalysisModule, errorPrefix+" - Feedback Analysis"); err != nil {
			return err
		}

		if err := c.SetupModule(ctx, provider, cm.GetScoreGenerationModule(), errorPrefix+" - Score Generation"); err != nil {
			return err
		}
		return nil
	}
	return fmt.Errorf("chained module does not implement required interface for setup")
}
