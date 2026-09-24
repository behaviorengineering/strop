package trajectory_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/behaviorengineering/strop/pkg/orchestration"
	"github.com/behaviorengineering/strop/pkg/stepplan"
	"github.com/behaviorengineering/strop/pkg/trajectory"
)

func TestOpenResumeSkipsCompleted(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := t.TempDir()
	id := trajectory.Identity{
		Pipeline:          "pipeline",
		Job:               "job",
		EntityID:          "entity-1",
		Version:           1,
		DefinitionVersion: trajectory.DefinitionVersion,
		SourceFingerprint: "src-a",
		Labels:            map[string]string{"arc_id": "trap_fork"},
	}
	steps := []trajectory.StepSpec{
		{ID: "hook", Goal: "hook"},
		{ID: "middle", Goal: "middle"},
		{ID: "end", Goal: "end"},
	}
	sess, err := trajectory.Open(ctx, trajectory.OpenOptions{Root: root, ID: id, Steps: steps})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]string{"title": "T"})
	if err := sess.SaveCompleteEvidence(ctx, "hook", trajectory.Evidence{
		Score:  8,
		Output: raw,
	}); err != nil {
		t.Fatal(err)
	}

	sess2, err := trajectory.Open(ctx, trajectory.OpenOptions{Root: root, ID: id, Steps: steps})
	if err != nil {
		t.Fatal(err)
	}
	if !sess2.Resumed {
		t.Fatal("expected resumed session")
	}
	if sess2.ResumedFrom != "middle" {
		t.Fatalf("resumed_from=%q", sess2.ResumedFrom)
	}
	if !sess2.IsStepComplete(ctx, "hook") {
		t.Fatal("hook should be complete")
	}
	if sess2.IsStepComplete(ctx, "middle") {
		t.Fatal("middle should be incomplete")
	}
}

func TestOpenRejectsLabelMismatch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := t.TempDir()
	id := trajectory.Identity{
		Pipeline:          "pipeline",
		Job:               "job",
		EntityID:          "entity-1",
		Version:           1,
		DefinitionVersion: trajectory.DefinitionVersion,
		SourceFingerprint: "src-a",
		Labels:            map[string]string{"arc_id": "trap_fork"},
	}
	steps := []trajectory.StepSpec{{ID: "hook", Goal: "hook"}}
	if _, err := trajectory.Open(ctx, trajectory.OpenOptions{Root: root, ID: id, Steps: steps}); err != nil {
		t.Fatal(err)
	}
	id.Labels["arc_id"] = "other_arc"
	if _, err := trajectory.Open(ctx, trajectory.OpenOptions{Root: root, ID: id, Steps: steps}); err == nil {
		t.Fatal("expected label mismatch")
	}
}

func TestOpenRejectsDefinitionMismatch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := t.TempDir()
	id := trajectory.Identity{
		Pipeline:          "pipeline",
		Job:               "job",
		EntityID:          "entity-def",
		Version:           1,
		DefinitionVersion: 1,
		SourceFingerprint: "src",
	}
	steps := []trajectory.StepSpec{{ID: "a", Goal: "a"}}
	if _, err := trajectory.Open(ctx, trajectory.OpenOptions{Root: root, ID: id, Steps: steps}); err != nil {
		t.Fatal(err)
	}
	id.DefinitionVersion = 2
	if _, err := trajectory.Open(ctx, trajectory.OpenOptions{Root: root, ID: id, Steps: steps}); err == nil {
		t.Fatal("expected definition_version mismatch")
	}
}

func TestSourceFingerprintChangeInvalidates(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := t.TempDir()
	id := trajectory.Identity{
		Pipeline:          "pipeline",
		Job:               "job",
		EntityID:          "entity-2",
		Version:           1,
		DefinitionVersion: trajectory.DefinitionVersion,
		SourceFingerprint: "old",
	}
	steps := []trajectory.StepSpec{{ID: "produce", Goal: "produce"}}
	sess, err := trajectory.Open(ctx, trajectory.OpenOptions{Root: root, ID: id, Steps: steps})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{"ok": true})
	if err := sess.SaveCompleteEvidence(ctx, "produce", trajectory.Evidence{Output: raw, Score: 9}); err != nil {
		t.Fatal(err)
	}
	id.SourceFingerprint = "new"
	sess2, err := trajectory.Open(ctx, trajectory.OpenOptions{Root: root, ID: id, Steps: steps})
	if err != nil {
		t.Fatal(err)
	}
	if sess2.IsStepComplete(ctx, "produce") {
		t.Fatal("produce must be incomplete after source change")
	}
}

