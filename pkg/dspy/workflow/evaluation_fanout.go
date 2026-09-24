package workflow

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/behaviorengineering/strop/pkg/dspy/actor"
	"github.com/behaviorengineering/strop/pkg/evaluation"
	"github.com/behaviorengineering/strop/pkg/streaming"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
)

func sendOrCancel[T any](ctx context.Context, ch chan<- T, value T) bool {
	select {
	case ch <- value:
		return true
	case <-ctx.Done():
		return false
	}
}

// emitInferenceEvent sends an inference event when a stream channel is present.
// A nil channel is a no-op (EvaluateWorkflow callers often pass nil). Sending on a
// nil Go channel would block forever.
func emitInferenceEvent(ctx context.Context, eventChan streaming.EventChannel, event streaming.InferenceEvent) error {
	if eventChan == nil {
		return nil
	}
	select {
	case eventChan <- event:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (w *ParallelEvaluationWorkflow) runIndividualEvaluators(
	ctx context.Context,
	inputs map[string]interface{},
) ([]*evaluation.IndividualEvaluation, error) {
	// Create channel for collecting results.
	results := make(chan *evaluation.IndividualEvaluation, len(w.evaluators))
	errorCh := make(chan error, len(w.evaluators))

	// Launch goroutine for each evaluator.
	var wg sync.WaitGroup
	for roleKey, evaluator := range w.evaluators {
		wg.Add(1)
		go func(roleKey evaluation.EvaluatorKey, evaluator actor.Evaluator) {
			defer wg.Done()

			// Check context cancellation before expensive operation.
			select {
			case <-ctx.Done():
				sendOrCancel(ctx, errorCh, fmt.Errorf("evaluator %s cancelled: %w", roleKey, ctx.Err()))
				return
			default:
			}

			if evaluator.Module == nil {
				sendOrCancel(ctx, errorCh, fmt.Errorf("evaluator %s has nil module", roleKey))
				return
			}

			// Each parallel goroutine needs its own ExecutionState so cost tracking (model ID,
			// token usage) is correct per span. Otherwise they overwrite the shared state.
			workerCtx := core.WithFreshExecutionState(ctx)
			result, err := evaluator.Process(workerCtx, inputs)
			if err != nil {
				var sanitizedErr error
				if w.sanitizeError != nil {
					sanitizedErr = w.sanitizeError(err)
				} else {
					sanitizedErr = err
				}
				sendOrCancel(ctx, errorCh, fmt.Errorf("evaluator %s failed: %w", roleKey, sanitizedErr))
				return
			}

			if result == nil {
				sendOrCancel(ctx, errorCh, fmt.Errorf("evaluator %s returned nil result", roleKey))
				return
			}

			if _, hasCriterionScores := result[w.fieldNames.CriterionScores]; !hasCriterionScores {
				w.logger.WithFields(map[string]interface{}{
					"evaluator":   roleKey,
					"result_keys": getMapKeys(result),
				}).Error("Evaluator result missing criterion_scores field - checking available fields")
			}

			individualEval, err := w.parseEvaluationResult(roleKey, result)
			if err != nil {
				w.logEvaluatorPayloadSizes(ctx, roleKey, inputs, false)
				w.logger.WithFields(map[string]interface{}{
					"evaluator":   roleKey,
					"result_keys": getMapKeys(result),
					"error":       err,
				}).Error("Failed to parse evaluator result")
				sendOrCancel(ctx, errorCh, fmt.Errorf("failed to parse result from %s: %w", roleKey, err))
				return
			}

			w.logEvaluatorPayloadSizes(ctx, roleKey, inputs, true)

			if !sendOrCancel(ctx, results, individualEval) {
				// Ctx cancel after success must not look like "no evaluators returned results".
				select {
				case errorCh <- fmt.Errorf("evaluator %s cancelled after success: %w", roleKey, ctx.Err()):
				default:
				}
				return
			}
		}(roleKey, evaluator)
	}

	// Wait for all goroutines to complete, then close channels.
	wg.Wait()
	close(results)
	close(errorCh)

	// Collect results and errors.
	var individualEvals []*evaluation.IndividualEvaluation
	var errs []error

	for eval := range results {
		individualEvals = append(individualEvals, eval)
	}

	for err := range errorCh {
		errs = append(errs, err)
	}

	// This ensures we always have complete, unbiased evaluation results.
	if len(errs) > 0 {
		totalExpected := len(w.evaluators)
		successCount := len(individualEvals)
		failureCount := len(errs)

		w.logger.WithFields(map[string]interface{}{
			"error_count":    failureCount,
			"success_count":  successCount,
			"total_expected": totalExpected,
		}).Error("Evaluation failed: some evaluators failed - failing fast to ensure complete evaluation")

		// Join every evaluator failure so errors.Is and errors.As can inspect all causes.
		msg := fmt.Sprintf("evaluation failed: %d of %d evaluators failed", failureCount, totalExpected)
		return nil, fmt.Errorf("%s: %w", msg, errors.Join(errs...))
	}

	// Validate we have at least one result (should always be true if no errors).
	if len(individualEvals) == 0 {
		return nil, fmt.Errorf("evaluation failed: no evaluators returned results")
	}

	return individualEvals, nil
}

// runIndividualEvaluatorsStream runs all evaluators in parallel with streaming support.
func (w *ParallelEvaluationWorkflow) runIndividualEvaluatorsStream(
	ctx context.Context,
	inputs map[string]interface{},
	eventChan streaming.EventChannel,
) ([]*evaluation.IndividualEvaluation, error) {
	// Create channel for collecting results.
	results := make(chan *evaluation.IndividualEvaluation, len(w.evaluators))
	errorCh := make(chan error, len(w.evaluators))

	var wg sync.WaitGroup
	for roleKey, evaluator := range w.evaluators {
		wg.Add(1)
		go func(roleKey evaluation.EvaluatorKey, evaluator actor.Evaluator) {
			defer wg.Done()

			if err := emitInferenceEvent(ctx, eventChan, evaluator.StartEvent()); err != nil {
				sendOrCancel(ctx, errorCh, fmt.Errorf("evaluator %s cancelled: %w", roleKey, err))
				return
			}

			select {
			case <-ctx.Done():
				sendOrCancel(ctx, errorCh, fmt.Errorf("evaluator %s cancelled: %w", roleKey, ctx.Err()))
				return
			default:
			}

			if evaluator.Module == nil {
				sendOrCancel(ctx, errorCh, fmt.Errorf("evaluator %s has nil module", roleKey))
				return
			}

			// Each parallel goroutine needs its own ExecutionState so cost tracking (model ID,
			// token usage) is correct per span. Otherwise they overwrite the shared state.
			workerCtx := core.WithFreshExecutionState(ctx)
			handler := evaluator.StreamHandler(eventChan)
			result, err := evaluator.Process(workerCtx, inputs, core.WithStreamHandler(handler))

			_ = emitInferenceEvent(ctx, eventChan, evaluator.EndEvent(result, err))

			if err != nil {
				var sanitizedErr error
				if w.sanitizeError != nil {
					sanitizedErr = w.sanitizeError(err)
				} else {
					sanitizedErr = err
				}
				sendOrCancel(ctx, errorCh, fmt.Errorf("evaluator %s failed: %w", roleKey, sanitizedErr))
				return
			}

			if result == nil {
				sendOrCancel(ctx, errorCh, fmt.Errorf("evaluator %s returned nil result", roleKey))
				return
			}

			if _, hasCriterionScores := result[w.fieldNames.CriterionScores]; !hasCriterionScores {
				w.logger.WithFields(map[string]interface{}{
					"evaluator":   roleKey,
					"result_keys": getMapKeys(result),
				}).Error("Evaluator result missing criterion_scores field - checking available fields")
			}

			individualEval, err := w.parseEvaluationResult(roleKey, result)
			if err != nil {
				w.logEvaluatorPayloadSizes(ctx, roleKey, inputs, false)
				w.logger.WithFields(map[string]interface{}{
					"evaluator":   roleKey,
					"result_keys": getMapKeys(result),
					"error":       err,
				}).Error("Failed to parse evaluator result")
				sendOrCancel(ctx, errorCh, fmt.Errorf("failed to parse result from %s: %w", roleKey, err))
				return
			}

			w.logEvaluatorPayloadSizes(ctx, roleKey, inputs, true)

			if !sendOrCancel(ctx, results, individualEval) {
				// Ctx cancel after success must not look like "no evaluators returned results".
				select {
				case errorCh <- fmt.Errorf("evaluator %s cancelled after success: %w", roleKey, ctx.Err()):
				default:
				}
				return
			}
		}(roleKey, evaluator)
	}

	// Wait for all goroutines to complete, then close channels.
	wg.Wait()
	close(results)
	close(errorCh)

	// Collect results and errors.
	var individualEvals []*evaluation.IndividualEvaluation
	var errs []error

	for eval := range results {
		individualEvals = append(individualEvals, eval)
	}

	for err := range errorCh {
		errs = append(errs, err)
	}

	// Fail fast: If any evaluator fails, fail the entire evaluation.
	if len(errs) > 0 {
		totalExpected := len(w.evaluators)
		successCount := len(individualEvals)
		failureCount := len(errs)

		w.logger.WithFields(map[string]interface{}{
			"error_count":    failureCount,
			"success_count":  successCount,
			"total_expected": totalExpected,
		}).Error("Evaluation failed: some evaluators failed - failing fast to ensure complete evaluation")

		msg := fmt.Sprintf("evaluation failed: %d of %d evaluators failed", failureCount, totalExpected)
		return nil, fmt.Errorf("%s: %w", msg, errors.Join(errs...))
	}

	// Validate we have at least one result.
	if len(individualEvals) == 0 {
		return nil, fmt.Errorf("evaluation failed: no evaluators returned results")
	}

	return individualEvals, nil
}
