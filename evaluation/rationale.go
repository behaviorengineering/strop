package evaluation

import (
	"fmt"
	"strings"
)

// CombineRationales combines individual evaluator rationales into a single string.
// Format: "Agent1: rationale1\n\nAgent2: rationale2\n\n..."
// Returns an empty string if agentRationale is nil, empty, or all rationales are empty.
func CombineRationales(agentRationale map[string]string) string {
	if len(agentRationale) == 0 {
		return ""
	}

	var rationaleParts []string
	for agentName, rationale := range agentRationale {
		if rationale != "" {
			rationaleParts = append(rationaleParts, fmt.Sprintf("%s: %s", agentName, rationale))
		}
	}

	if len(rationaleParts) == 0 {
		return ""
	}

	return strings.Join(rationaleParts, "\n\n")
}
