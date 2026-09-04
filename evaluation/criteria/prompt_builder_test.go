package criteria

import (
	"strings"
	"testing"
)

// TestPromptBuilder_GeneratesPromptFromRegistry demonstrates that prompts are generated from registry.
func TestPromptBuilder_GeneratesPromptFromRegistry(t *testing.T) {
	registry := NewCriterionRegistry()
	promptBuilder, err := NewPromptBuilder(registry)
	if err != nil {
		t.Fatalf("Failed to create prompt builder: %v", err)
	}

	// Use the same criterion IDs as TextEvaluator.
	criterionIDs := []CriterionID{
		CriterionIDInstructionCompliance,
		CriterionIDCompleteness,
		CriterionIDFeedbackAdherence,
		CriterionIDContextAwareness,
		CriterionIDOutputQuality,
		CriterionIDClarityReadability,
	}

	prompt, err := promptBuilder.BuildEvaluatorPrompt(
		"Cultural Anthropologist",
		"cultural authenticity, nuance preservation, context accuracy, and cultural sensitivity",
		criterionIDs,
	)
	if err != nil {
		t.Fatalf("Failed to build prompt: %v", err)
	}

	// Verify key sections are present and generated from registry.
	checks := []struct {
		name     string
		contains string
		desc     string
	}{
		{
			name:     "Criterion ID Mapping",
			contains: `Instruction Compliance → "instruction_compliance"`,
			desc:     "Should contain criterion ID mapping generated from registry",
		},
		{
			name:     "Registry Description",
			contains: "All instructions followed, all basic requirements met",
			desc:     "Should contain description text from registry (not hardcoded)",
		},
		{
			name:     "Point Totals",
			contains: "12 points total",
			desc:     "Should calculate total points from registry (6 criteria × 2 points = 12)",
		},
		{
			name:     "Category Grouping",
			contains: "PROCESS EVALUATION (8 points)",
			desc:     "Should group and calculate process criteria points (4 criteria × 2 points = 8)",
		},
		{
			name:     "Category Grouping",
			contains: "OUTPUT QUALITY EVALUATION (4 points)",
			desc:     "Should group and calculate quality criteria points (2 criteria × 2 points = 4)",
		},
		{
			name:     "Score Breakdown",
			contains: "Instruction Compliance: {points} of 2 max",
			desc:     "Should generate score breakdown from registry names and max points",
		},
		{
			name:     "Registry Name",
			contains: "Completeness",
			desc:     "Should use criterion name from registry",
		},
	}

	t.Log("")
	t.Log("=== GENERATED PROMPT (Evidence that it's from Registry) ===")
	t.Log("")
	t.Log("Key Sections Generated from Registry:")
	t.Log(strings.Repeat("=", 80))

	for _, check := range checks {
		if strings.Contains(prompt, check.contains) {
			t.Logf("✓ %s: FOUND", check.name)
			t.Logf("  %s", check.desc)
			t.Logf("  Contains: %s\n", check.contains)
		} else {
			t.Errorf("❌ %s: NOT FOUND\n  Expected: %s", check.name, check.contains)
		}
	}

	t.Log("")
	t.Log("=== SAMPLE OF GENERATED PROMPT ===")
	t.Log(strings.Repeat("=", 80))

	// Show a sample section.
	lines := strings.Split(prompt, "\n")
	startIdx := 0
	for i, line := range lines {
		if strings.Contains(line, "CRITERION ID MAPPING") {
			startIdx = i
			break
		}
	}

	// Show 30 lines starting from CRITERION ID MAPPING.
	endIdx := startIdx + 30
	if endIdx > len(lines) {
		endIdx = len(lines)
	}

	for i := startIdx; i < endIdx; i++ {
		t.Log(lines[i])
	}

	t.Log("... (prompt continues) ...")
}

func TestPromptBuilder_CompletenessEvidenceDoesNotRequireGeneratorRationale(t *testing.T) {
	pb, err := NewPromptBuilder(DefaultRegistry())
	if err != nil {
		t.Fatalf("NewPromptBuilder: %v", err)
	}
	got := pb.getEvidencePlaceholder(CriterionDescription{ID: CriterionIDCompleteness})
	if strings.Contains(strings.ToLower(got), "check rationale") {
		t.Fatalf("completeness evidence must not tell evaluators to check a rationale field: %q", got)
	}
	if !strings.Contains(got, "required generator_output fields") {
		t.Fatalf("expected required-fields wording, got %q", got)
	}

	prompt, err := pb.BuildEvaluatorPrompt(
		"Process Evaluator",
		"instruction compliance, completeness, feedback adherence",
		[]CriterionID{CriterionIDInstructionCompliance, CriterionIDCompleteness, CriterionIDFeedbackAdherence},
	)
	if err != nil {
		t.Fatalf("BuildEvaluatorPrompt: %v", err)
	}
	if strings.Contains(prompt, "Check rationale for task understanding") {
		t.Fatal("process/feedback prompt must not include stale completeness rationale check")
	}
	if !strings.Contains(prompt, "CRITICAL ANALYSIS PHASE (in directives_ack)") {
		t.Fatal("expected directives_ack analysis phase wording")
	}
}
