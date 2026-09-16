package validation

import (
	"context"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
)

// RequiredInputsResolver selects required input field names from module inputs and info.
// Return nil to use defaultFields from ValidateRequiredInputsWithResolver; return a non-nil
// slice to override (including an empty slice meaning "no required inputs").
type RequiredInputsResolver func(inputs map[string]any, info *core.ModuleInfo) []string

// ValidateRequiredInputs ensures specified input keys exist and are non-empty before the module runs.
// If requiredFields is nil or empty, it validates all input fields from the signature (when info is set).
// For string values, whitespace-only counts as empty. Nil values and empty []interface{} slices fail.
// Non-string, non-nil values (for example numbers or structs) count as present.
//
// Use with InputProcessingInterceptor, or call the returned InputProcessor directly (for example
// before RLMComplete when evidence is a named map).
func ValidateRequiredInputs(requiredFields []string) InputProcessor {
	return ValidateRequiredInputsWithResolver(requiredFields, nil)
}

// ValidateRequiredInputsWithResolver validates inputs using defaultFields unless resolver returns a non-nil slice.
func ValidateRequiredInputsWithResolver(defaultFields []string, resolver RequiredInputsResolver) InputProcessor {
	return func(ctx context.Context, inputs map[string]any, info *core.ModuleInfo) error {
		_ = ctx
		fieldsToValidate := defaultFields
		if resolver != nil {
			if resolved := resolver(inputs, info); resolved != nil {
				fieldsToValidate = resolved
			}
		}
		if len(fieldsToValidate) == 0 {
			if info == nil {
				return nil
			}
			fieldsToValidate = make([]string, 0, len(info.Signature.Inputs))
			for _, inputField := range info.Signature.Inputs {
				fieldsToValidate = append(fieldsToValidate, inputField.Name)
			}
		}
		if len(fieldsToValidate) == 0 {
			return nil
		}
		return validateNamedFieldList("required input", fieldsToValidate, inputs)
	}
}
