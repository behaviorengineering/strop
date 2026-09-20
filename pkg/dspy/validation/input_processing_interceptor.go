package validation

import (
	"context"
	"fmt"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
)

// InputProcessor is a function that processes module inputs before they are passed to the module handler.
// It can perform validation, transformation, mutation, or any other input processing logic.
//
// Capabilities:
//   - Validation: Return an error if inputs are invalid (prevents module execution)
//   - Mutation: Modify the inputs map directly (e.g., truncate context, normalize fields, add defaults)
//   - Transformation: Rename fields, convert types, or restructure input data
//   - Context-aware: Use info.ModuleInfo to make decisions based on module name, type, or signature
//
// The inputs map is passed by reference, so mutations will affect the inputs passed to the module handler.
// If an error is returned, the module handler will not be called.
//
// Example use cases:
//   - Token-based context validation (reject if exceeds limit)
//   - Context truncation (truncate if exceeds limit instead of rejecting)
//   - Field name normalization (e.g., "saying_context" -> "context")
//   - Default value injection (add missing required fields)
//   - Module-specific input transformation (different processing per module type)
type InputProcessor func(ctx context.Context, inputs map[string]any, info *core.ModuleInfo) error

// InputProcessingInterceptor creates a generic interceptor that processes module inputs before execution.
// The processor can validate, mutate, or transform inputs based on the module information.
//
// Processing happens BEFORE the module handler is called, allowing for:
//   - Early rejection of invalid inputs (return error)
//   - Input transformation (mutate inputs map)
//   - Context-aware processing (use info.ModuleInfo for decisions)
//
// This interceptor should be placed BEFORE other interceptors (outermost position)
// so that input processing happens early in the execution chain.
//
// If logger is provided, it will log processing failures (validation errors, etc.).
func InputProcessingInterceptor(processor InputProcessor, logger Logger) core.ModuleInterceptor {
	return func(ctx context.Context, inputs map[string]any, info *core.ModuleInfo, handler core.ModuleHandler, opts ...core.Option) (map[string]any, error) {
		// Process inputs before calling handler (validation, mutation, transformation).
		if err := processor(ctx, inputs, info); err != nil {
			// Log processing failure - this helps users understand why requests are rejected.
			if logger != nil {
				logger.WithFields(map[string]interface{}{
					"module":      info.ModuleName,
					"module_type": info.ModuleType,
					"error":       err.Error(),
				}).Warn("Input processing failed - request rejected")
			}
			// Return processing error - this prevents the module from being called.
			return nil, fmt.Errorf("input processing failed: %w", err)
		}

		// Processing passed - call handler with (potentially mutated) inputs.
		result, err := handler(ctx, inputs, opts...)
		return result, err
	}
}
