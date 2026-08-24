package factory

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	kitdspy "github.com/behaviorengineering/strop/dspy"
)

func TestExtractGeminiCandidateText_singlePart(t *testing.T) {
	got := extractGeminiCandidateText([]geminiResponsePart{{Text: "<response><topic>a</topic></response>"}})
	assert.Equal(t, "<response><topic>a</topic></response>", got)
}

func TestExtractGeminiCandidateText_skipsThoughtParts(t *testing.T) {
	th := true
	got := extractGeminiCandidateText([]geminiResponsePart{
		{Text: "internal reasoning", Thought: &th},
		{Text: "<response><topic>x</topic></response>"},
	})
	assert.Equal(t, "<response><topic>x</topic></response>", got)
}

func TestExtractGeminiCandidateText_concatMultipleNonThought(t *testing.T) {
	got := extractGeminiCandidateText([]geminiResponsePart{
		{Text: "line1\n"},
		{Text: "line2"},
	})
	assert.Equal(t, "line1\n\nline2", got)
}

func TestExtractGeminiCandidateText_fallbackWhenOnlyThought(t *testing.T) {
	th := true
	got := extractGeminiCandidateText([]geminiResponsePart{
		{Text: "only thought body", Thought: &th},
	})
	assert.Equal(t, "only thought body", got)
}

func TestExtractGeminiCandidateText_emptyParts(t *testing.T) {
	assert.Equal(t, "", extractGeminiCandidateText(nil))
	assert.Equal(t, "", extractGeminiCandidateText([]geminiResponsePart{{Text: "  "}}))
}

func TestNewGroundingLLMWrapper_usesProviderTimeout(t *testing.T) {
	wrapper := NewGroundingLLMWrapper(
		nil,
		&kitdspy.GroundingConfig{},
		"key",
		"https://generativelanguage.googleapis.com",
		"gemini-2.5-flash",
		60*time.Second,
		nil,
	)
	assert.Equal(t, 60*time.Second, wrapper.httpClient.Timeout)
}

func TestNewGroundingLLMWrapper_zeroTimeoutUsesAttemptDefault(t *testing.T) {
	wrapper := NewGroundingLLMWrapper(nil, &kitdspy.GroundingConfig{}, "key", "https://example", "model", 0, nil)
	assert.Equal(t, 60*time.Second, wrapper.httpClient.Timeout)
}
