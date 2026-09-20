package humanreview

import "testing"

func TestBuildAndExtractStoredFeedback(t *testing.T) {
	stored := BuildStoredFeedbackForRejection("too generic", "Keep the title specific.")
	if stored != "User said: too generic\n\nKeep the title specific." {
		t.Fatalf("stored = %q", stored)
	}
	got := ExtractStructuredFeedbackFromStored(stored)
	if got != "Keep the title specific." {
		t.Fatalf("extracted = %q", got)
	}
}

func TestExtractStructuredFeedbackFromStored_passthrough(t *testing.T) {
	raw := "plain comment"
	if got := ExtractStructuredFeedbackFromStored(raw); got != raw {
		t.Fatalf("got %q", got)
	}
}
