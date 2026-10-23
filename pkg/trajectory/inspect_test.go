package trajectory

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestInspect_readOnlyDoesNotMutatePlan(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := t.TempDir()
	id := Identity{
		Pipeline:          "essays",
		Job:               "essays_composition_generation",
		EntityID:          "00000000-0000-0000-0000-000000000001",
		Version:           1,
		DefinitionVersion: DefinitionVersion,
		SourceFingerprint: "fp",
	}
	steps := []StepSpec{
		{ID: "hook", Goal: "hook"},
		{ID: "fork", Goal: "fork"},
	}
	sess, err := Open(ctx, OpenOptions{Root: root, ID: id, Steps: steps})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]string{"ok": "1"})
	if err := sess.SaveCompleteEvidence(ctx, "hook", Evidence{Output: raw, Score: 8}); err != nil {
		t.Fatal(err)
	}
	planPath := filepath.Join(sess.PlanDir, "plan.json")
	before, err := os.Stat(planPath)
	if err != nil {
		t.Fatal(err)
	}

	inspected, err := Inspect(ctx, root, sess.Plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if inspected == nil {
		t.Fatal("expected session")
	}
	sums, err := inspected.StepSummaries(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(sums) != 2 || sums[0].Status != "complete" || sums[1].Status != "incomplete" {
		t.Fatalf("sums=%+v", sums)
	}

	after, err := os.Stat(planPath)
	if err != nil {
		t.Fatal(err)
	}
	if after.ModTime().After(before.ModTime()) {
		t.Fatalf("plan.json mtime changed before=%v after=%v", before.ModTime(), after.ModTime())
	}
}

func TestHighestVersionPlanID(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := t.TempDir()
	for _, spec := range []struct {
		ver int
	}{
		{1}, {3}, {2},
	} {
		id := Identity{
			Pipeline:          "essays",
			Job:               "job",
			EntityID:          "piece",
			Version:           spec.ver,
			DefinitionVersion: DefinitionVersion,
			SourceFingerprint: "fp",
		}
		_, err := Open(ctx, OpenOptions{
			Root:  root,
			ID:    id,
			Steps: []StepSpec{{ID: "a", Goal: "a"}},
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	prefix := PlanIDPrefix(Identity{Pipeline: "essays", Job: "job", EntityID: "piece", Version: 1})
	got, err := HighestVersionPlanID(ctx, root, prefix)
	if err != nil {
		t.Fatal(err)
	}
	if planIDVersion(got) != 3 {
		t.Fatalf("got %q", got)
	}
}
