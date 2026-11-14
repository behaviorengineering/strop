package criteria

import (
	"fmt"
	"strings"
)

// FormatRubric concatenates registry Name, Description, Scoring, and Examples for each criterion ID.
// Used as JEV ScoreRequest.Feedback when scoring before LLM feedback analysis.
func FormatRubric(registry *CriterionRegistry, ids []CriterionID) (string, error) {
	if len(ids) == 0 {
		return "", fmt.Errorf("criteria: FormatRubric requires at least one criterion ID")
	}
	if registry == nil {
		registry = DefaultRegistry()
	}
	var parts []string
	for _, id := range ids {
		desc, err := registry.Get(id)
		if err != nil {
			return "", fmt.Errorf("criteria: FormatRubric %s: %w", id, err)
		}
		parts = append(parts, fmt.Sprintf("=== %s ===\n%s\n%s\n%s\n%s",
			id,
			desc.Name,
			strings.TrimSpace(desc.Description),
			strings.TrimSpace(desc.Scoring),
			strings.TrimSpace(desc.Examples)))
	}
	return strings.Join(parts, "\n\n"), nil
}
