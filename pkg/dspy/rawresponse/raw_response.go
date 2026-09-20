// Package rawresponse centralizes map keys used for unparsed LLM text in DSPy module outputs.
// Some paths use "__raw_response" (canonical); others use "_raw_response" — callers should use
// TextFrom / TextFromInterface so parsing and cleanup stay consistent.
package rawresponse

import "strings"

// CanonicalKey is the normalized key preserved on parsed outputs when raw text is kept.
const CanonicalKey = "__raw_response"

// AlternateKey is used by some dspy-go / provider paths instead of CanonicalKey.
const AlternateKey = "_raw_response"

func orderedKeys() []string {
	return []string{CanonicalKey, AlternateKey}
}

// TextFrom returns non-empty raw LLM text and which output key held it ("", if none).
func TextFrom(outputs map[string]any) (text string, sourceKey string) {
	if outputs == nil {
		return "", ""
	}
	for _, k := range orderedKeys() {
		if v, ok := outputs[k]; ok {
			if s, ok := v.(string); ok {
				s = strings.TrimSpace(s)
				if s != "" {
					return s, k
				}
			}
		}
	}
	return "", ""
}

// TextFromInterface is like TextFrom for map[string]interface{} (tracing and legacy callers).
func TextFromInterface(outputs map[string]interface{}) (text string, sourceKey string) {
	if outputs == nil {
		return "", ""
	}
	for _, k := range orderedKeys() {
		if v, ok := outputs[k]; ok {
			if s, ok := v.(string); ok {
				s = strings.TrimSpace(s)
				if s != "" {
					return s, k
				}
			}
		}
	}
	return "", ""
}
