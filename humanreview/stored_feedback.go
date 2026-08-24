package humanreview

import "strings"

const storedFeedbackUserPrefix = "User said: "

// BuildStoredFeedbackForRejection builds the canonical stored format for rejection
// feedback: "User said: <raw>" then a blank line, then the structured feedback.
func BuildStoredFeedbackForRejection(rawFeedback, structuredFeedback string) string {
	return storedFeedbackUserPrefix + rawFeedback + "\n\n" + structuredFeedback
}

// ExtractStructuredFeedbackFromStored returns the structured part of a stored feedback
// string (the part after "User said: ...\n\n"). If the stored string does not match
// the canonical format, it returns stored as-is.
func ExtractStructuredFeedbackFromStored(stored string) string {
	if !strings.HasPrefix(stored, storedFeedbackUserPrefix) {
		return stored
	}
	idx := strings.Index(stored[len(storedFeedbackUserPrefix):], "\n\n")
	if idx < 0 {
		return stored
	}
	return stored[len(storedFeedbackUserPrefix)+idx+2:]
}
