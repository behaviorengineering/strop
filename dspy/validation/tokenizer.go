package validation

import (
	"fmt"
	"strings"

	"github.com/tiktoken-go/tokenizer"
)

// ModelEncoding maps model names to their tokenizer encodings.
// This is used to determine which tokenizer to use for token counting.
var ModelEncoding = map[string]tokenizer.Encoding{
	// OpenAI models (cl100k_base encoding).
	"gpt-4":                  tokenizer.Cl100kBase,
	"gpt-4-turbo":            tokenizer.Cl100kBase,
	"gpt-4o":                 tokenizer.Cl100kBase,
	"gpt-4o-mini":            tokenizer.Cl100kBase,
	"gpt-3.5-turbo":          tokenizer.Cl100kBase,
	"gpt-3.5-turbo-16k":      tokenizer.Cl100kBase,
	"text-embedding-3-small": tokenizer.Cl100kBase,
	"text-embedding-3-large": tokenizer.Cl100kBase,
	"text-embedding-ada-002": tokenizer.Cl100kBase,

	// Perplexity models (OpenAI-compatible, use cl100k_base).
	"sonar":     tokenizer.Cl100kBase,
	"sonar-pro": tokenizer.Cl100kBase,

	// OpenRouter models (OpenAI-compatible, use cl100k_base).
	"google/gemini-2.5-flash-lite":    tokenizer.Cl100kBase, // OpenRouter uses OpenAI API.
	"deepseek/deepseek-v3.2-speciale": tokenizer.Cl100kBase, // OpenRouter uses OpenAI API.
	"x-ai/grok-4.1-fast":              tokenizer.Cl100kBase, // OpenRouter uses OpenAI API.

	// O200k_base models (newer OpenAI models).
	"gpt-4o-2024-08-06": tokenizer.O200kBase,
	"o1-preview":        tokenizer.O200kBase,
	"o1-mini":           tokenizer.O200kBase,
}

// CountTokens counts the number of tokens in a text string using the specified encoding.
// Returns the token count and any error encountered.
func CountTokens(text string, encoding tokenizer.Encoding) (int, error) {
	if text == "" {
		return 0, nil
	}

	enc, err := tokenizer.Get(encoding)
	if err != nil {
		return 0, fmt.Errorf("failed to get tokenizer encoding: %w", err)
	}

	tokens, _, err := enc.Encode(text)
	if err != nil {
		return 0, fmt.Errorf("failed to encode text: %w", err)
	}

	return len(tokens), nil
}

// CountTokensForModel counts tokens for a specific model by looking up its encoding.
// If the model is not found in ModelEncoding, it falls back to Cl100kBase (most common).
// Returns the token count and any error encountered.
func CountTokensForModel(text, modelID string) (int, error) {
	// Normalize model ID (remove any path prefixes for OpenRouter models).
	normalizedModel := normalizeModelID(modelID)

	// Look up encoding for this model.
	encoding, found := ModelEncoding[normalizedModel]
	if !found {
		// Default to Cl100kBase for unknown models (most common encoding).
		encoding = tokenizer.Cl100kBase
	}

	return CountTokens(text, encoding)
}

// ValidateContextTokens validates that the context text does not exceed the maximum token limit.
// Returns an error if the context exceeds the limit, nil otherwise.
func ValidateContextTokens(context, modelID string, maxTokens int) error {
	if maxTokens <= 0 {
		// No limit set, skip validation.
		return nil
	}

	if context == "" {
		// Empty context is always valid.
		return nil
	}

	tokenCount, err := CountTokensForModel(context, modelID)
	if err != nil {
		return fmt.Errorf("failed to count tokens: %w", err)
	}

	if tokenCount > maxTokens {
		return fmt.Errorf("context exceeds token limit: %d tokens (limit: %d tokens)", tokenCount, maxTokens)
	}

	return nil
}

// normalizeModelID normalizes a model ID by converting to lowercase for case-insensitive matching.
// This helps match models like "google/gemini-2.5-flash-lite" to the map keys.
// Note: The ModelEncoding map uses full model names (including provider prefixes like "google/"),
// so no prefix removal is needed - only case normalization is required.
func normalizeModelID(modelID string) string {
	// Convert to lowercase for case-insensitive matching.
	normalized := strings.ToLower(modelID)

	// Return normalized model ID (full name preserved, just lowercased).
	return normalized
}
