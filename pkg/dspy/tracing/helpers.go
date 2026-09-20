package tracing

import (
	"fmt"
	"strings"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
)

// getFieldNames extracts field names from a map (works for both inputs and outputs).
func getFieldNames(fields map[string]any) []string {
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	return keys
}

// addDSPySpanAnnotations safely adds annotations to a DSPy span if it's not nil.
func addDSPySpanAnnotations(dspySpan *core.Span, annotations map[string]interface{}) {
	if dspySpan == nil {
		return
	}
	for key, value := range annotations {
		dspySpan.WithAnnotation(key, value)
	}
}

// convertMap converts map[string]any to map[string]interface{} for compatibility.
func convertMap(m map[string]any) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range m {
		result[k] = v
	}
	return result
}

// buildSpanName creates a descriptive span name from module type, name, and content version
// Examples:
//   - generator + TranslationGenerator + v3 -> "generator.TranslationGenerator.v3"
//   - evaluator + LinkedIn Professional Evaluator + v2 -> "evaluator.LinkedInProfessional.v2"
//   - evaluator + Feedback Consolidator + v1 -> "evaluator.FeedbackConsolidator.v1"
func buildSpanName(moduleType, moduleName string, contentVersion int) string {
	if moduleName == defaultModuleName {
		if moduleType == defaultModuleType {
			if contentVersion > 0 {
				return fmt.Sprintf("module.process.v%d", contentVersion)
			}
			return "module.process"
		}
		if contentVersion > 0 {
			return fmt.Sprintf("%s.process.v%d", moduleType, contentVersion)
		}
		return fmt.Sprintf("%s.process", moduleType)
	}

	// Infer type from name if type is unknown.
	inferredType := moduleType
	if moduleType == defaultModuleType {
		switch {
		case strings.HasSuffix(moduleName, "Generator"):
			inferredType = moduleTypeGenerator
		case strings.Contains(moduleName, "Evaluator") || strings.Contains(moduleName, "Consolidator"):
			inferredType = moduleTypeEvaluator
		default:
			inferredType = moduleTypeModule
		}
	}

	// Clean up module name for span name (remove spaces, "Evaluator" suffix, etc.)
	cleanName := cleanModuleName(moduleName)

	// Add content version if available.
	if contentVersion > 0 {
		return fmt.Sprintf("%s.%s.v%d", inferredType, cleanName, contentVersion)
	}

	return fmt.Sprintf("%s.%s", inferredType, cleanName)
}

// extractVersionFromValue extracts an integer version from a value that may be int, float64, or string.
// This handles type conversions that may occur during JSON unmarshaling.
func extractVersionFromValue(version interface{}) int {
	if version == nil {
		return 0
	}
	// Try int first.
	if v, ok := version.(int); ok {
		return v
	}
	// Handle float64 case (JSON unmarshaling might convert int to float64).
	if v, ok := version.(float64); ok {
		return int(v)
	}
	// Handle string case (in case it's stored as string).
	if vStr, ok := version.(string); ok {
		var v int
		if _, err := fmt.Sscanf(vStr, "%d", &v); err == nil {
			return v
		}
	}
	return 0
}

// extractContentVersion extracts the content version (refinement iteration) from inputs.
// All modules now use a consistent top-level "iterationVersion" field.
func extractContentVersion(inputs map[string]any) int {
	if inputs == nil {
		return 0
	}

	// Check for consistent iterationVersion field at top level (all modules now use this).
	if iterationVersion, ok := inputs[iterationVersionKey]; ok {
		if v := extractVersionFromValue(iterationVersion); v > 0 {
			return v
		}
	}

	return 0
}

// cleanModuleName cleans up module names for use in span names
// Examples:
//   - "TranslationGenerator" -> "TranslationGenerator"
//   - "LinkedIn Professional Evaluator" -> "LinkedInProfessional"
//   - "Feedback Consolidator" -> "FeedbackConsolidator"
func cleanModuleName(name string) string {
	// Remove "Evaluator" suffix if present.
	name = strings.TrimSuffix(name, " Evaluator")
	name = strings.TrimSuffix(name, "Evaluator")

	// Remove "Consolidator" suffix if present.
	name = strings.TrimSuffix(name, " Consolidator")
	name = strings.TrimSuffix(name, "Consolidator")

	// Remove spaces and make it a valid identifier.
	name = strings.ReplaceAll(name, " ", "")

	return name
}
