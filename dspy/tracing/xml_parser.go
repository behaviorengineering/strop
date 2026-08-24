package tracing

import "github.com/behaviorengineering/strop/dspy/rawresponse"

// removeRawResponse removes raw LLM response keys from outputs.
// The XML interceptor has already parsed the XML into structured fields,
// so we just need to clean up the raw-response artifact for clean traces.
// Structured fields are already present in the outputs map (same as what application code uses).
func removeRawResponse(outputs map[string]interface{}) map[string]interface{} {
	if _, key := rawresponse.TextFromInterface(outputs); key == "" {
		return outputs // No cleanup needed.
	}

	// (application code extracts them directly, so we can use them too).
	result := make(map[string]interface{})
	for k, v := range outputs {
		if k != rawresponse.CanonicalKey && k != rawresponse.AlternateKey {
			result[k] = v
		}
	}
	return result
}

// RemoveRawResponseForTest exposes removeRawResponse for testing.
func RemoveRawResponseForTest(outputs map[string]interface{}) map[string]interface{} {
	return removeRawResponse(outputs)
}
