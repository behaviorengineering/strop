package dspy

import (
	"errors"
	"fmt"
	"strings"

	dspyErrors "github.com/XiaoConstantine/dspy-go/pkg/errors"
)

// sanitizedDSPyError wraps a DSPy error with a clean message while preserving
// the original DSPy error for errors.As() to find.
type sanitizedDSPyError struct {
	message string
	dspyErr *dspyErrors.Error
	cause   error
}

func (e *sanitizedDSPyError) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %v", e.message, e.cause)
	}
	return e.message
}

func (e *sanitizedDSPyError) Unwrap() error {
	// Return the original DSPy error so errors.As() can find it.
	if e.dspyErr != nil {
		return e.dspyErr
	}
	return e.cause
}

const (
	// maxFieldValueLength is the maximum length for field values in sanitized errors.
	// Values longer than this will be truncated.
	maxFieldValueLength = 50
	// truncatedSuffix is appended to truncated field values.
	truncatedSuffix = "..."
)

// SanitizeDSPyError extracts a clean, user-friendly error message from DSPy-Go errors.
// It removes verbose details while preserving essential error information.
//
// The function uses structured error handling (errors.As, Fields()) to efficiently
// access and filter DSPy-Go error fields. It traverses the entire error chain to
// find DSPy errors, even when deeply wrapped.
//
// If no DSPy errors are found (validation failures, retry-exhausted wrappers),
// the original error is returned unchanged.
//
// Excluded fields (prompt, module, content_blocks) are too verbose for logs.
// Field values are truncated to maxFieldValueLength to prevent log spam.
func SanitizeDSPyError(err error) error {
	if err == nil {
		return nil
	}

	// Try structured error handling - errors.As() automatically traverses the chain.
	var dspyErr *dspyErrors.Error
	if errors.As(err, &dspyErr) {
		return sanitizeDSPyErrorStructured(dspyErr)
	}

	// This handles cases where the error chain might be broken or non-standard.
	// Use errors.As to check for wrapped errors in the chain.
	var fallbackErr *dspyErrors.Error
	if errors.As(err, &fallbackErr) {
		return sanitizeDSPyErrorStructured(fallbackErr)
	}

	return err
}

// sanitizeDSPyErrorStructured uses structured error handling to efficiently sanitize DSPy-Go errors.
// It uses Fields() for structured access and avoids string parsing by working with structured data.
func sanitizeDSPyErrorStructured(dspyErr *dspyErrors.Error) error {
	// Get the unwrapped error first.
	unwrapped := errors.Unwrap(dspyErr)

	// But we'll use Fields() to reconstruct a clean message without parsing.
	fullMessage := dspyErr.Error()

	// We'll use the full message but filter fields using Fields() instead of parsing.

	// Get structured fields.
	fields := dspyErr.Fields()

	// Since message is unexported, we accept that we need Error() but we won't parse it.
	baseMessage := fullMessage

	// Filter out verbose fields.
	excludedFields := map[string]bool{
		"prompt":         true,
		"module":         true,
		"content_blocks": true,
	}

	cleanFields := make(dspyErrors.Fields)
	for k, v := range fields {
		if !excludedFields[k] {
			// Handle different field value types.
			var cleanValue interface{}
			switch val := v.(type) {
			case string:
				// Truncate long string values.
				if len(val) > maxFieldValueLength {
					truncateLength := maxFieldValueLength - len(truncatedSuffix)
					if truncateLength <= 0 {
						truncateLength = 1
					}
					cleanValue = val[:truncateLength] + truncatedSuffix
				} else {
					cleanValue = val
				}
			case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, bool:
				// Simple numeric and boolean types - use as-is.
				cleanValue = val
			case fmt.Stringer:
				// Types that implement Stringer - use their String() method.
				cleanValue = val.String()
			default:
				// Don't try to parse string representations - just skip.
				continue
			}
			cleanFields[k] = cleanValue
		}
	}

	// Sanitize wrapped error recursively.
	var sanitizedCause error
	if unwrapped != nil {
		sanitizedCause = SanitizeDSPyError(unwrapped)
	}

	// Reconstruct error with cleaned fields, preserving the original DSPy error
	// so that errors.As() can still find it in the chain
	// Note: We need minimal string parsing (finding bracket) because the message field
	// is unexported, so we can't access it directly. This is the minimum parsing needed.
	var cleanMessage string
	if len(cleanFields) > 0 {
		// Build field string from structured fields.
		var fieldParts []string
		for k, v := range cleanFields {
			fieldParts = append(fieldParts, fmt.Sprintf("%s=%v", k, v))
		}
		fieldStr := strings.Join(fieldParts, ", ")

		// Minimal parsing: find where fields start (bracket) to separate message from fields.
		bracketIdx := strings.Index(baseMessage, " [")
		if bracketIdx >= 0 {
			// Extract message part (before fields) and add cleaned fields.
			messagePart := strings.TrimSpace(baseMessage[:bracketIdx])
			cleanMessage = fmt.Sprintf("%s [%s]", messagePart, fieldStr)
		} else {
			// No fields in original message, just add cleaned fields.
			cleanMessage = fmt.Sprintf("%s [%s]", baseMessage, fieldStr)
		}
	} else {
		// No cleaned fields, remove original fields from baseMessage if present.
		bracketIdx := strings.Index(baseMessage, " [")
		if bracketIdx >= 0 {
			cleanMessage = strings.TrimSpace(baseMessage[:bracketIdx])
		} else {
			cleanMessage = baseMessage
		}
	}

	return &sanitizedDSPyError{
		message: cleanMessage,
		dspyErr: dspyErr,
		cause:   sanitizedCause,
	}
}
