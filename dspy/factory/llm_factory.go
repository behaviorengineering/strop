package factory

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	stropdspy "github.com/behaviorengineering/strop/dspy"
	stroplog "github.com/behaviorengineering/strop/log"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
	"github.com/XiaoConstantine/dspy-go/pkg/llms"
)

// LLMFactory creates LLM instances from provider configurations.
type LLMFactory struct {
	// This is used by OpenInference interceptor to look up provider names.
	onModelCreated func(modelID string, providerType string)
	logger         stroplog.Logger
	moduleTimeout  time.Duration // Default timeout for HTTP clients (defaults to module timeout if provider.Timeout not set).
	instrumentHTTP func(*http.Client)
}

// NewLLMFactory creates a new LLM factory.
func NewLLMFactory(onModelCreated func(modelID string, providerType string), moduleTimeout time.Duration) *LLMFactory {
	return &LLMFactory{
		onModelCreated: onModelCreated,
		moduleTimeout:  moduleTimeout,
	}
}

// SetLogger sets the logger for the LLM factory (optional, for debugging).
func (f *LLMFactory) SetLogger(logger stroplog.Logger) {
	f.logger = logger
}

// SetInstrumentHTTP sets an optional HTTP client instrumenter (app OTEL wrapping).
func (f *LLMFactory) SetInstrumentHTTP(fn func(*http.Client)) {
	if f == nil {
		return
	}
	f.instrumentHTTP = fn
}

// CreateLLM configures and returns an LLM instance based on the provider configuration.
func (f *LLMFactory) CreateLLM(ctx context.Context, provider stropdspy.ProviderConfig) (core.LLM, error) {
	// Configure default LLM.
	llms.EnsureFactory()

	// Determine model ID - trim whitespace and validate it's provided.
	modelName := strings.TrimSpace(provider.Model)
	if modelName == "" {
		return nil, fmt.Errorf("model name is required")
	}
	modelID := core.ModelID(modelName)

	// Determine API schema type - must be explicitly configured.
	apiSchema := provider.APISchema
	if apiSchema == "" {
		return nil, fmt.Errorf("api_schema is required - must be explicitly set in configuration")
	}

	// Resolve provider timeout (defaults to module timeout if not set).
	providerTimeout := provider.GetTimeout(f.moduleTimeout)

	// Configure LLM - always use explicit API schema (apiSchema is guaranteed to be set above).
	registry := core.GetRegistry()
	providerConfig := core.ProviderConfig{
		Name:   apiSchema,
		APIKey: provider.APIKey,
	}

	// For OpenAI-compatible providers (Perplexity, OpenRouter, etc.), configure baseURL and endpoint.
	if apiSchema == "openai" && provider.BaseURL != "" {
		providerConfig.BaseURL = provider.BaseURL
		if providerConfig.Endpoint == nil {
			providerConfig.Endpoint = &core.EndpointConfig{}
		}
		// Default to /chat/completions if not specified.
		if providerConfig.Endpoint.Path == "" {
			providerConfig.Endpoint.Path = "/chat/completions"
		}
		// Set HTTP client timeout (in seconds).
		providerConfig.Endpoint.TimeoutSec = int(providerTimeout.Seconds())
	} else if apiSchema == "google" {
		// For Google AI, configure endpoint timeout if endpoint exists.
		if providerConfig.Endpoint == nil {
			providerConfig.Endpoint = &core.EndpointConfig{}
		}
		providerConfig.Endpoint.TimeoutSec = int(providerTimeout.Seconds())
	}
	// For other providers, set timeout if endpoint is configured.
	if providerConfig.Endpoint != nil && providerConfig.Endpoint.TimeoutSec == 0 {
		providerConfig.Endpoint.TimeoutSec = int(providerTimeout.Seconds())
	}

	llmInstance, err := registry.CreateLLMWithConfig(ctx, providerConfig, modelID)
	if err != nil {
		return nil, fmt.Errorf("failed to create LLM with API schema %s: %w", apiSchema, err)
	}
	f.instrumentLLMHTTPClient(llmInstance)

	// llmInstance = core.NewModelContextDecorator(llmInstance) // ❌ DISABLED - causes empty modelID overwrite.

	// Wrap with grounding wrapper if Google API and grounding is enabled
	if apiSchema == "google" && provider.Grounding != nil {
		// Extract base URL - use provider's baseURL or default
		baseURL := provider.BaseURL
		if baseURL == "" {
			baseURL = "https://generativelanguage.googleapis.com"
		}

		llmInstance = NewGroundingLLMWrapper(
			llmInstance,
			provider.Grounding,
			provider.APIKey,
			baseURL,
			string(modelID),
			providerTimeout,
			f.logger,
		)

		if f.logger != nil {
			f.logger.WithFields(map[string]interface{}{
				"model":             string(modelID),
				"dynamic_threshold": provider.Grounding.DynamicThreshold,
			}).Debug("Wrapped Gemini LLM with Google Search grounding")
		}
	}

	// dspy-go OpenAILLM does not implement GenerateWithContent; wrap OpenAI-compatible
	// providers so multimodal (image_url) requests reach proxies like Polypus.
	if apiSchema == "openai" {
		llmInstance = NewOpenAIVisionLLMWrapper(
			llmInstance,
			provider.APIKey,
			provider.BaseURL,
			string(modelID),
			providerTimeout,
			f.logger,
		)

		if f.logger != nil {
			f.logger.WithFields(map[string]interface{}{
				"model":    string(modelID),
				"base_url": provider.BaseURL,
			}).Debug("Wrapped OpenAI-compatible LLM with multimodal vision support")
		}
	}

	// Wrap with output token limit wrapper if max_output_tokens is configured
	if provider.MaxOutputTokens > 0 {
		llmInstance = NewOutputTokenLimitWrapper(
			llmInstance,
			provider.MaxOutputTokens,
			string(modelID),
			f.logger,
		)

		if f.logger != nil {
			f.logger.WithFields(map[string]interface{}{
				"model":             string(modelID),
				"max_output_tokens": provider.MaxOutputTokens,
			}).Debug("Wrapped LLM with output token limit enforcement")
		}
	}

	// Wrap with logging decorator to ensure model ID is set in ExecutionState.
	if f.logger != nil {
		llmInstance = newLoggingModelContextDecorator(llmInstance, string(modelID), f.logger)
	} else {
		// So we'll create a decorator without logging.
		llmInstance = newLoggingModelContextDecorator(llmInstance, string(modelID), nil)
	}

	// Register model for cost tracking. Local Polypus stays labeled polypus in Phoenix.
	if f.onModelCreated != nil {
		f.onModelCreated(string(modelID), providerNameForTrace(apiSchema, provider.BaseURL))
	}

	return llmInstance, nil
}

