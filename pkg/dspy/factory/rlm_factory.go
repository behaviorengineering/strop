package factory

import (
	"context"
	"fmt"
	"strings"

	stropdspy "github.com/behaviorengineering/strop/pkg/dspy"
	"github.com/behaviorengineering/strop/pkg/openaibatch"

	dspyrlm "github.com/XiaoConstantine/dspy-go/pkg/modules/rlm"
)

// CreateRLM builds a Recursive Language Model from a provider config.
// Resolves the LLM, sets it on cfg, then calls cfg.CreateModule (config create).
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
	cfg.BatchModel = strings.TrimSpace(provider.Model)
	if strings.EqualFold(strings.TrimSpace(provider.APISchema), "openai") {
		root, err := openaibatch.GatewayRoot(provider.BaseURL)
		if err == nil {
			cfg.OpenAIBatch.BaseURL = root
			cfg.OpenAIBatch.APIKey = provider.APIKey
		}
	}
	llm, err := f.configurator.llmFactory.CreateLLM(ctx, provider)
	if err != nil {
		return nil, fmt.Errorf("%s: create LLM: %w", errorPrefix, err)
	}
	cfg.LLM = llm
	module, err := cfg.CreateModule()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", errorPrefix, err)
	}
	return module, nil
}
