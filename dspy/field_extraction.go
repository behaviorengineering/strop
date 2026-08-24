package dspy

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"unicode"
)

// ExtractStringField extracts a string field from outputs.
// With XML interceptors enabled, fields are always at the top level.
// Returns the string value and true if the field exists, is a string, and is non-empty.
// Returns empty string and false otherwise.
func ExtractStringField(outputs map[string]interface{}, fieldName string) (string, bool) {
	if value, ok := outputs[fieldName]; ok {
		if str, isString := value.(string); isString && str != "" {
			return str, true
		}
	}
	return "", false
}

// ExtractRequiredStringField extracts and validates a required string field from outputs.
// Returns an error if the field is missing, not a string, or empty.
// The error can be wrapped with domain-specific error types by the caller.
func ExtractRequiredStringField(outputs map[string]interface{}, fieldName string) (string, error) {
	value, ok := ExtractStringField(outputs, fieldName)
	if !ok {
		return "", fmt.Errorf("required field %q is missing, not a string, or empty", fieldName)
	}
	return value, nil
}

// CoerceCriterionScoresMap normalizes criterion_scores from structured output parsing.
// Accepts map[string]interface{} and map[string]string. When the model emits a JSON object
// inside the XML field (plain text under <criterion_scores>), unmarshals it to a map.
func CoerceCriterionScoresMap(value interface{}) (map[string]interface{}, error) {
	if value == nil {
		return nil, fmt.Errorf("criterion_scores is nil")
	}

	switch scores := value.(type) {
	case map[string]interface{}:
		return scores, nil
	case map[string]string:
		out := make(map[string]interface{}, len(scores))
		for k, v := range scores {
			out[k] = v
		}
		return out, nil
	case string:
		s := strings.TrimSpace(scores)
		if s == "" {
			return nil, fmt.Errorf("criterion_scores is an empty string")
		}
		if !strings.HasPrefix(s, "{") {
			return nil, fmt.Errorf(
				"criterion_scores has invalid type string (expected nested XML map or JSON object). "+
					"Preview: %q. Expected format: <criterion_scores><instruction_compliance>2.0</instruction_compliance>...</criterion_scores>",
				truncateForError(s, 200),
			)
		}
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(s), &parsed); err != nil {
			return nil, fmt.Errorf(
				"criterion_scores string is not valid JSON object: %w (preview: %q)",
				err,
				truncateForError(s, 200),
			)
		}
		if len(parsed) == 0 {
			return nil, fmt.Errorf("criterion_scores JSON object is empty")
		}
		return parsed, nil
	default:
		return nil, fmt.Errorf("criterion_scores has invalid type %T (expected map[string]interface{}). Value preview: %s", value, valuePreviewForError(value))
	}
}