func TestRunStepPlanThroughSession(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := t.TempDir()
	id := trajectory.Identity{
		Pipeline:          "pipeline",
		Job:               "job",
		EntityID:          "entity-3",
		Version:           1,
		DefinitionVersion: trajectory.DefinitionVersion,
		SourceFingerprint: "src",
	}
	steps := []trajectory.StepSpec{
		{ID: "hook", Goal: "hook", RequiredKeys: []string{"output"}},
		{ID: "end", Goal: "end", RequiredKeys: []string{"output"}},
	}
	sess, err := trajectory.Open(ctx, trajectory.OpenOptions{Root: root, ID: id, Steps: steps})
	if err != nil {
		t.Fatal(err)
	}
	var ran []string
	runner := orchestration.StepRunnerFunc(func(ctx context.Context, plan *stepplan.Plan, step stepplan.Step) (*orchestration.StepRunResult, error) {
		ran = append(ran, step.ID)
		body, err := trajectory.MarshalEvidence(trajectory.Evidence{
			Score:  8,
			Output: json.RawMessage(`{"phase":"` + step.ID + `"}`),
		})
		if err != nil {
			return nil, err
		}
		return &orchestration.StepRunResult{Output: body}, nil
	})
	if _, err := sess.Run(ctx, runner, orchestration.StepPlanConfig{MaxAttemptsPerStep: 1}, nil); err != nil {
		t.Fatal(err)
	}
	if len(ran) != 2 {
		t.Fatalf("ran=%v", ran)
	}

	sess2, err := trajectory.Open(ctx, trajectory.OpenOptions{Root: root, ID: id, Steps: steps})
	if err != nil {
		t.Fatal(err)
	}
	ran = nil
	res, err := sess2.Run(ctx, runner, orchestration.StepPlanConfig{MaxAttemptsPerStep: 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(ran) != 0 {
		t.Fatalf("expected skip, ran=%v", ran)
	}
	if len(res.SkippedSteps) != 2 {
		t.Fatalf("skipped=%v", res.SkippedSteps)
	}
	planPath := filepath.Join(root, "plans", trajectory.PlanID(id), "plan.json")
	if _, err := stepplan.NewFileStore(root); err != nil {
		t.Fatal(err)
	}
	_ = planPath
}

func TestForceNewStartsFresh(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := t.TempDir()
	id := trajectory.Identity{
		Pipeline:          "pipeline",
		Job:               "job",
		EntityID:          "entity-4",
		Version:           1,
		DefinitionVersion: trajectory.DefinitionVersion,
		SourceFingerprint: "src",
	}
	steps := []trajectory.StepSpec{{ID: "hook", Goal: "hook"}}
	sess, err := trajectory.Open(ctx, trajectory.OpenOptions{Root: root, ID: id, Steps: steps})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]string{"ok": "1"})
	if err := sess.SaveCompleteEvidence(ctx, "hook", trajectory.Evidence{Output: raw}); err != nil {
		t.Fatal(err)
	}
	sess2, err := trajectory.Open(ctx, trajectory.OpenOptions{Root: root, ID: id, Steps: steps, ForceNew: true})
	if err != nil {
		t.Fatal(err)
	}
	if sess2.Resumed {
		t.Fatal("force new must not resume")
	}
	if sess2.IsStepComplete(ctx, "hook") {
		t.Fatal("force new must clear checkpoints")
	}
}

func TestEvidenceMetadataRoundTrip(t *testing.T) {
	t.Parallel()
	raw, err := trajectory.MarshalEvidence(trajectory.Evidence{
		Score:    7,
		Output:   json.RawMessage(`{"x":1}`),
		Metadata: map[string]string{"demo_near_id": "n1", "demo_contrast_id": "c1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	ev, err := trajectory.UnmarshalEvidence(raw)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Metadata["demo_near_id"] != "n1" || ev.Metadata["demo_contrast_id"] != "c1" {
		t.Fatalf("metadata=%v", ev.Metadata)
	}
}
