package orchestration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/behaviorengineering/strop/stepplan"
)

func TestRunStepPlanResumeSkipsCompleted(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store, err := stepplan.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	plan, err := stepplan.NewPlan("resume_plan", "test", []stepplan.Step{
		{ID: "s1", Goal: "one", InputRefs: []stepplan.InputRef{{Key: "in", Hash: "h1"}}, DoneCriteria: stepplan.DoneCriteria{RequiredKeys: []string{"v"}}},
		{ID: "s2", Goal: "two", DoneCriteria: stepplan.DoneCriteria{RequiredKeys: []string{"v"}}},
		{ID: "s3", Goal: "three", DoneCriteria: stepplan.DoneCriteria{RequiredKeys: []string{"v"}}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SavePlan(ctx, plan); err != nil {
		t.Fatal(err)
	}

	out1, _ := json.Marshal(map[string]any{"v": "a"})
	cp, err := stepplan.NewCompleteCheckpoint(plan.ID, plan.Steps[0], out1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveStep(ctx, cp); err != nil {
		t.Fatal(err)
	}

	var ran []string
	runner := StepRunnerFunc(func(ctx context.Context, p *stepplan.Plan, step stepplan.Step) (*StepRunResult, error) {
		ran = append(ran, step.ID)
		body, err := json.Marshal(map[string]any{"v": step.ID})
		if err != nil {
			return nil, err
		}
		return &StepRunResult{Output: body}, nil
	})

	res, err := RunStepPlan(ctx, plan, store, runner, StepPlanConfig{MaxAttemptsPerStep: 2}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.SkippedSteps) != 1 || res.SkippedSteps[0] != "s1" {
		t.Fatalf("skipped=%v", res.SkippedSteps)
	}
	if len(ran) != 2 || ran[0] != "s2" || ran[1] != "s3" {
		t.Fatalf("ran=%v", ran)
	}
	completed, err := store.ListCompleted(ctx, plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(completed) != 3 {
		t.Fatalf("completed=%v", completed)
	}
	loaded, err := store.LoadPlan(ctx, plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != stepplan.PlanStatusCompleted {
		t.Fatalf("status=%q", loaded.Status)
	}
}

func TestRunStepPlanBudgetTimeoutAbortsStepNotWholePlanSemantics(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store, err := stepplan.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	plan, err := stepplan.NewPlan("budget_plan", "test", []stepplan.Step{
		{
			ID:     "slow",
			Goal:   "slow step",
			Budget: stepplan.Budget{Timeout: 20 * time.Millisecond},
		},
		{ID: "never", Goal: "should not run"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SavePlan(ctx, plan); err != nil {
		t.Fatal(err)
	}

	var ranSecond atomic.Bool
	runner := StepRunnerFunc(func(ctx context.Context, p *stepplan.Plan, step stepplan.Step) (*StepRunResult, error) {
		if step.ID == "never" {
			ranSecond.Store(true)
			return &StepRunResult{Output: json.RawMessage(`{}`)}, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(500 * time.Millisecond):
			return &StepRunResult{Output: json.RawMessage(`{}`)}, nil
		}
	})

	res, err := RunStepPlan(ctx, plan, store, runner, StepPlanConfig{
		MaxAttemptsPerStep: 1,
		Classify: func(err error) StepErrorClass {
			return StepErrorClass{Retryable: false}
		},
	}, nil)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if res.FailedStepID != "slow" {
		t.Fatalf("failed=%q", res.FailedStepID)
	}
	if ranSecond.Load() {
		t.Fatal("second step must not run after step failure")
	}
	cp, loadErr := store.LoadStep(ctx, plan.ID, "slow")
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if cp.Status != stepplan.StepStatusFailed {
		t.Fatalf("checkpoint status=%q", cp.Status)
	}
}

func TestRunStepPlanBreakerBackoffThenSuccess(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store, err := stepplan.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	plan, err := stepplan.NewPlan("breaker_plan", "test", []stepplan.Step{
		{ID: "s1", Goal: "flaky", DoneCriteria: stepplan.DoneCriteria{RequiredKeys: []string{"ok"}}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SavePlan(ctx, plan); err != nil {
		t.Fatal(err)
	}

	var calls atomic.Int32
	var slept []time.Duration
	runner := StepRunnerFunc(func(ctx context.Context, p *stepplan.Plan, step stepplan.Step) (*StepRunResult, error) {
		n := calls.Add(1)
		if n == 1 {
			return nil, errors.New("upstream 503 circuit open")
		}
		body, err := json.Marshal(map[string]any{"ok": true})
		if err != nil {
			return nil, err
		}
		return &StepRunResult{Output: body}, nil
	})

	res, err := RunStepPlan(ctx, plan, store, runner, StepPlanConfig{
		MaxAttemptsPerStep: 3,
		Sleep: func(ctx context.Context, d time.Duration) error {
			slept = append(slept, d)
			return nil
		},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("calls=%d", calls.Load())
	}
	if len(slept) != 1 || slept[0] != DefaultBreakerBackoff {
		t.Fatalf("slept=%v", slept)
	}
	if len(res.CompletedSteps) != 1 {
		t.Fatalf("completed=%v", res.CompletedSteps)
	}
}

func TestRunStepPlanStaleFingerprintReruns(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store, err := stepplan.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	step := stepplan.Step{
		ID:        "s1",
		Goal:      "one",
		InputRefs: []stepplan.InputRef{{Key: "in", Hash: "old"}},
	}
	plan, err := stepplan.NewPlan("stale_plan", "test", []stepplan.Step{step}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SavePlan(ctx, plan); err != nil {
		t.Fatal(err)
	}
	cp, err := stepplan.NewCompleteCheckpoint(plan.ID, step, json.RawMessage(`{"v":1}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveStep(ctx, cp); err != nil {
		t.Fatal(err)
	}

	// Change input hash: checkpoint fingerprint no longer matches.
	plan.Steps[0].InputRefs[0].Hash = "new"
	var ran int
	runner := StepRunnerFunc(func(ctx context.Context, p *stepplan.Plan, step stepplan.Step) (*StepRunResult, error) {
		ran++
		return &StepRunResult{Output: json.RawMessage(`{"v":2}`)}, nil
	})
	if _, err := RunStepPlan(ctx, plan, store, runner, StepPlanConfig{MaxAttemptsPerStep: 1}, nil); err != nil {
		t.Fatal(err)
	}
	if ran != 1 {
		t.Fatalf("expected rerun, ran=%d", ran)
	}
}

func TestClassifyTransientStepError(t *testing.T) {
	t.Parallel()
	cases := []struct {
		err       error
		retryable bool
		backoff   time.Duration
	}{
		{context.Canceled, false, 0},
		{context.DeadlineExceeded, true, DefaultTransientBackoff},
		{fmt.Errorf("bifrost 502 deadline exceeded"), true, DefaultTransientBackoff},
		{fmt.Errorf("polypus 503"), true, DefaultBreakerBackoff},
		{fmt.Errorf("validation missing keys"), true, 0},
	}
	for _, tc := range cases {
		got := ClassifyTransientStepError(tc.err)
		if got.Retryable != tc.retryable || got.Backoff != tc.backoff {
			t.Fatalf("err=%v got=%+v want retryable=%v backoff=%s", tc.err, got, tc.retryable, tc.backoff)
		}
	}
}

func TestValidateOutputRequiredKeys(t *testing.T) {
	t.Parallel()
	step := stepplan.Step{
		ID:           "s1",
		Goal:         "g",
		DoneCriteria: stepplan.DoneCriteria{RequiredKeys: []string{"evidence"}},
	}
	if err := stepplan.ValidateOutput(step, json.RawMessage(`{"evidence":"ok"}`)); err != nil {
		t.Fatal(err)
	}
	if err := stepplan.ValidateOutput(step, json.RawMessage(`{"evidence":""}`)); err == nil {
		t.Fatal("expected empty evidence to fail")
	}
}
