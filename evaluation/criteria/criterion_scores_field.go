package criteria

import (
	"regexp"
	"strings"
)

const (
	criterionScoresDescBase = "Map of individual criterion scores. Each key is a criterion ID and each value is the score (0.0 to max_points). Scores MUST align with the feedback provided"
	exactMapKeysMarker      = "Exact map keys:"
)

// criterionIDMappingArrow matches score-prompt lines like: - Name → "instruction_compliance"
var criterionIDMappingArrow = regexp.MustCompile(`→\s*"([a-z][a-z0-9_]*)"`)

// ParseCriterionIDsFromMappingPrompt extracts criterion IDs from CRITERION ID MAPPING lines in a score prompt.
func ParseCriterionIDsFromMappingPrompt(instruction string) []CriterionID {
	if instruction == "" {
		return nil
	}
	matches := criterionIDMappingArrow.FindAllStringSubmatch(instruction, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(matches))
	out := make([]CriterionID, 0, len(matches))
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		id := strings.TrimSpace(m[1])
		if id == "" {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, CriterionID(id))
	}
	return out
}

// CriterionScoresOutputDescription builds the score-generation output field description.
// When ids is non-empty, includes Exact map keys so the XML formatter emits one child tag per criterion.
func CriterionScoresOutputDescription(ids []CriterionID) string {
	if len(ids) == 0 {
		return criterionScoresDescBase + "."
	}
	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = string(id)
	}
	return criterionScoresDescBase + ". " + exactMapKeysMarker + " " + strings.Join(keys, ", ") + "."
}
