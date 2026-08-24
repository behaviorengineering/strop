package factory

import (
	"context"
	"fmt"

	"github.com/behaviorengineering/strop/dspy/validation"
	kitlog "github.com/behaviorengineering/strop/log"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
)

// OutputTokenLimitWrapper wraps an LLM and enforces output token limits from provider config.
// It injects max_tokens into GenerateOptions and validates response token counts.
type OutputTokenLimitWrapper struct {
	core.LLM
	maxOutputTokens int
	modelID         string
	logger          kitlog.Logger
}

// NewOutputTokenLimitWrapper creates a new wrapper that enforces output token limits.
// If maxOutputTokens is 0, no limit is enforced (provider default applies).
func NewOutputTokenLimitWrapper(
	wrapped core.LLM,
	maxOutputTokens int,
	modelID string,
	logger kitlog.Logger,
) *OutputTokenLimitWrapper {
	return &OutputTokenLimitWrapper{
		LLM:             wrapped,
		maxOutputTokens: maxOutputTokens,
		modelID:         modelID,
		logger:          logger,
	}
}

// Generate intercepts Generate calls and enforces output token limits.
func (w *OutputTokenLimitWrapper) Generate(ctx context.Context, prompt string, options ...core.GenerateOption) (*core.LLMResponse, error) {
	// Inject max_tokens if configured
	options = w.injectMaxTokens(options)

	// Call wrapped LLM
	response, err := w.LLM.Generate(ctx, prompt, options...)
	if err != nil {
		return nil, err
	}

	// Validate response token count if limit is configured
	if err := w.validateResponseTokens(response); err != nil {
		return nil, err
	}

	return response, nil
}

// GenerateWithContent intercepts GenerateWithContent calls and enforces output token limits.
func (w *OutputTokenLimitWrapper) GenerateWithContent(ctx context.Context, content []core.ContentBlock, options ...core.GenerateOption) (*core.LLMResponse, error) {
	// Inject max_tokens if configured
	options = w.injectMaxTokens(options)

	// Call wrapped LLM
	response, err := w.LLM.GenerateWithContent(ctx, content, options...)
	if err != nil {
		return nil, err
	}

	// Validate response token count if limit is configured
	if err := w.validateResponseTokens(response); err != nil {
		return nil, err
	}

	return response, nil
}

// injectMaxTokens injects max_tokens into GenerateOptions if configured.
// Returns the options slice with max_tokens option prepended if limit is set.
func (w *OutputTokenLimitWrapper) injectMaxTokens(options []core.GenerateOption) []core.GenerateOption {
	if w.maxOutputTokens <= 0 {
		// No limit configured - use provider default
		return options
	}

	// Prepend max_tokens option so it can be overridden by caller if needed.
	// We need to check if dspy-go has a WithMaxTokens function, or we need to modify opts directly.
	// For now, we'll create a custom option that sets MaxTokens.
	maxTokensOption := func(opts *core.GenerateOptions) {
		// Only set if not already set by caller.
		if opts.MaxTokens <= 0 {
			opts.MaxTokens = w.maxOutputTokens
			if w.logger != nil {
				w.logger.WithFields(map[string]interface{}{
					"model":             w.modelID,
					"max_output_tokens": w.maxOutputTokens,
				}).Debug("Injected max_output_tokens into GenerateOptions")
			}
		}
	}

	// Prepend our option so it executes first (caller options can override).
	return append([]core.GenerateOption{maxTokensOption}, options...)
}

// validateResponseTokens validates that the response doesn't exceed the configured token limit.
func (w *OutputTokenLimitWrapper) validateResponseTokens(response *core.LLMResponse) error {
	if w.maxOutputTokens <= 0 {
		// No limit configured - skip validation
		return nil
	}

	if response == nil || response.Content == "" {
		// Empty response - nothing to validate
		return nil
	}

	// Count tokens in response
	tokenCount, err := validation.CountTokensForModel(response.Content, w.modelID)
	if err != nil {
		if w.logger != nil {
			w.logger.WithError(err).Warn("Failed to count response tokens for validation")
		}
		// Don't fail on token counting errors - just log.
		return nil
	}

	// Check if response exceeds limit
	if tokenCount > w.maxOutputTokens {
		errorMsg := fmt.Sprintf(
			"response exceeds output token limit: %d tokens (limit: %d tokens, model: %s)",
			tokenCount,
			w.maxOutputTokens,
			w.modelID,
		)

		if w.logger != nil {
			w.logger.WithFields(map[string]interface{}{
				"model":             w.modelID,
				"response_tokens":   tokenCount,
				"max_output_tokens": w.maxOutputTokens,
				"response_preview":  truncateString(response.Content, 200),
			}).Error(errorMsg)
		}

		return fmt.Errorf("output token limit exceeded: %s", errorMsg)
	}

	if w.logger != nil {
		w.logger.WithFields(map[string]interface{}{
			"model":             w.modelID,
			"response_tokens":   tokenCount,
			"max_output_tokens": w.maxOutputTokens,
		}).Debug("Response token count validated")
	}

	return nil
}

// truncateString truncates a string to the specified length with ellipsis.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
