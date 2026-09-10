package factory

import (
	"context"
	"fmt"

	stropdspy "github.com/behaviorengineering/strop/dspy"

	dspyrlm "github.com/XiaoConstantine/dspy-go/pkg/modules/rlm"
)

// CreateRLM builds a Recursive Language Model from a provider config.
// The module explores large context via a Go REPL; hosts pass context and query to Complete.
func (f *GeneratorFactory) CreateRLM(
	ctx context.Context,
	provider stropdspy.ProviderConfig,
	cfg stropdspy.RLMConfig,
	errorPrefix string,
) (*dspyrlm.RLM, error) {
	if f == nil || f.configurator == nil {
		return nil, fmt.Errorf("%s: generator factory is nil", errorPrefix)
	}
	if err := provider.Validate(); err != nil {
		return nil, fmt.Errorf(ErrInvalidProviderConfig, err)
	}
	llm, err := f.configurator.llmFactory.CreateLLM(ctx, provider)
	if err != nil {
		return nil, fmt.Errorf("%s: create LLM: %w", errorPrefix, err)
	}
	module, err := stropdspy.CreateRLMModule(llm, cfg)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", errorPrefix, err)
	}
	return module, nil
}