func truncateForError(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func valuePreviewForError(value interface{}) string {
	switch v := value.(type) {
	case string:
		return truncateForError(fmt.Sprintf("%q", v), 220)
	default:
		return truncateForError(fmt.Sprintf("%v", v), 220)
	}
}

// ExtractReasoningField returns DirectivesCoT structured-attention text from directives_ack.
func ExtractReasoningField(outputs map[string]interface{}) (string, bool) {
	return ExtractStringField(outputs, FieldDirectivesAck)
}

// ExtractRequiredReasoningField requires a non-empty directives_ack field.
func ExtractRequiredReasoningField(outputs map[string]interface{}) (string, error) {
	value, ok := ExtractReasoningField(outputs)
	if !ok {
		return "", fmt.Errorf("required field %q is missing, not a string, or empty", FieldDirectivesAck)
	}
	return value, nil
}

// ExtractOptionalStringField extracts an optional string field from outputs.
// Returns the value (possibly empty after trim) and true if the field exists and is a string.
// Returns ("", false) if the field is missing or not a string. Use this when empty is valid (e.g. optional research question).
func ExtractOptionalStringField(outputs map[string]interface{}, fieldName string) (string, bool) {
	value, ok := outputs[fieldName]
	if !ok {
		return "", false
	}
	str, isString := value.(string)
	if !isString {
		return "", false
	}
	return strings.TrimSpace(str), true
}

// ExtractStringArray extracts a string array field from outputs.
// Handles []interface{}, []string, and string types (for XML parsing where arrays may be returned as strings).
// Returns the string array and true if the field exists and can be parsed as a string array.
// Returns empty array and false otherwise.
func ExtractStringArray(outputs map[string]interface{}, fieldName string) ([]string, bool) {
	value, ok := outputs[fieldName]
	if !ok {
		return []string{}, false
	}

	// Try []interface{} first (common from JSON unmarshaling).
	if arr, ok := value.([]interface{}); ok {
		result := make([]string, 0, len(arr))
		for _, item := range arr {
			if str, ok := item.(string); ok {
				result = append(result, str)
			} else {
				return []string{}, false
			}
		}
		return result, true
	}

	// Try []string directly.
	if arr, ok := value.([]string); ok {
		return arr, true
	}

	// Handle string input (common when XML parser returns array fields as strings).
	if str, ok := value.(string); ok {
		s := strings.TrimSpace(str)
		// Try JSON array first (e.g. ["item1", "item2"] from structured output).
		if len(s) >= 2 && s[0] == '[' && s[len(s)-1] == ']' {
			var parsed []interface{}
			if err := json.Unmarshal([]byte(s), &parsed); err == nil {
				result := make([]string, 0, len(parsed))
				for _, item := range parsed {
					if s2, ok := item.(string); ok {
						result = append(result, strings.TrimSpace(s2))
					}
				}
				if len(result) > 0 {
					return result, true
				}
			}
			// Fallback: LLM may output slightly invalid JSON (trailing comma, etc.). Extract quoted strings.
			if extracted := extractQuotedStringsFromArrayLike(s); len(extracted) > 0 {
				return extracted, true
			}
		}
		// Try to parse as a list by splitting on newlines (e.g. line-per-item).
		lines := strings.Split(str, "\n")
		result := make([]string, 0, len(lines))
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			// Remove common list prefixes (numbered, bulleted, etc.)
			line = strings.TrimPrefix(line, "- ")
			line = strings.TrimPrefix(line, "* ")
			line = strings.TrimPrefix(line, "• ")
			// Remove numbered prefixes (1., 2., etc.)
			if len(line) > 2 && line[1] == '.' && line[0] >= '0' && line[0] <= '9' {
				line = strings.TrimSpace(line[2:])
			}
			if line != "" {
				result = append(result, line)
			}
		}
		if len(result) > 0 {
			return result, true
		}
		// If newline splitting didn't work, try comma splitting.
		parts := strings.Split(str, ",")
		if len(parts) > 1 {
			result = make([]string, 0, len(parts))
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if part != "" {
					result = append(result, part)
				}
			}
			if len(result) > 0 {
				return result, true
			}
		}
		// If all else fails, return the string as a single-item array.
		if str != "" {
			return []string{strings.TrimSpace(str)}, true
		}
	}

	return []string{}, false
}

// NormalizeOutputArrayFields parses string values for the given field names (e.g. "quotes", "topics")
// into []interface{} and sets them back on the map. Use so display and downstream code see proper arrays
// instead of raw JSON strings. Modifies the map in place; no-op if the value is already a slice.
func NormalizeOutputArrayFields(outputs map[string]interface{}, fieldNames []string) {
	if outputs == nil {
		return
	}
	for _, name := range fieldNames {
		arr, ok := ExtractStringArray(outputs, name)
		if !ok || len(arr) == 0 {
			continue
		}
		// Only replace if current value is a string (avoid overwriting already-parsed slice).
		if _, isString := outputs[name].(string); isString {
			quotesIf := make([]interface{}, len(arr))
			for i, s := range arr {
				quotesIf[i] = s
			}
			outputs[name] = quotesIf
		}
	}
}

