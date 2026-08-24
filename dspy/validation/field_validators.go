package validation

import (
	"context"
	"fmt"

	stropdspy "github.com/behaviorengineering/strop/dspy"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
)

// FieldTokenValidator creates a validator factory for any field that checks token limits.
// The field name is configurable, allowing different pipelines to validate different fields
// (e.g., "saying_context" for sayings, "video_transcript" for videos) using the same logic.
//
// Returns a function that takes a provider config and returns an InputProcessor.
// The InputProcessor validates that the specified field does not exceed the provider's MaxContextTokens.
//
// Example usage:
//
//	validator := FieldTokenValidator("saying_context")(provider)
//	err := validator(ctx, inputs, info)
func FieldTokenValidator(fieldName string) func(stropdspy.ProviderConfig) InputProcessor {
	return func(provider stropdspy.ProviderConfig) InputProcessor {
		return func(ctx context.Context, inputs map[string]any, info *core.ModuleInfo) error {
			// Get field by name (configurable!).
			fieldValue, exists := inputs[fieldName]
			if !exists {
				// Field not present - nothing to validate.
				return nil
			}

			// Convert to string.
			contextStr, ok := fieldValue.(string)
			if !ok {
				// Not a string - skip validation (might be empty map or other type).
				return nil
			}

			// Get max tokens from provider config (defaults to 2500 if not set).
			maxTokens := provider.MaxContextTokens
			if maxTokens <= 0 {
				// No limit configured - skip validation.
				return nil
			}

			// Validate token count using generic tokenizer from dspy/validation.
			if err := ValidateContextTokens(contextStr, provider.Model, maxTokens); err != nil {
				return fmt.Errorf("%s validation failed: %w", fieldName, err)
			}

			return nil
		}
	}
}

// FieldLanguageValidator creates a validator factory for any field that checks language codes.
// The field name is configurable, allowing different pipelines to validate different fields
// (e.g., "source_language" and "target_language" for sayings) using the same logic.
//
// Returns a function that takes a provider config and returns an InputProcessor.
// The InputProcessor validates that the specified field contains a valid ISO 639-1 language code.
//
// Example usage:
//
//	validator := FieldLanguageValidator("source_language")(provider)
//	err := validator(ctx, inputs, info)
func FieldLanguageValidator(fieldName string) func(stropdspy.ProviderConfig) InputProcessor {
	return func(_ stropdspy.ProviderConfig) InputProcessor {
		return func(ctx context.Context, inputs map[string]any, info *core.ModuleInfo) error {
			// Get field by name (configurable!).
			langCode, exists := inputs[fieldName]
			if !exists {
				// Field not present - nothing to validate.
				return nil
			}

			// Convert to string.
			codeStr, ok := langCode.(string)
			if !ok {
				// Not a string - skip validation.
				return nil
			}

			// Validate language code using generic validation function.
			if err := ValidateLanguageCode(codeStr); err != nil {
				return fmt.Errorf("%s validation failed: %w", fieldName, err)
			}

			return nil
		}
	}
}

// ComposeValidators combines multiple validators into one InputProcessor.
// All validators are executed in order, and if any validator returns an error, the composition fails.
//
// Example usage:
//
//	composed := ComposeValidators(
//	    FieldTokenValidator("saying_context")(provider),
//	    FieldLanguageValidator("source_language")(provider),
//	    FieldLanguageValidator("target_language")(provider),
//	)
//	err := composed(ctx, inputs, info)
func ComposeValidators(validators ...InputProcessor) InputProcessor {
	return func(ctx context.Context, inputs map[string]any, info *core.ModuleInfo) error {
		for _, validator := range validators {
			if err := validator(ctx, inputs, info); err != nil {
				return err
			}
		}
		return nil
	}
}
