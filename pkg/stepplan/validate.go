package stepplan

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ValidateOutput checks DoneCriteria.RequiredKeys against a JSON object output.
// Non-object output fails when RequiredKeys is non-empty. Empty RequiredKeys is a no-op.
func ValidateOutput(step Step, output json.RawMessage) error {
	keys := step.DoneCriteria.RequiredKeys
	if len(keys) == 0 {
		return nil
	}
	if len(output) == 0 {
		return fmt.Errorf("stepplan: step %q output is empty but required_keys=%v", step.ID, keys)
	}
	var obj map[string]any
	if err := json.Unmarshal(output, &obj); err != nil {
		return fmt.Errorf("stepplan: step %q output is not a JSON object: %w", step.ID, err)
	}
	missing := make([]string, 0)
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		v, ok := obj[key]
		if !ok || isEmptyOutputValue(v) {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("stepplan: step %q missing required output keys: %s", step.ID, strings.Join(missing, ", "))
	}
	return nil
}

func isEmptyOutputValue(v any) bool {
	if v == nil {
		return true
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t) == ""
	case []any:
		return len(t) == 0
	case map[string]any:
		return len(t) == 0
	default:
		return false
	}
}