type httpClientGetter interface {
	GetHTTPClient() *http.Client
}

func (f *LLMFactory) instrumentLLMHTTPClient(llm core.LLM) {
	if f == nil || f.instrumentHTTP == nil {
		return
	}
	getter, ok := llm.(httpClientGetter)
	if !ok {
		return
	}
	f.instrumentHTTP(getter.GetHTTPClient())
}

func providerNameForTrace(apiSchema, baseURL string) string {
	if apiSchema == "openai" && isPolypusBaseURL(baseURL) {
		return "polypus"
	}
	return apiSchema
}

func isPolypusBaseURL(baseURL string) bool {
	u := strings.ToLower(strings.TrimSpace(baseURL))
	return strings.Contains(u, ":1320") || strings.Contains(u, "polypus")
}

// loggingModelContextDecorator wraps ModelContextDecorator to log what ModelID() returns.
// This helps diagnose why ModelContextDecorator might not be setting the model ID in ExecutionState.
type loggingModelContextDecorator struct {
	core.LLM
	expectedModelID string
	logger          stroplog.Logger
}

// newLoggingModelContextDecorator creates a logging wrapper around ModelContextDecorator.
func newLoggingModelContextDecorator(decorated core.LLM, expectedModelID string, logger stroplog.Logger) core.LLM {
	return &loggingModelContextDecorator{
		LLM:             decorated,
		expectedModelID: expectedModelID,
		logger:          logger,
	}
}

// Generate sets the model ID in ExecutionState before calling the wrapped decorator's Generate().
func (d *loggingModelContextDecorator) Generate(ctx context.Context, prompt string, options ...core.GenerateOption) (*core.LLMResponse, error) {
	// Debug: Log that decorator is being called (DEBUG level to reduce log noise).
	if d.logger != nil {
		d.logger.WithFields(map[string]interface{}{
			"expected_model_id": d.expectedModelID,
		}).Debug("🔧 loggingModelContextDecorator.Generate CALLED")
	}

	// We do this BEFORE calling the wrapped LLM to prevent ModelContextDecorator from overwriting with empty string.
	state := core.GetExecutionState(ctx)
	if d.logger != nil {
		d.logger.WithFields(map[string]interface{}{
			"has_state":         state != nil,
			"expected_model_id": d.expectedModelID,
			"state_ptr":         fmt.Sprintf("%p", state),
		}).Debug("🔧 loggingModelContextDecorator.Generate: checking state")
	}

	if state != nil {
		// Always use expectedModelID since it's guaranteed to be set during LLM creation.
		if d.expectedModelID != "" {
			// Get modelID BEFORE setting it.
			beforeModelID := state.GetModelID()
			// WithModelID() mutates the state in place.
			state.WithModelID(d.expectedModelID)
			// Get modelID AFTER setting it to verify mutation worked.
			afterModelID := state.GetModelID()
			if d.logger != nil {
				d.logger.WithFields(map[string]interface{}{
					"expected_model_id": d.expectedModelID,
					"before_model_id":   beforeModelID,
					"after_model_id":    afterModelID,
					"mutation_worked":   afterModelID == d.expectedModelID,
				}).Debug("🔧 loggingModelContextDecorator.Generate: SET model ID in ExecutionState")
			}
		} else if d.logger != nil {
			d.logger.Warn("loggingModelContextDecorator.Generate: expectedModelID is empty!")
		}
	} else if d.logger != nil {
		d.logger.Warn("🚨 loggingModelContextDecorator.Generate: ExecutionState is nil!")
	}

	// The updated context now has the correct modelID set in ExecutionState.
	return d.LLM.Generate(ctx, prompt, options...)
}

// GenerateWithContent implements the LLM interface and ensures model ID is set.
// ModelContextDecorator doesn't implement this, so calls bypass the decorator.
// This wrapper fixes that by setting the model ID before delegating.
func (d *loggingModelContextDecorator) GenerateWithContent(ctx context.Context, content []core.ContentBlock, options ...core.GenerateOption) (*core.LLMResponse, error) {
	// Always use expectedModelID since it's guaranteed to be set during LLM creation.
	if state := core.GetExecutionState(ctx); state != nil {
		if d.expectedModelID != "" {
			// WithModelID() mutates the state in place.
			state.WithModelID(d.expectedModelID)
		}
	}

	// Call the wrapped LLM's GenerateWithContent() method.
	return d.LLM.GenerateWithContent(ctx, content, options...)
}
