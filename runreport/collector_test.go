package runreport

import (
	"errors"
	"testing"
	"time"
)

func TestCollector_RecordPerItemRefinement(t *testing.T) {
	c := newCollector(NewMeta("youtube", "chapter_quotes", "vid-1", 3))
	c.RecordPerItemRefinement(2, 1, 7.5, "chapter 3 round 1")
	report := c.snapshot(OutcomeSuccess, "")
	if len(report.Steps) != 1 {
		t.Fatalf("steps = %d, want 1", len(report.Steps))
	}
	if report.Steps[0].Details["item_index"] != 2 {
		t.Fatalf("item_index = %v", report.Steps[0].Details["item_index"])
	}
}

func TestCollector_RecordsSteps(t *testing.T) {
	c := newCollector(NewMeta("youtube", "chapters", "vid-1", 0))
	c.RecordPhase("skim", 1, false, 6.5, "retry")
	c.RecordModule("PostGenerator", true, nil, 120*time.Millisecond)
	c.RecordEvaluator("StyleEditor", "warmth", false, map[string]interface{}{"parse_ok": false})
	c.RecordAlignment("depth", []string{"voice mismatch"})
	c.RecordWarning("XMLParser", "malformed tag")
	c.RecordHealing(8.0, 6.0, "score drop")

	report := c.snapshot(OutcomeSuccess, "")
	if len(report.Steps) != 6 {
		t.Fatalf("steps = %d, want 6", len(report.Steps))
	}
	if report.Steps[0].Kind != StepCompositionPhase {
		t.Fatalf("first step kind = %q", report.Steps[0].Kind)
	}
}

func TestCollector_RecordModuleCapturesError(t *testing.T) {
	c := newCollector(NewMeta("sayings", "translation", "id", 0))
	err := errors.New("timeout")
	c.RecordModule("Translator", false, err, time.Second)
	report := c.snapshot(OutcomeFailed, err.Error())
	if report.Steps[0].Error != "timeout" {
		t.Fatalf("error = %q", report.Steps[0].Error)
	}
}
