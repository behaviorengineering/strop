package validation

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/behaviorengineering/strop/dspy/rawresponse"
	"github.com/behaviorengineering/strop/streaming"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
	stroplog "github.com/behaviorengineering/strop/log"
)

// MandatoryFieldsResolver selects mandatory output fields from module inputs.
// Return nil to use defaultFields from ValidateMandatoryFieldsWithResolver; return a non-nil slice to override.
type MandatoryFieldsResolver func(inputs map[string]any, info *core.ModuleInfo) []string

// OutputValidator is a function that validates module outputs.
// Returns an error if validation fails, nil if validation passes.
type OutputValidator func(ctx context.Context, inputs map[string]any, outputs map[string]any, info *core.ModuleInfo) error

// Logger is the strop logger used for validation failures and retries.
type Logger = stroplog.Logger

// ValidationInterceptor creates a generic interceptor that validates module outputs.
// If validation fails, it returns an error that will trigger the retry interceptor.
// This interceptor should be placed AFTER the retry interceptor (innermost position)
// so that validation errors bubble up to the retry interceptor.
// If logger is provided, it will log validation failures to help debug retry attempts.
func ValidationInterceptor(validator OutputValidator, logger Logger) core.ModuleInterceptor {
	return func(ctx context.Context, inputs map[string]any, info *core.ModuleInfo, handler core.ModuleHandler, opts ...core.Option) (map[string]any, error) {
		// Call handler to get result.
		result, err := handler(ctx, inputs, opts...)
		if err != nil {
			// Preserve result (e.g. raw LLM text) so outer OpenInference can record it on error spans.
			return result, err
		}

		// Validate result.
		if err := validator(ctx, inputs, result, info); err != nil {
			streaming.EmitWarningf(ctx, info.ModuleName,
				"Output validation failed (another attempt may run if retries are enabled): %v", err)
			// Log validation failure - this helps users understand why retries are happening.
			if logger != nil {
				fields := map[string]interface{}{
					"module":      info.ModuleName,
					"module_type": info.ModuleType,
					"error":       err.Error(),
				}
				if raw, _ := rawresponse.TextFrom(result); raw != "" {
					preview := raw
					if len(preview) > 800 {
						preview = preview[:800] + "... (truncated)"
					}
					fields["raw_response_preview"] = preview
				}
				logger.WithFields(fields).Warn("Output validation failed - retry will be triggered")
			}
			// Return validation error - this will trigger retry interceptor.
			return nil, fmt.Errorf("output validation failed: %w", err)
		}

		// Validation passed - return result.
		return result, nil
	}
}

// ValidateMandatoryFields creates a validator that ensures specified mandatory fields exist and are not empty.
// If mandatoryFields is nil or empty, it validates all output fields from the signature.
// For string fields, it checks that the value is not empty after trimming whitespace.
// For non-string fields, it only checks that the field exists (not nil).
func ValidateMandatoryFields(mandatoryFields []string) OutputValidator {
	return ValidateMandatoryFieldsWithResolver(mandatoryFields, nil)
}

// ValidateMandatoryFieldsWithResolver validates outputs using defaultFields unless resolver returns a non-nil slice.
func ValidateMandatoryFieldsWithResolver(defaultFields []string, resolver MandatoryFieldsResolver) OutputValidator {
	return func(ctx context.Context, inputs map[string]any, outputs map[string]any, info *core.ModuleInfo) error {
		fieldsToValidate := defaultFields
		if resolver != nil {
			if resolved := resolver(inputs, info); resolved != nil {
				fieldsToValidate = resolved
			}
		}
		if len(fieldsToValidate) == 0 {
			fieldsToValidate = make([]string, 0, len(info.Signature.Outputs))
			for _, outputField := range info.Signature.Outputs {
				fieldsToValidate = append(fieldsToValidate, outputField.Name)
			}
		}
		return validateMandatoryFieldList(fieldsToValidate, outputs)
	}
}

// ValidateFieldList checks that each named field exists and is non-empty.
func ValidateFieldList(fieldsToValidate []string, outputs map[string]any) error {
	return validateMandatoryFieldList(fieldsToValidate, outputs)
}

