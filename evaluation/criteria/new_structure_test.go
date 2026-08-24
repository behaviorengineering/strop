package criteria

import (
	"strings"
	"testing"
)

func TestNewCriterionStructure(t *testing.T) {
	registry := NewCriterionRegistry()

	// Test that all criteria have separate fields
	criterion, err := registry.Get(CriterionIDAntiFluffCompliance)
	if err != nil {
		t.Fatalf("Failed to get criterion: %v", err)
	}

	// Verify Description is concise
	if criterion.Description == "" {
		t.Error("Description should not be empty")
	}
	if strings.Contains(criterion.Description, "2 points:") {
		t.Error("Description should not contain scoring information")
	}

	// Verify Scoring has the rubric
	if criterion.Scoring == "" {
		t.Error("Scoring should not be empty")
	}
	if !strings.Contains(criterion.Scoring, "2 points:") {
		t.Error("Scoring should contain '2 points:'")
	}

	// Verify Examples has guidance
	if criterion.Examples == "" {
		t.Error("Examples should not be empty for output quality criteria")
	}
	if !strings.Contains(criterion.Examples, "FORBIDDEN") && !strings.Contains(criterion.Examples, "FIXES") {
		t.Error("Examples should contain pattern guidance")
	}
}

func TestBuildGeneratorGuidance(t *testing.T) {
	registry := NewCriterionRegistry()
	promptBuilder, err := NewPromptBuilder(registry)
	if err != nil {
		t.Fatalf("Failed to create prompt builder: %v", err)
	}

	guidance, err := promptBuilder.BuildGeneratorGuidance([]CriterionID{
		CriterionIDAntiFluffCompliance,
		CriterionIDAntiAIFeel,
	})
	if err != nil {
		t.Fatalf("Failed to build generator guidance: %v", err)
	}

	// Verify guidance includes examples
	if !strings.Contains(guidance, "OUTPUT QUALITY STANDARDS") {
		t.Error("Guidance should have header")
	}
	if !strings.Contains(guidance, "Anti-Fluff Compliance") {
		t.Error("Guidance should include criterion name")
	}
	if !strings.Contains(guidance, "FORBIDDEN") || !strings.Contains(guidance, "FIXES") {
		t.Error("Guidance should include examples")
	}
}

func TestBuildEvaluatorPrompt(t *testing.T) {
	registry := NewCriterionRegistry()
	promptBuilder, err := NewPromptBuilder(registry)
	if err != nil {
		t.Fatalf("Failed to create prompt builder: %v", err)
	}

	prompt, err := promptBuilder.BuildEvaluatorPrompt(
		"Style Editor",
		"format and recipe compliance",
		[]CriterionID{
			CriterionIDAntiFluffCompliance,
			CriterionIDAntiAIFeel,
		},
	)
	if err != nil {
		t.Fatalf("Failed to build evaluator prompt: %v", err)
	}

	// Verify prompt includes scoring
	if !strings.Contains(prompt, "2 points:") {
		t.Error("Prompt should include scoring rubric")
	}
	if !strings.Contains(prompt, "Anti-Fluff Compliance") {
		t.Error("Prompt should include criterion name")
	}
}

func TestParseScoringLevels(t *testing.T) {
	registry := NewCriterionRegistry()
	promptBuilder, err := NewPromptBuilder(registry)
	if err != nil {
		t.Fatalf("Failed to create prompt builder: %v", err)
	}

	scoring := `2 points: No violations, excellent.
1 point: Some minor violations.
0 points: Major violations.`

	levels := promptBuilder.parseScoringLevels(scoring)

	if len(levels) != 3 {
		t.Errorf("Expected 3 scoring levels, got %d", len(levels))
	}

	// Verify levels are in correct order and contain expected text
	// Note: parser formats all as "{score} points:" for consistency
	if !strings.HasPrefix(levels[0], "2 points:") {
		t.Errorf("First level should start with '2 points:', got: %s", levels[0])
	}
	if !strings.HasPrefix(levels[1], "1 points:") {
		t.Errorf("Second level should start with '1 points:', got: %s", levels[1])
	}
	if !strings.HasPrefix(levels[2], "0 points:") {
		t.Errorf("Third level should start with '0 points:', got: %s", levels[2])
	}
}
