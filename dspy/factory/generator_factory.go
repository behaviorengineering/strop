package factory

import (
	"context"
	"fmt"

	kitdspy "github.com/behaviorengineering/strop/dspy"
	kitvalidation "github.com/behaviorengineering/strop/dspy/validation"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
)

// GeneratorFactory creates generator modules (translation, explanation, etc.).
// Generators don't have roles, so this factory is simpler than evaluator/feedback factories.
type GeneratorFactory struct {
	configurator *ModuleConfigurator
}

// NewGeneratorFactory creates a new generator factory.
func NewGeneratorFactory(configurator *ModuleConfigurator) *GeneratorFactory {
	if configurator == nil {
		panic("configurator cannot be nil")
	}
	return &GeneratorFactory{
		configurator: configurator,
	}
}

// CreateGenerator creates a generator module from a provider config and module factory.
// The validator parameter is optional - if provided, it will be used to create an input processor
// for field validation (e.g., token limits, language codes).
//
// moduleFactory is a function that creates a generator module (DirectivesCoT).
func (f *GeneratorFactory) CreateGenerator(
	ctx context.Context,
	provider kitdspy.ProviderConfig,
	moduleFactory func() (core.Module, error),
	errorPrefix string,
	validator func(kitdspy.ProviderConfig) kitvalidation.InputProcessor, // Optional
) (core.Module, error) {
	// Create module using factory function.
	module, err := moduleFactory()
	if err != nil {
		return nil, fmt.Errorf(ErrModuleCreationFailed, errorPrefix, err)
	}

	// If validator is provided, temporarily override the interceptor setup's input processor factory.
	// This allows pipeline-specific validation (e.g., "saying_context" vs "video_transcript").
	// Intentional: container init is single-threaded and we restore the original factory after
	// SetupModule, so the shared InterceptorSetup is not left in a modified state.
	// Optional future refactor: add an optional parameter to AddInterceptors to accept a
	// per-call input processor factory instead of mutating the shared instance.
	originalInputProcessorFactory := f.configurator.interceptorSetup.inputProcessorFactory
	if validator != nil {
		// Temporarily set the validator as the input processor factory.
		f.configurator.interceptorSetup.inputProcessorFactory = validator
	}

	// Setup module (LLM, XML output, interceptors).
	// The validator (if provided) will be used by AddInterceptors to create an input processing interceptor.
	setupErr := f.configurator.SetupModule(ctx, provider, module, errorPrefix)

	// Always restore original factory, even if setup failed.
	f.configurator.interceptorSetup.inputProcessorFactory = originalInputProcessorFactory

	if setupErr != nil {
		return nil, setupErr
	}

	return module, nil
}
