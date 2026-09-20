package orchestration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/behaviorengineering/strop/pkg/runreport"
	"github.com/behaviorengineering/strop/pkg/stepplan"
	"github.com/behaviorengineering/strop/pkg/streaming"
)

// DefaultMaxAttemptsPerStep is used when StepPlanConfig.MaxAttemptsPerStep is unset.
const DefaultMaxAttemptsPerStep = 3

// StepRunResult is the successful outcome of one StepRunner call.
type StepRunResult struct {
	Output json.RawMessage
	Usage  *stepplan.TokenUsage
}

// StepRunner executes one plan step under the caller's inference stack.
// The runner SHOULD honor step.Budget (timeout / iterations / chars).
// RunStepPlan also wraps ctx with step.Budget.Timeout when set.
type StepRunner interface {
	RunStep(ctx context.Context, plan *stepplan.Plan, step stepplan.Step) (*StepRunResult, error)
}

// StepRunnerFunc adapts a function to StepRunner.
type StepRunnerFunc func(ctx context.Context, plan *stepplan.Plan, step stepplan.Step) (*StepRunResult, error)

// RunStep implements StepRunner.
func (f StepRunnerFunc) RunStep(ctx context.Context, plan *stepplan.Plan, step stepplan.Step) (*StepRunResult, error) {
	return f(ctx, plan, step)
}

// StepErrorClass tells the driver whether to retry a failed attempt and how long to wait.
type StepErrorClass struct {
	Retryable bool
	Backoff   time.Duration
}

// StepErrorClassifier classifies runner or validation errors for retry policy.
type StepErrorClassifier func(err error) StepErrorClass

// StepPlanConfig configures RunStepPlan. Zero values use defaults.
type StepPlanConfig struct {
	// MaxAttemptsPerStep caps retries for each step (default DefaultMaxAttemptsPerStep).
	MaxAttemptsPerStep int
	// Classify maps errors to retry/backoff. Default: ClassifyTransientStepError.
	Classify StepErrorClassifier
	// Sleep waits between retries. Default: time.Sleep respecting ctx cancel.
	Sleep func(ctx context.Context, d time.Duration) error
	// SkipValidateRequiredKeys disables built-in DoneCriteria.RequiredKeys checks.
	SkipValidateRequiredKeys bool
	// Validate is an optional host validator run after the built-in check (when enabled).
	Validate func(ctx context.Context, plan *stepplan.Plan, step stepplan.Step, output json.RawMessage) error
}

// StepPlanResult summarizes a finished (or aborted) plan run.
type StepPlanResult struct {
	PlanID         string
	CompletedSteps []string
	SkippedSteps   []string
	FailedStepID   string
	AttemptsOnFail int
	Err            error
}