func validateMandatoryFieldList(fieldsToValidate []string, outputs map[string]any) error {
	// Deduplicate while preserving first-seen order so each map key is validated once and error messages stay clear.
	if len(fieldsToValidate) > 1 {
		seen := make(map[string]bool, len(fieldsToValidate))
		unique := fieldsToValidate[:0]
		for _, name := range fieldsToValidate {
			if seen[name] {
				continue
			}
			seen[name] = true
			unique = append(unique, name)
		}
		fieldsToValidate = unique
	}

	if len(outputs) == 0 {
		return fmt.Errorf("result is empty, expected fields: %v", fieldsToValidate)
	}

	var missingFields []string
	var emptyFields []string

	for _, fieldName := range fieldsToValidate {
		value, exists := outputs[fieldName]
		if !exists {
			missingFields = append(missingFields, fieldName)
			continue
		}

		if strValue, ok := value.(string); ok {
			if strings.TrimSpace(strValue) == "" {
				emptyFields = append(emptyFields, fieldName)
			}
		} else if value == nil {
			emptyFields = append(emptyFields, fieldName)
		} else if slice, ok := value.([]interface{}); ok && len(slice) == 0 {
			emptyFields = append(emptyFields, fieldName)
		}
	}

	if len(missingFields) > 0 || len(emptyFields) > 0 {
		var errorParts []string
		if len(missingFields) > 0 {
			errorParts = append(errorParts, fmt.Sprintf("missing fields: %v", missingFields))
		}
		if len(emptyFields) > 0 {
			errorParts = append(errorParts, fmt.Sprintf("empty fields: %v", emptyFields))
		}

		availableFields := make([]string, 0, len(outputs))
		for k := range outputs {
			availableFields = append(availableFields, k)
		}

		return fmt.Errorf("mandatory field validation failed (%s) - available fields: %v",
			strings.Join(errorParts, ", "), availableFields)
	}

	return nil
}

// ValidateCriterionScoresNumeric ensures criterion_scores is present and every value is a plain decimal number.
// Checklist punctuation (".", "✓", "[✓]") must fail so RetryModuleInterceptor can re-run Score Generation.
func ValidateCriterionScoresNumeric(ctx context.Context, inputs map[string]any, outputs map[string]any, info *core.ModuleInfo) error {
	_ = ctx
	_ = inputs
	_ = info
	if err := validateMandatoryFieldList([]string{"criterion_scores"}, outputs); err != nil {
		return err
	}
	return ValidateCriterionScoreValues(outputs["criterion_scores"])
}

// ValidateCriterionScoreValues checks that criterion_scores values are numeric (float or decimal string).
func ValidateCriterionScoreValues(value any) error {
	if value == nil {
		return fmt.Errorf("criterion_scores is nil")
	}
	scoresMap, ok := value.(map[string]any)
	if !ok {
		// Accept map[string]interface{} via conversion when needed.
		if legacy, legacyOK := value.(map[string]interface{}); legacyOK {
			scoresMap = make(map[string]any, len(legacy))
			for k, v := range legacy {
				scoresMap[k] = v
			}
		} else {
			return fmt.Errorf("criterion_scores has invalid type %T (expected map)", value)
		}
	}
	if len(scoresMap) == 0 {
		return fmt.Errorf("criterion_scores is empty")
	}
	var invalid []string
	for criterionID, scoreValue := range scoresMap {
		if _, err := parseNumericScore(scoreValue); err != nil {
			invalid = append(invalid, fmt.Sprintf("%s=%q", criterionID, fmt.Sprint(scoreValue)))
		}
	}
	if len(invalid) == 0 {
		return nil
	}
	sort.Strings(invalid)
	return fmt.Errorf("criterion_scores must be plain decimal numbers (not checklist marks); invalid: [%s]", strings.Join(invalid, ", "))
}

func parseNumericScore(scoreValue any) (float64, error) {
	switch v := scoreValue.(type) {
	case float64:
		return v, nil
	case float32:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case int32:
		return float64(v), nil
	case int:
		return float64(v), nil
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return 0, fmt.Errorf("empty score")
		}
		score, err := strconv.ParseFloat(trimmed, 64)
		if err != nil {
			return 0, err
		}
		return score, nil
	default:
		return 0, fmt.Errorf("invalid type %T", scoreValue)
	}
}

// ValidateFeedbackField is a specific validator that ensures the feedback field exists and is not empty.
// This is a convenience function for the common case of validating feedback.
func ValidateFeedbackField(ctx context.Context, outputs map[string]any, info *core.ModuleInfo) error {
	return ValidateMandatoryFields([]string{"feedback"})(ctx, nil, outputs, info)
}
