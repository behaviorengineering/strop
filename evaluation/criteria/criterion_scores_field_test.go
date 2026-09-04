package criteria

import (
	"strings"
	"testing"
)

func TestParseCriterionIDsFromMappingPrompt(t *testing.T) {
	t.Parallel()
	prompt, err := BuildScoreGenerationPromptFromCriteria(
		"Process Evaluator",
		[]CriterionID{CriterionIDInstructionCompliance, CriterionIDCompleteness, CriterionIDFeedbackAdherence},
		"",
	)
	if err != nil {
		t.Fatalf("BuildScoreGenerationPromptFromCriteria: %v", err)
	}
	got := ParseCriterionIDsFromMappingPrompt(prompt)
	want := []CriterionID{CriterionIDInstructionCompliance, CriterionIDCompleteness, CriterionIDFeedbackAdherence}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestCriterionScoresOutputDescription_includesExactMapKeys(t *testing.T) {
	t.Parallel()
	desc := CriterionScoresOutputDescription([]CriterionID{
		CriterionIDInstructionCompliance,
		CriterionIDCompleteness,
	})
	if !strings.Contains(desc, exactMapKeysMarker+" instruction_compliance, completeness.") {
		t.Fatalf("unexpected description: %q", desc)
	}
	if !strings.Contains(desc, "plain decimal number") {
		t.Fatalf("expected numeric score guidance in description: %q", desc)
	}
}

func TestCriterionScoresOutputDescription_emptyIDsOmitsExactKeys(t *testing.T) {
	t.Parallel()
	desc := CriterionScoresOutputDescription(nil)
	if strings.Contains(desc, exactMapKeysMarker) {
		t.Fatalf("expected no exact map keys marker, got %q", desc)
	}
}

func TestScoreGenerationPromptBase_RequiresPlainDecimals(t *testing.T) {
	t.Parallel()
	if !strings.Contains(ScoreGenerationPromptBase, "plain decimal number") {
		t.Fatalf("ScoreGenerationPromptBase missing numeric score rule")
	}
	if !strings.Contains(ScoreGenerationPromptBase, "checklist") {
		t.Fatalf("ScoreGenerationPromptBase missing checklist-ban rule")
	}
	if strings.Contains(ScoreGenerationPromptBase, "2.0/2.0") {
		t.Fatalf("ScoreGenerationPromptBase must not teach slash score shapes like 2.0/2.0")
	}
	if strings.Contains(ScoreGenerationPromptBase, "assign ./.") {
		t.Fatalf("ScoreGenerationPromptBase must not instruct assigning ./.")
	}
	if !strings.Contains(ScoreGenerationPromptBase, "<![CDATA[2.0]]>") {
		t.Fatalf("ScoreGenerationPromptBase missing decimal XML example")
	}
	if !strings.Contains(ScoreGenerationPromptBase, "Map feedback [✓]") {
		t.Fatalf("ScoreGenerationPromptBase missing [✓]→2.0 mapping rule")
	}
}