// RunStepPlan executes a persisted plan step-by-step with checkpoint resume.
//
// For each step: load checkpoint; skip when complete and input fingerprint still matches;
// otherwise run StepRunner under the step budget; validate; save checkpoint; continue.
// Transient failures retry within MaxAttemptsPerStep with classifier backoff (e.g. wait on 503).
// On final step failure the failed checkpoint is saved, the plan is marked failed, and an error returns.
// On full success the plan is marked completed.
func RunStepPlan(
	ctx context.Context,
	plan *stepplan.Plan,
	store stepplan.Store,
	runner StepRunner,
	cfg StepPlanConfig,
	eventChan streaming.EventChannel,
) (*StepPlanResult, error) {
	if plan == nil {
		return nil, fmt.Errorf("orchestration: plan is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("orchestration: stepplan store is nil")
	}
	if runner == nil {
		return nil, fmt.Errorf("orchestration: step runner is nil")
	}
	if err := stepplan.Validate(plan); err != nil {
		return nil, err
	}

	maxAttempts := cfg.MaxAttemptsPerStep
	if maxAttempts <= 0 {
		maxAttempts = DefaultMaxAttemptsPerStep
	}
	classify := cfg.Classify
	if classify == nil {
		classify = ClassifyTransientStepError
	}
	sleep := cfg.Sleep
	if sleep == nil {
		sleep = sleepContext
	}

	result := &StepPlanResult{PlanID: plan.ID}
	meta := runreport.Meta{
		EntityID: plan.ID,
		Job:      planKindOrDefault(plan),
		Version:  0,
	}
	reportCfg := runreport.ConfigFromContext(ctx)
	var runErr error
	ctx, finishReport := runreport.StartSession(ctx, reportCfg, meta)
	defer func() {
		finishReport(runErr)
	}()

	for _, step := range plan.Steps {
		select {
		case <-ctx.Done():
			runErr = ctx.Err()
			result.Err = runErr
			return result, runErr
		default:
		}

		key := step.EffectiveCheckpointKey()
		if shouldSkipStep(ctx, store, plan.ID, step) {
			result.SkippedSteps = append(result.SkippedSteps, key)
			sendStepPlanEvent(eventChan, fmt.Sprintf("Step plan %s skip %s (checkpoint complete)", plan.ID, step.ID))
			continue
		}

		completed, attempts, err := runOneStep(ctx, plan, step, store, runner, maxAttempts, classify, sleep, cfg, eventChan)
		if err != nil {
			result.FailedStepID = step.ID
			result.AttemptsOnFail = attempts
			result.Err = err
			runErr = err
			_ = markPlanStatus(ctx, store, plan, stepplan.PlanStatusFailed)
			return result, err
		}
		if completed {
			result.CompletedSteps = append(result.CompletedSteps, key)
		}
	}

	if err := markPlanStatus(ctx, store, plan, stepplan.PlanStatusCompleted); err != nil {
		runErr = err
		result.Err = err
		return result, err
	}
	sendStepPlanEvent(eventChan, fmt.Sprintf("Step plan %s completed", plan.ID))
	return result, nil
}

func runOneStep(
	ctx context.Context,
	plan *stepplan.Plan,
	step stepplan.Step,
	store stepplan.Store,
	runner StepRunner,
	maxAttempts int,
	classify StepErrorClassifier,
	sleep func(context.Context, time.Duration) error,
	cfg StepPlanConfig,
	eventChan streaming.EventChannel,
) (completed bool, attempts int, err error) {
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		attempts = attempt
		select {
		case <-ctx.Done():
			return false, attempts, ctx.Err()
		default:
		}

		sendStepPlanEvent(eventChan, fmt.Sprintf(
			"Step plan %s run %s (attempt %d/%d)",
			plan.ID,
			step.ID,
			attempt,
			maxAttempts,
		))

		stepCtx := ctx
		cancel := func() {}
		if step.Budget.Timeout > 0 {
			stepCtx, cancel = context.WithTimeout(ctx, step.Budget.Timeout)
		}

		out, runErr := runner.RunStep(stepCtx, plan, step)
		cancel()
		if runErr != nil {
			lastErr = fmt.Errorf("orchestration: step %q: %w", step.ID, runErr)
			if !retryAfterError(ctx, lastErr, attempt, maxAttempts, classify, sleep, eventChan, plan.ID, step.ID) {
				return false, attempts, persistFailed(ctx, store, plan.ID, step, lastErr)
			}
			continue
		}
		if out == nil {
			lastErr = fmt.Errorf("orchestration: step %q returned nil result", step.ID)
			if !retryAfterError(ctx, lastErr, attempt, maxAttempts, classify, sleep, eventChan, plan.ID, step.ID) {
				return false, attempts, persistFailed(ctx, store, plan.ID, step, lastErr)
			}
			continue
		}

		if err := validateStepOutput(ctx, cfg, plan, step, out.Output); err != nil {
			lastErr = err
			if !retryAfterError(ctx, lastErr, attempt, maxAttempts, classify, sleep, eventChan, plan.ID, step.ID) {
				return false, attempts, persistFailed(ctx, store, plan.ID, step, lastErr)
			}
			continue
		}

		cp, err := stepplan.NewCompleteCheckpoint(plan.ID, step, out.Output, out.Usage)
		if err != nil {
			return false, attempts, err
		}
		if err := store.SaveStep(ctx, cp); err != nil {
			return false, attempts, fmt.Errorf("orchestration: save checkpoint %q: %w", step.ID, err)
		}
		if c := runreport.CollectorFromContext(ctx); c != nil {
			c.RecordPhase(step.ID, attempt, true, 0, "")
		}
		sendStepPlanEvent(eventChan, fmt.Sprintf("Step plan %s completed %s", plan.ID, step.ID))
		return true, attempts, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("orchestration: step %q exhausted attempts", step.ID)
	}
	return false, attempts, persistFailed(ctx, store, plan.ID, step, lastErr)
}