// extractQuotedStringsFromArrayLike extracts quoted strings from an array-like string (e.g. ["a", "b"] or ['a', 'b']).
// Handles both double and single quotes; escaped quotes (\", \') inside strings. Used when json.Unmarshal fails (e.g. trailing comma, single quotes).
func extractQuotedStringsFromArrayLike(s string) []string {
	var out []string
	i := 0
	for i < len(s) {
		switch s[i] {
		case '"', '\'':
			quote := s[i]
			i++
			var buf strings.Builder
			for i < len(s) {
				if s[i] == '\\' && i+1 < len(s) && (s[i+1] == '"' || s[i+1] == '\'') {
					buf.WriteByte(s[i+1])
					i += 2
					continue
				}
				if s[i] == quote {
					i++
					out = append(out, strings.TrimSpace(buf.String()))
					break
				}
				buf.WriteByte(s[i])
				i++
			}
		default:
			i++
		}
	}
	return out
}

// MapStructFromMap uses reflection to map a map[string]interface{} to a struct type T.
// Struct field names are converted from PascalCase to snake_case to match map keys.
// Only exported string fields are mapped.
// Returns a pointer to a new instance of T with fields populated from the map.
// Fields not present in the map are left as zero values.
//
// Example:
//
//	type Output struct {
//	    LiteralTranslation  string
//	    SemanticTranslation string
//	}
//
//	output, err := MapStructFromMap[Output](map[string]interface{}{
//	    "literal_translation":  "value1",
//	    "semantic_translation": "value2",
//	})
func MapStructFromMap[T any](m map[string]interface{}) (*T, error) {
	if m == nil {
		return nil, fmt.Errorf("map is nil - cannot map to struct")
	}

	// Create a new instance of T
	var zero T
	target := reflect.ValueOf(&zero).Elem()
	t := target.Type()

	for i := 0; i < target.NumField(); i++ {
		field := target.Field(i)
		fieldType := t.Field(i)

		// Skip unexported fields
		if !field.CanSet() {
			continue
		}

		// Only handle string fields
		if field.Kind() != reflect.String {
			continue
		}

		// Convert field name from PascalCase to snake_case
		key := toSnakeCase(fieldType.Name)

		// Get value from map
		value, ok := m[key]
		if !ok {
			// Field not in map - leave struct field as zero value
			continue
		}

		// Convert to string
		var strValue string
		if str, ok := value.(string); ok {
			strValue = str
		} else {
			return nil, fmt.Errorf("field %q is not a string in map (got %T)", key, value)
		}

		// Set the field value
		field.SetString(strValue)
	}

	return &zero, nil
}

// MapStructFromMapRequired is like MapStructFromMap but requires all string fields
// to be present in the map. Returns an error if any string field is missing.
func MapStructFromMapRequired[T any](m map[string]interface{}) (*T, error) {
	if m == nil {
		return nil, fmt.Errorf("map is nil - cannot map to struct")
	}

	// Create a new instance of T
	var zero T
	target := reflect.ValueOf(&zero).Elem()
	t := target.Type()

	for i := 0; i < target.NumField(); i++ {
		field := target.Field(i)
		fieldType := t.Field(i)

		// Skip unexported fields
		if !field.CanSet() {
			continue
		}

		// Only handle string fields
		if field.Kind() != reflect.String {
			continue
		}

		// Convert field name from PascalCase to snake_case
		key := toSnakeCase(fieldType.Name)

		// Get value from map - required
		value, ok := m[key]
		if !ok {
			return nil, fmt.Errorf("required field %q not found in map", key)
		}

		// Convert to string
		var strValue string
		if str, ok := value.(string); ok {
			strValue = str
		} else {
			return nil, fmt.Errorf("field %q is not a string in map (got %T)", key, value)
		}

		// Set the field value
		field.SetString(strValue)
	}

	return &zero, nil
}

// toSnakeCase converts PascalCase to snake_case.
// Example: "LiteralTranslation" -> "literal_translation".
func toSnakeCase(s string) string {
	var result strings.Builder
	for i, r := range s {
		if unicode.IsUpper(r) {
			if i > 0 {
				result.WriteByte('_')
			}
			result.WriteRune(unicode.ToLower(r))
		} else {
			result.WriteRune(r)
		}
	}
	return result.String()
}
