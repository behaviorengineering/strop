package validation

import (
	"fmt"

	iso6391 "github.com/emvi/iso-639-1"
)

// ValidateLanguageCode validates that a language code is a valid ISO 639-1 code.
// It checks both the length (2-3 characters) and validates against the actual ISO 639-1 standard.
func ValidateLanguageCode(code string) error {
	if code == "" {
		return fmt.Errorf("language code is required")
	}

	// First check length (ISO 639-1 codes are typically 2 characters, but some are 3).
	if len(code) < 2 || len(code) > 3 {
		return fmt.Errorf("language must be a valid ISO 639-1 code (2-3 characters, got: %s)", code)
	}

	// Validate against actual ISO 639-1 codes.
	if !iso6391.ValidCode(code) {
		return fmt.Errorf("'%s' is not a valid ISO 639-1 language code", code)
	}

	return nil
}
