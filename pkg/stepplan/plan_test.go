package stepplan

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewPlanAndValidate(t *testing.T) {
	t.Parallel()
	steps := []Step{
		{
			ID:   "pkg_a_evidence",
			Goal: "Distill evidence for package a",
			InputRefs: []InputRef{
				{Key: "package", Hash: "abc123"},
			},
			Budget: Budget{Timeout: time.Minute, MaxIterations: 4, MaxChars: 8000},
			DoneCriteria: DoneCriteria{
				RequiredKeys: []string{"evidence"},
			},
		},
		{
			ID:   "synthesize",
			Goal: "Synthesize slice objective",
			InputRefs: []InputRef{
				{Key: "evidence_a", Hash: "abc123"},
			},
		},
	}
	plan, err := NewPlan("slice_operations", "slice_grounding", steps, map[string]any{
		"slice": "operations",
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Status != PlanStatusOpen {
		t.Fatalf("status=%q", plan.Status)
	}
	if plan.Steps[0].EffectiveCheckpointKey() != "pkg_a_evidence" {
		t.Fatalf("checkpoint key=%q", plan.Steps[0].EffectiveCheckpointKey())
	}

	_, err = NewPlan("", "k", steps, nil)
	if err == nil {
		t.Fatal("expected empty plan id to fail")
	}
	_, err = NewPlan("bad/id", "k", steps, nil)
	if err == nil {
		t.Fatal("expected invalid plan id to fail")
	}
	dup := append([]Step{}, steps...)
	dup = append(dup, Step{ID: "pkg_a_evidence", Goal: "dup"})
	_, err = NewPlan("p1", "k", dup, nil)
	if err == nil {
		t.Fatal("expected duplicate step id to fail")
	}
}

func TestFileStoreSaveLoadResume(t *testing.T) {
	t.Parallel()
	store, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	plan, err := NewPlan("slice_ops", "slice_grounding", []Step{
		{ID: "evidence_pkg_a", Goal: "Evidence A", InputRefs: []InputRef{{Key: "pkg", Hash: "h1"}}},
		{ID: "evidence_pkg_b", Goal: "Evidence B", InputRefs: []InputRef{{Key: "pkg", Hash: "h2"}}},
		{ID: "synthesize", Goal: "Synthesize"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SavePlan(ctx, plan); err != nil {
		t.Fatal(err)
	}

	loaded, err := store.LoadPlan(ctx, plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ID != plan.ID || len(loaded.Steps) != 3 {
		t.Fatalf("loaded=%+v", loaded)
	}
	planPath := filepath.Join(store.PlansDir(), plan.ID, FilePlan)
	if _, err := os.Stat(planPath); err != nil {
		t.Fatalf("plan.json missing: %v", err)
	}

	out, err := json.Marshal(map[string]any{"evidence": "pkg a notes"})
	if err != nil {
		t.Fatal(err)
	}
	cp, err := NewCompleteCheckpoint(plan.ID, plan.Steps[0], out, &TokenUsage{TotalTokens: 42})
	if err != nil {
		t.Fatal(err)
	}
	if cp.InputFingerprint != "pkg=h1" {
		t.Fatalf("fingerprint=%q", cp.InputFingerprint)
	}
	if err := store.SaveStep(ctx, cp); err != nil {
		t.Fatal(err)
	}

	got, err := store.LoadStep(ctx, plan.ID, "evidence_pkg_a")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StepStatusComplete || got.Usage.TotalTokens != 42 {
		t.Fatalf("checkpoint=%+v", got)
	}

	_, err = store.LoadStep(ctx, plan.ID, "evidence_pkg_b")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	completed, err := store.ListCompleted(ctx, plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(completed) != 1 || completed[0] != "evidence_pkg_a" {
		t.Fatalf("completed=%v", completed)
	}

	// Resume path: reload plan, skip completed, continue at first incomplete.
	resume, err := store.LoadPlan(ctx, plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	done, err := store.ListCompleted(ctx, resume.ID)
	if err != nil {
		t.Fatal(err)
	}
	doneSet := map[string]struct{}{}
	for _, k := range done {
		doneSet[k] = struct{}{}
	}
	var next string
	for _, step := range resume.Steps {
		if _, ok := doneSet[step.EffectiveCheckpointKey()]; !ok {
			next = step.ID
			break
		}
	}
	if next != "evidence_pkg_b" {
		t.Fatalf("next step=%q", next)
	}
}

func TestFileStoreFailedCheckpointExcludedFromCompleted(t *testing.T) {
	t.Parallel()
	store, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	plan, err := NewPlan("p_fail", "test", []Step{
		{ID: "s1", Goal: "one"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SavePlan(ctx, plan); err != nil {
		t.Fatal(err)
	}
	cp, err := NewFailedCheckpoint(plan.ID, plan.Steps[0], "deadline exceeded")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveStep(ctx, cp); err != nil {
		t.Fatal(err)
	}
	completed, err := store.ListCompleted(ctx, plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(completed) != 0 {
		t.Fatalf("failed step must not count as completed: %v", completed)
	}
}

func TestNewFileStoreRequiresRoot(t *testing.T) {
	t.Parallel()
	if _, err := NewFileStore(""); err == nil {
		t.Fatal("expected empty root to fail")
	}
}

func TestFileStoreDeleteStepsAfter(t *testing.T) {
	t.Parallel()
	store, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	plan, err := NewPlan("p_del", "test", []Step{
		{ID: "s1", Goal: "one"},
		{ID: "s2", Goal: "two"},
		{ID: "s3", Goal: "three"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SavePlan(ctx, plan); err != nil {
		t.Fatal(err)
	}
	for _, step := range plan.Steps {
		cp, err := NewCompleteCheckpoint(plan.ID, step, json.RawMessage(`{"ok":true}`), nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.SaveStep(ctx, cp); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.DeleteStepsAfter(ctx, plan, 1); err != nil {
		t.Fatal(err)
	}
	completed, err := store.ListCompleted(ctx, plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(completed) != 1 || completed[0] != "s1" {
		t.Fatalf("completed=%v want [s1]", completed)
	}
	if _, err := store.LoadStep(ctx, plan.ID, "s2"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("s2 load err=%v", err)
	}
}

func TestInvalidateFromRequiresCapabilityForDownstream(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	plan, err := NewPlan("p_inv", "test", []Step{
		{ID: "s1", Goal: "one"},
		{ID: "s2", Goal: "two"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	store := &minimalStore{}
	err = InvalidateFrom(ctx, store, plan, 0)
	if !errors.Is(err, ErrInvalidationUnsupported) {
		t.Fatalf("err=%v want ErrInvalidationUnsupported", err)
	}
	// Last step only: missing capability is a no-op.
	if err := InvalidateFrom(ctx, store, plan, 1); err != nil {
		t.Fatal(err)
	}
}

func TestAsCheckpointInvalidatorFileStore(t *testing.T) {
	t.Parallel()
	store, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	inv, ok := AsCheckpointInvalidator(store)
	if !ok || inv == nil {
		t.Fatal("FileStore must implement CheckpointInvalidator")
	}
}

// minimalStore implements Store without CheckpointInvalidator.
type minimalStore struct{}

func (m *minimalStore) SavePlan(context.Context, *Plan) error { return nil }
func (m *minimalStore) LoadPlan(context.Context, string) (*Plan, error) {
	return nil, ErrNotFound
}
func (m *minimalStore) SaveStep(context.Context, *Checkpoint) error { return nil }
func (m *minimalStore) LoadStep(context.Context, string, string) (*Checkpoint, error) {
	return nil, ErrNotFound
}
func (m *minimalStore) ListCompleted(context.Context, string) ([]string, error) {
	return nil, nil
}
