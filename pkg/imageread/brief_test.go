package imageread

import (
	"strings"
	"testing"
)

func TestVisualBrief_FormatPlan(t *testing.T) {
	b := &VisualBrief{
		SceneSummary: "Two fish in a glass tank",
		Subjects:     "Two orange fish",
		Setting:      "Fish tank",
		TeachingBeat: "Together in the tank, not blended",
		Source:       SourceVision,
	}
	got := b.FormatPlan()
	if got == "" {
		t.Fatal("expected non-empty plan")
	}
	for _, label := range []string{"SCENE:", "SUBJECTS:", "TEACHING BEAT:"} {
		if !strings.Contains(got, label) {
			t.Fatalf("plan missing %q: %q", label, got)
		}
	}
}