func validateStepOutput(
	ctx context.Context,
	cfg StepPlanConfig,
	plan *stepplan.Plan,
	step stepplan.Step,
	output json.RawMessage,
) error {
	if !cfg.SkipValidateRequiredKeys {
		if err := stepplan.ValidateOutput(step, output); err != nil {
			return err
		}
	}
	if cfg.Validate != nil {
		if err := cfg.Validate(ctx, plan, step, output); err != nil {
			return fmt.Errorf("orchestration: step %q validation: %w", step.ID, err)
		}
	}
	return nil
}

func retryAfterError(
	ctx context.Context,
	err error,
	attempt, maxAttempts int,
	classify StepErrorClassifier,
	sleep func(context.Context, time.Duration) error,
	eventChan streaming.EventChannel,
	planID, stepID string,
) bool {
	if attempt >= maxAttempts {
		return false
	}
	class := classify(err)
	if !class.Retryable {
		return false
	}
	if c := runreport.CollectorFromContext(ctx); c != nil {
		c.RecordPhase(stepID, attempt, false, 0, truncateFeedback(err.Error(), 200))
	}
	msg := fmt.Sprintf("Step plan %s retry %s after error", planID, stepID)
	if class.Backoff > 0 {
		msg = fmt.Sprintf("%s (backoff %s)", msg, class.Backoff)
		sendStepPlanEvent(eventChan, msg)
		if sleepErr := sleep(ctx, class.Backoff); sleepErr != nil {
			return false
		}
		return true
	}
	sendStepPlanEvent(eventChan, msg)
	return true
}

func persistFailed(ctx context.Context, store stepplan.Store, planID string, step stepplan.Step, err error) error {
	cp, cpErr := stepplan.NewFailedCheckpoint(planID, step, err.Error())
	if cpErr != nil {
		return errors.Join(err, cpErr)
	}
	if saveErr := store.SaveStep(ctx, cp); saveErr != nil {
		return errors.Join(err, fmt.Errorf("orchestration: save failed checkpoint: %w", saveErr))
	}
	return err
}

func shouldSkipStep(ctx context.Context, store stepplan.Store, planID string, step stepplan.Step) bool {
	cp, err := store.LoadStep(ctx, planID, step.EffectiveCheckpointKey())
	if err != nil {
		return false
	}
	if cp.Status != stepplan.StepStatusComplete {
		return false
	}
	want := stepplan.FingerprintInputs(step.InputRefs)
	if want == "" {
		return true
	}
	return cp.InputFingerprint == "" || cp.InputFingerprint == want
}

func markPlanStatus(ctx context.Context, store stepplan.Store, plan *stepplan.Plan, status string) error {
	plan.Status = status
	plan.UpdatedAt = time.Now().UTC()
	if err := store.SavePlan(ctx, plan); err != nil {
		return fmt.Errorf("orchestration: save plan status %q: %w", status, err)
	}
	return nil
}

func planKindOrDefault(plan *stepplan.Plan) string {
	kind := strings.TrimSpace(plan.Kind)
	if kind == "" {
		return "step_plan"
	}
	return kind
}

func sendStepPlanEvent(eventChan streaming.EventChannel, content string) {
	streaming.SendInfo(eventChan, content)
}

func sleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
