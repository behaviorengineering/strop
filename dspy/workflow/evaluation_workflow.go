package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	stropdspy "github.com/behaviorengineering/strop/dspy"
	"github.com/behaviorengineering/strop/dspy/actor"
	"github.com/behaviorengineering/strop/dspy/rawresponse"
	"github.com/behaviorengineering/strop/evaluation"
	"github.com/behaviorengineering/strop/evaluation/criteria"
	stroplog "github.com/behaviorengineering/strop/log"
	"github.com/behaviorengineering/strop/runreport"
	"github.com/behaviorengineering/strop/streaming"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// criterionRegistry is the shared DefaultRegistry (packs register product rubrics at container startup).
var criterionRegistry = criteria.DefaultRegistry()

// FieldNames contains field name constants for the workflow.
// This allows the workflow to be generic while using field names from the framework.
type FieldNames struct {
	Score                string
	CriterionScores      string
	Feedback             string
	Rationale            string
	IterationVersion     string
	IndividualFeedbacks  string
	AgentScores          string
	WeightedScore        string
	ConsolidatedFeedback string
}

// WorkflowConfig contains all configuration for creating a ParallelEvaluationWorkflow.
// Evaluator keys are evaluation.EvaluatorKey; RoleInfo supplies labels and consolidator identity.
type WorkflowConfig struct {
	Evaluators          map[evaluation.EvaluatorKey]actor.Evaluator
	RoleInfo            evaluation.RoleInfo
	Consolidator        actor.Consolidator
	Logger              stroplog.Logger
	FieldNames          FieldNames
	SanitizeError       func(error) error
	ChainSpanCtxKeyType interface{}
	RoleToCriterionIDs  map[evaluation.EvaluatorKey][]criteria.CriterionID
	TraceServiceName    string // OpenTelemetry tracer name; defaults to "github.com/behaviorengineering/strop"
}

// ParallelEvaluationWorkflow runs multiple evaluators in parallel and aggregates results.
type ParallelEvaluationWorkflow struct {
	evaluators          map[evaluation.EvaluatorKey]actor.Evaluator
	roleInfo            evaluation.RoleInfo
	consolidator        actor.Consolidator
	logger              stroplog.Logger
	fieldNames          FieldNames
	sanitizeError       func(error) error
	chainSpanCtxKeyType interface{}
	roleToCriterionIDs  map[evaluation.EvaluatorKey][]criteria.CriterionID
	traceServiceName    string
}

// NewParallelEvaluationWorkflow creates a new parallel evaluation workflow.
// RoleInfo consolidator key and labels must be distinct from every evaluator in Evaluators.
func NewParallelEvaluationWorkflow(config WorkflowConfig) (*ParallelEvaluationWorkflow, error) {
	evaluatorKeys := make([]evaluation.EvaluatorKey, 0, len(config.Evaluators))
	for key := range config.Evaluators {
		evaluatorKeys = append(evaluatorKeys, key)
	}
	if err := evaluation.ValidateRoleInfo(config.RoleInfo, evaluatorKeys); err != nil {
		return nil, fmt.Errorf("evaluation workflow roles: %w", err)
	}
	traceServiceName := config.TraceServiceName
	if traceServiceName == "" {
		traceServiceName = "github.com/behaviorengineering/strop"
	}
	return &ParallelEvaluationWorkflow{
		evaluators:          config.Evaluators,
		roleInfo:            config.RoleInfo,
		consolidator:        config.Consolidator,
		logger:              config.Logger,
		fieldNames:          config.FieldNames,
		sanitizeError:       config.SanitizeError,
		chainSpanCtxKeyType: config.ChainSpanCtxKeyType,
		roleToCriterionIDs:  config.RoleToCriterionIDs,
		traceServiceName:    traceServiceName,
	}, nil
}

// getMapKeys returns all keys from a map for debugging purposes.
func getMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// parseCriterionScores parses criterion scores from a map[string]interface{}.
// Returns a map of criterion ID to score.
func parseCriterionScores(value interface{}, expectedCriterionIDs map[string]struct{}) (map[string]float64, error) {
	if value == nil {
		return nil, fmt.Errorf("criterion_scores is nil")
	}

	scoresMap, err := stropdspy.CoerceCriterionScoresMap(value)
	if err != nil {
		return nil, err
	}

	result := make(map[string]float64, len(scoresMap))
	for criterionID, scoreValue := range scoresMap {
		if len(expectedCriterionIDs) > 0 {
			if _, expected := expectedCriterionIDs[criterionID]; !expected {
				// Ignore unexpected keys so malformed extra fields do not fail the whole parse.
				continue
			}
		}

		var score float64
		switch v := scoreValue.(type) {
		case float64:
			score = v
		case float32:
			score = float64(v)
		case int64:
			score = float64(v)
		case int32:
			score = float64(v)
		case int:
			score = float64(v)
		case string:
			if _, err := fmt.Sscanf(v, "%f", &score); err != nil {
				return nil, fmt.Errorf("invalid score format for criterion %s: %q", criterionID, v)
			}
		default:
			return nil, fmt.Errorf("score for criterion %s has invalid type: %T (expected float64, int64, int, or string)", criterionID, scoreValue)
		}
		result[criterionID] = score
	}

	return result, nil
}

// normalizeScoreFromCriteria calculates a normalized total score (0-10) from individual criterion scores.
// It uses the criterion registry to determine the maximum possible score for each criterion.
func normalizeScoreFromCriteria(criterionScores map[string]float64, criterionIDs []criteria.CriterionID) (float64, error) {
	if len(criterionScores) == 0 {
		return 0.0, fmt.Errorf("criterion_scores is empty")
	}

	// Calculate actual score and max possible score using shared registry.
	var actualScore float64
	var maxPossibleScore float64

	for _, criterionID := range criterionIDs {
		criterionIDStr := string(criterionID)
		score, hasScore := criterionScores[criterionIDStr]
		if !hasScore {
			// If a criterion is configured, it MUST have a score - this is a system error requiring retry.
			return 0.0, fmt.Errorf("criterion %s is configured but missing from evaluator output - system error requiring retry", criterionID)
		}

		// Get max points for this criterion.
		criterion, err := criterionRegistry.Get(criterionID)
		if err != nil {
			return 0.0, fmt.Errorf("failed to get criterion %s: %w", criterionID, err)
		}

		actualScore += score
		maxPossibleScore += criterion.MaxPoints
	}

	if maxPossibleScore == 0 {
		return 0.0, fmt.Errorf("max possible score is 0")
	}

	// Normalize to 0-10 range: (actual / max) * 10.
	normalizedScore := (actualScore / maxPossibleScore) * 10.0

	// Round to 2 decimal places to match database DECIMAL(4,2) precision.
	return math.Round(normalizedScore*100) / 100, nil
}

// sendOrCancel attempts to send a value to a channel, respecting context cancellation.
// Returns true if the value was sent successfully, false if context was cancelled.
func sendOrCancel[T any](ctx context.Context, ch chan<- T, value T) bool {
	select {
	case ch <- value:
		return true
	case <-ctx.Done():
		return false
	}
}

// Evaluate runs all evaluators in parallel, then consolidates their feedback.
func (w *ParallelEvaluationWorkflow) Evaluate(
	ctx context.Context,
	inputs map[string]interface{}, // Contains generator_input and generator_output.
) (*evaluation.AggregatedEvaluation, error) {
	startTime := time.Now()

	// This ensures errors from child spans (evaluators) propagate to this parent span.
	tracer := otel.Tracer(w.traceServiceName)
	contentVersion := extractContentVersionFromInputs(inputs, w.fieldNames.IterationVersion)
	spanName := fmt.Sprintf("evaluation.workflow.v%d", contentVersion)
	if contentVersion == 0 {
		spanName = "evaluation.workflow"
	}

	// Use injected chainSpanCtxKeyType to check for chain context.
	workflowParentCtx := ctx
	if w.chainSpanCtxKeyType != nil {
		if chainCtx, ok := ctx.Value(w.chainSpanCtxKeyType).(context.Context); ok {
			workflowParentCtx = chainCtx
		}
	}

	ctx, span := tracer.Start(workflowParentCtx, spanName, trace.WithAttributes(
		attribute.String("workflow.type", "parallel_evaluation"),
		attribute.Int("evaluator.count", len(w.evaluators)),
		// This represents the evaluation process/workflow, while child Predict modules are LLM spans.
		attribute.String("openinference.span.kind", "EVALUATOR"),
	))
	defer span.End()

	// Validate modules are configured.
	if len(w.evaluators) == 0 {
		err := fmt.Errorf("no evaluation modules configured")
		span.SetAttributes(
			attribute.Int64("evaluation.duration_ms", time.Since(startTime).Milliseconds()),
		)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	// Step 1: Run individual evaluators in parallel.
	individualEvals, err := w.runIndividualEvaluators(ctx, inputs)
	if err != nil {
		// Record error in parent span - this ensures errors propagate up.
		span.SetAttributes(
			attribute.Int("evaluation.agent_count", 0),
			attribute.Int64("evaluation.duration_ms", time.Since(startTime).Milliseconds()),
		)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	// Step 2: Calculate weighted score from individual evaluators.
	weightedScore, agentScores, err := w.calculateWeightedScore(individualEvals)
	if err != nil {
		span.SetAttributes(
			attribute.Int("evaluation.agent_count", len(individualEvals)),
			attribute.Int64("evaluation.duration_ms", time.Since(startTime).Milliseconds()),
		)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	// Step 2b: Calculate weighted criterion scores from individual evaluators.
	criterionScores, err := w.calculateWeightedCriterionScores(individualEvals)
	if err != nil {
		span.SetAttributes(
			attribute.Int("evaluation.agent_count", len(individualEvals)),
			attribute.Int64("evaluation.duration_ms", time.Since(startTime).Milliseconds()),
		)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	// Extract version from inputs to pass to consolidator.
	consolidatedFeedback, err := w.consolidateFeedbacks(ctx, individualEvals, agentScores, weightedScore, contentVersion)
	if err != nil {
		span.SetAttributes(
			attribute.Float64("evaluation.weighted_score", weightedScore),
			attribute.Int("evaluation.agent_count", len(agentScores)),
			attribute.Int64("evaluation.duration_ms", time.Since(startTime).Milliseconds()),
		)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("%s: %w", "Failed to consolidate feedbacks", err)
	}

	// Build agent feedback map.
	agentFeedback := make(map[string]string)
	agentRationale := make(map[string]string)
	for _, eval := range individualEvals {
		agentFeedback[eval.AgentName] = eval.Feedback
		// Keep empty check as defensive programming in case of edge cases.
		if eval.Rationale != "" {
			agentRationale[eval.AgentName] = eval.Rationale
		}
	}

	// Mark span as successful and set all evaluation attributes.
	span.SetStatus(codes.Ok, "")
	span.SetAttributes(
		attribute.Float64("evaluation.weighted_score", weightedScore),
		attribute.Int("evaluation.agent_count", len(agentScores)),
		attribute.Int64("evaluation.duration_ms", time.Since(startTime).Milliseconds()),
	)

	return &evaluation.AggregatedEvaluation{
		WeightedScore:        weightedScore,
		CriterionScores:      criterionScores,
		ConsolidatedFeedback: consolidatedFeedback,
		AgentScores:          agentScores,
		AgentFeedback:        agentFeedback,
		AgentRationale:       agentRationale,
		EvaluationTime:       time.Since(startTime),
	}, nil
}

// EvaluateStream runs all evaluators in parallel with streaming support, then consolidates their feedback.
func (w *ParallelEvaluationWorkflow) EvaluateStream(
	ctx context.Context,
	inputs map[string]interface{}, // Contains generator_input and generator_output.
	eventChan streaming.EventChannel,
) (*evaluation.AggregatedEvaluation, error) {
	startTime := time.Now()

	// Create OpenTelemetry span for the evaluation workflow.
	tracer := otel.Tracer(w.traceServiceName)
	contentVersion := extractContentVersionFromInputs(inputs, w.fieldNames.IterationVersion)
	spanName := fmt.Sprintf("evaluation.workflow.v%d", contentVersion)
	if contentVersion == 0 {
		spanName = "evaluation.workflow"
	}

	// Check if chain span context exists.
	workflowParentCtx := ctx
	if w.chainSpanCtxKeyType != nil {
		if chainCtx, ok := ctx.Value(w.chainSpanCtxKeyType).(context.Context); ok {
			workflowParentCtx = chainCtx
		}
	}

	ctx, span := tracer.Start(workflowParentCtx, spanName, trace.WithAttributes(
		attribute.String("workflow.type", "parallel_evaluation"),
		attribute.Int("evaluator.count", len(w.evaluators)),
		attribute.String("openinference.span.kind", "EVALUATOR"),
	))
	defer span.End()

	if eventChan != nil {
		ctx = streaming.ContextWithEventChannel(ctx, eventChan)
	}

	// Validate modules are configured.
	if len(w.evaluators) == 0 {
		err := fmt.Errorf("%s", "no evaluation modules configured")
		span.SetAttributes(
			attribute.Int64("evaluation.duration_ms", time.Since(startTime).Milliseconds()),
		)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	// Step 1: Run individual evaluators in parallel with streaming.
	individualEvals, err := w.runIndividualEvaluatorsStream(ctx, inputs, eventChan)
	if err != nil {
		span.SetAttributes(
			attribute.Int("evaluation.agent_count", 0),
			attribute.Int64("evaluation.duration_ms", time.Since(startTime).Milliseconds()),
		)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	// Step 2: Calculate weighted score from individual evaluators.
	weightedScore, agentScores, err := w.calculateWeightedScore(individualEvals)
	if err != nil {
		span.SetAttributes(
			attribute.Int("evaluation.agent_count", len(individualEvals)),
			attribute.Int64("evaluation.duration_ms", time.Since(startTime).Milliseconds()),
		)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	// Step 2b: Calculate weighted criterion scores from individual evaluators.
	criterionScores, err := w.calculateWeightedCriterionScores(individualEvals)
	if err != nil {
		span.SetAttributes(
			attribute.Int("evaluation.agent_count", len(individualEvals)),
			attribute.Int64("evaluation.duration_ms", time.Since(startTime).Milliseconds()),
		)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	// Step 3: Consolidate feedbacks using consolidator with streaming.
	consolidatedFeedback, err := w.consolidateFeedbacksStream(ctx, individualEvals, agentScores, weightedScore, contentVersion, eventChan)
	if err != nil {
		span.SetAttributes(
			attribute.Float64("evaluation.weighted_score", weightedScore),
			attribute.Int("evaluation.agent_count", len(agentScores)),
			attribute.Int64("evaluation.duration_ms", time.Since(startTime).Milliseconds()),
		)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("%s: %w", "Failed to consolidate feedbacks", err)
	}

	// Build agent feedback map.
	agentFeedback := make(map[string]string)
	agentRationale := make(map[string]string)
	for _, eval := range individualEvals {
		agentFeedback[eval.AgentName] = eval.Feedback
		if eval.Rationale != "" {
			agentRationale[eval.AgentName] = eval.Rationale
		}
	}

	// Mark span as successful.
	span.SetStatus(codes.Ok, "")
	span.SetAttributes(
		attribute.Float64("evaluation.weighted_score", weightedScore),
		attribute.Int("evaluation.agent_count", len(agentScores)),
		attribute.Int64("evaluation.duration_ms", time.Since(startTime).Milliseconds()),
	)

	return &evaluation.AggregatedEvaluation{
		WeightedScore:        weightedScore,
		CriterionScores:      criterionScores,
		ConsolidatedFeedback: consolidatedFeedback,
		AgentScores:          agentScores,
		AgentFeedback:        agentFeedback,
		AgentRationale:       agentRationale,
		EvaluationTime:       time.Since(startTime),
	}, nil
}

// runIndividualEvaluators runs all evaluators in parallel.
func (w *ParallelEvaluationWorkflow) runIndividualEvaluators(
	ctx context.Context,
	inputs map[string]interface{},
) ([]*evaluation.IndividualEvaluation, error) {
	// Create channel for collecting results.
	results := make(chan *evaluation.IndividualEvaluation, len(w.evaluators))
	errors := make(chan error, len(w.evaluators))

	// Launch goroutine for each evaluator.
	var wg sync.WaitGroup
	for roleKey, evaluator := range w.evaluators {
		wg.Add(1)
		go func(roleKey evaluation.EvaluatorKey, evaluator actor.Evaluator) {
			defer wg.Done()

			// Check context cancellation before expensive operation.
			select {
			case <-ctx.Done():
				sendOrCancel(ctx, errors, fmt.Errorf("evaluator %s cancelled: %w", roleKey, ctx.Err()))
				return
			default:
			}

			if evaluator.Module == nil {
				sendOrCancel(ctx, errors, fmt.Errorf("evaluator %s has nil module", roleKey))
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
				sendOrCancel(ctx, errors, fmt.Errorf("evaluator %s failed: %w", roleKey, sanitizedErr))
				return
			}

			if result == nil {
				sendOrCancel(ctx, errors, fmt.Errorf("evaluator %s returned nil result", roleKey))
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
				sendOrCancel(ctx, errors, fmt.Errorf("failed to parse result from %s: %w", roleKey, err))
				return
			}

			w.logEvaluatorPayloadSizes(ctx, roleKey, inputs, true)

			if !sendOrCancel(ctx, results, individualEval) {
				// Ctx cancel after success must not look like "no evaluators returned results".
				select {
				case errors <- fmt.Errorf("evaluator %s cancelled after success: %w", roleKey, ctx.Err()):
				default:
				}
				return
			}
		}(roleKey, evaluator)
	}

	// Wait for all goroutines to complete, then close channels.
	wg.Wait()
	close(results)
	close(errors)

	// Collect results and errors.
	var individualEvals []*evaluation.IndividualEvaluation
	var errs []error

	for eval := range results {
		individualEvals = append(individualEvals, eval)
	}

	for err := range errors {
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

		// Use %w for the first error to preserve unwrapping, %v for others.
		msg := fmt.Sprintf("evaluation failed: %d of %d evaluators failed", failureCount, totalExpected)
		if failureCount == 1 {
			return nil, fmt.Errorf("%s: %w", msg, errs[0])
		}
		// For multiple errors, wrap the first and mention the count.
		return nil, fmt.Errorf("%s: %w (and %d more)", msg, errs[0], failureCount-1)
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
	errors := make(chan error, len(w.evaluators))

	var wg sync.WaitGroup
	for roleKey, evaluator := range w.evaluators {
		wg.Add(1)
		go func(roleKey evaluation.EvaluatorKey, evaluator actor.Evaluator) {
			defer wg.Done()

			select {
			case eventChan <- evaluator.StartEvent():
			case <-ctx.Done():
				sendOrCancel(ctx, errors, fmt.Errorf("evaluator %s cancelled: %w", roleKey, ctx.Err()))
				return
			}

			select {
			case <-ctx.Done():
				sendOrCancel(ctx, errors, fmt.Errorf("evaluator %s cancelled: %w", roleKey, ctx.Err()))
				return
			default:
			}

			if evaluator.Module == nil {
				sendOrCancel(ctx, errors, fmt.Errorf("evaluator %s has nil module", roleKey))
				return
			}

			// Each parallel goroutine needs its own ExecutionState so cost tracking (model ID,
			// token usage) is correct per span. Otherwise they overwrite the shared state.
			workerCtx := core.WithFreshExecutionState(ctx)
			handler := evaluator.StreamHandler(eventChan)
			result, err := evaluator.Process(workerCtx, inputs, core.WithStreamHandler(handler))

			select {
			case eventChan <- evaluator.EndEvent(result, err):
			case <-ctx.Done():
			}

			if err != nil {
				var sanitizedErr error
				if w.sanitizeError != nil {
					sanitizedErr = w.sanitizeError(err)
				} else {
					sanitizedErr = err
				}
				sendOrCancel(ctx, errors, fmt.Errorf("evaluator %s failed: %w", roleKey, sanitizedErr))
				return
			}

			if result == nil {
				sendOrCancel(ctx, errors, fmt.Errorf("evaluator %s returned nil result", roleKey))
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
				sendOrCancel(ctx, errors, fmt.Errorf("failed to parse result from %s: %w", roleKey, err))
				return
			}

			w.logEvaluatorPayloadSizes(ctx, roleKey, inputs, true)

			if !sendOrCancel(ctx, results, individualEval) {
				// Ctx cancel after success must not look like "no evaluators returned results".
				select {
				case errors <- fmt.Errorf("evaluator %s cancelled after success: %w", roleKey, ctx.Err()):
				default:
				}
				return
			}
		}(roleKey, evaluator)
	}

	// Wait for all goroutines to complete, then close channels.
	wg.Wait()
	close(results)
	close(errors)

	// Collect results and errors.
	var individualEvals []*evaluation.IndividualEvaluation
	var errs []error

	for eval := range results {
		individualEvals = append(individualEvals, eval)
	}

	for err := range errors {
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
		if failureCount == 1 {
			return nil, fmt.Errorf("%s: %w", msg, errs[0])
		}
		return nil, fmt.Errorf("%s: %w (and %d more)", msg, errs[0], failureCount-1)
	}

	// Validate we have at least one result.
	if len(individualEvals) == 0 {
		return nil, fmt.Errorf("evaluation failed: no evaluators returned results")
	}

	return individualEvals, nil
}

// parseEvaluationResult parses the module result into IndividualEvaluation.
func (w *ParallelEvaluationWorkflow) parseEvaluationResult(
	roleKey evaluation.EvaluatorKey,
	result map[string]interface{},
) (*evaluation.IndividualEvaluation, error) {
	if w.roleInfo != nil && !w.roleInfo.HasEvaluator(roleKey) {
		return nil, fmt.Errorf("unknown agent role: %s", roleKey)
	}

	// Extract criterion_scores - must be present at top level.
	criterionScoresValue, exists := result[w.fieldNames.CriterionScores]
	if !exists || criterionScoresValue == nil {
		return nil, fmt.Errorf("criterion_scores field is missing or nil in evaluator result (evaluator: %s)", roleKey)
	}

	criterionIDs, ok := w.roleToCriterionIDs[roleKey]
	if !ok {
		return nil, fmt.Errorf("no criterion IDs configured for role %s", roleKey)
	}
	expectedCriterionIDs := make(map[string]struct{}, len(criterionIDs))
	for _, criterionID := range criterionIDs {
		expectedCriterionIDs[string(criterionID)] = struct{}{}
	}

	criterionScores, err := parseCriterionScores(criterionScoresValue, expectedCriterionIDs)
	if err != nil {
		// Log raw LLM response for debugging when parsing fails (key spelling varies by path).
		if rawResponseStr, rawKey := rawresponse.TextFrom(result); rawKey != "" {
			// Truncate for logging if too long (increased limit for debugging long XML documents).
			if len(rawResponseStr) > 20000 {
				rawResponseStr = rawResponseStr[:20000] + "..."
			}
			w.logger.WithFields(map[string]interface{}{
				"evaluator":              roleKey,
				"criterion_scores":       criterionScoresValue,
				"raw_response_key":       rawKey,
				"__raw_response_preview": rawResponseStr,
			}).Error("Failed to parse criterion_scores - logging raw response for debugging")
		}
		return nil, fmt.Errorf("failed to parse criterion_scores for evaluator %s: %w", roleKey, err)
	}

	// Calculate normalized total score from criterion scores.
	normalizedScore, err := normalizeScoreFromCriteria(criterionScores, criterionIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to normalize score for evaluator %s: %w", roleKey, err)
	}

	// Extract feedback - must be present at top level.
	feedback, ok := result[w.fieldNames.Feedback].(string)
	if !ok || feedback == "" {
		return nil, fmt.Errorf("feedback field is missing or empty in evaluator result (evaluator: %s)", roleKey)
	}

	rationale, err := stropdspy.ExtractRequiredReasoningField(result)
	if err != nil {
		return nil, fmt.Errorf("directives_ack field is missing or empty in evaluator result (evaluator: %s): %w", roleKey, err)
	}

	agentName := roleKey.String()
	if w.roleInfo != nil {
		agentName = w.roleInfo.EvaluatorName(roleKey)
	}
	return &evaluation.IndividualEvaluation{
		AgentID:         roleKey.String(),
		AgentName:       agentName,
		Score:           normalizedScore,
		CriterionScores: criterionScores,
		Feedback:        feedback,
		Rationale:       rationale,
	}, nil
}

// calculateWeightedScore calculates weighted score from individual evaluations.
func (w *ParallelEvaluationWorkflow) calculateWeightedScore(
	individualEvals []*evaluation.IndividualEvaluation,
) (float64, map[string]float64, error) {
	var weightedSum float64
	var totalWeight float64
	agentScores := make(map[string]float64)

	for _, eval := range individualEvals {
		weight := 1.0
		if w.roleInfo != nil {
			weight = w.roleInfo.EvaluatorWeight(evaluation.EvaluatorKey(eval.AgentID))
		}
		weightedSum += eval.Score * weight
		totalWeight += weight
		agentScores[eval.AgentName] = eval.Score
	}

	// Validate we have weights before division.
	if totalWeight == 0 {
		return 0, nil, fmt.Errorf("no valid evaluations with weights - cannot calculate weighted score")
	}

	// Calculate weighted average.
	weightedScoreRaw := weightedSum / totalWeight

	// This ensures the stored score matches the feedback string exactly.
	weightedScore := math.Round(weightedScoreRaw*100) / 100

	return weightedScore, agentScores, nil
}

// Validates that ALL configured criteria have scores from ALL evaluators - missing scores are system errors requiring retry.
func (w *ParallelEvaluationWorkflow) calculateWeightedCriterionScores(
	individualEvals []*evaluation.IndividualEvaluation,
) (map[string]float64, error) {
	// Collect all unique criterion IDs across all evaluators.
	criterionScoresMap := make(map[string]map[string]float64) // criterionID -> agentName -> score.
	agentWeights := make(map[string]float64)                  // agentName -> weight.

	for _, eval := range individualEvals {
		weight := 1.0
		if w.roleInfo != nil {
			weight = w.roleInfo.EvaluatorWeight(evaluation.EvaluatorKey(eval.AgentID))
		}
		agentWeights[eval.AgentName] = weight

		expectedCriterionIDs, ok := w.roleToCriterionIDs[evaluation.EvaluatorKey(eval.AgentID)]
		if !ok {
			return nil, fmt.Errorf("no criterion IDs configured for role %s", eval.AgentID)
		}

		// Validate that evaluator has scores for ALL expected criteria.
		for _, expectedCriterionID := range expectedCriterionIDs {
			expectedCriterionIDStr := string(expectedCriterionID)
			score, hasScore := eval.CriterionScores[expectedCriterionIDStr]
			if !hasScore {
				return nil, fmt.Errorf("evaluator %s (role %s) is missing score for configured criterion %s - system error requiring retry", eval.AgentName, eval.AgentID, expectedCriterionID)
			}
			// Validate score is not negative (should be 0 or positive).
			if score < 0 {
				return nil, fmt.Errorf("evaluator %s (role %s) has negative score for criterion %s: %f - invalid score", eval.AgentName, eval.AgentID, expectedCriterionID, score)
			}
		}

		// Collect criterion scores for this evaluator.
		for criterionID, score := range eval.CriterionScores {
			if criterionScoresMap[criterionID] == nil {
				criterionScoresMap[criterionID] = make(map[string]float64)
			}
			criterionScoresMap[criterionID][eval.AgentName] = score
		}
	}

	// Validate we have weights.
	if len(agentWeights) == 0 {
		return nil, fmt.Errorf("no valid evaluations with weights - cannot calculate weighted criterion scores")
	}

	allExpectedCriterionIDs := make(map[string]bool)
	for _, eval := range individualEvals {
		expectedCriterionIDs, ok := w.roleToCriterionIDs[evaluation.EvaluatorKey(eval.AgentID)]
		if !ok {
			continue
		}
		for _, criterionID := range expectedCriterionIDs {
			allExpectedCriterionIDs[string(criterionID)] = true
		}
	}

	// Second pass: calculate weighted average for each expected criterion.
	result := make(map[string]float64)
	for expectedCriterionIDStr := range allExpectedCriterionIDs {
		agentScores, hasScores := criterionScoresMap[expectedCriterionIDStr]
		if !hasScores {
			return nil, fmt.Errorf("configured criterion %s has no scores from any evaluator - system error requiring retry", expectedCriterionIDStr)
		}

		var weightedSum float64
		var totalWeight float64

		for agentName, score := range agentScores {
			weight, ok := agentWeights[agentName]
			if !ok {
				return nil, fmt.Errorf("weight not found for agent %s - cannot calculate weighted average for criterion %s", agentName, expectedCriterionIDStr)
			}
			weightedSum += score * weight
			totalWeight += weight
		}

		if totalWeight == 0 {
			return nil, fmt.Errorf("total weight is 0 for criterion %s - cannot calculate weighted average", expectedCriterionIDStr)
		}

		// Calculate weighted average and round to 2 decimal places.
		weightedAvg := math.Round((weightedSum/totalWeight)*100) / 100
		result[expectedCriterionIDStr] = weightedAvg
	}

	return result, nil
}

// All modules now use a consistent top-level "iterationVersion" field.
func extractContentVersionFromInputs(inputs map[string]interface{}, fieldIterationVersion string) int {
	if inputs == nil {
		return 0
	}

	// Helper function to extract version from a value (handles multiple types).
	extractVersionFromValue := func(version interface{}) int {
		if version == nil {
			return 0
		}
		// Try int first.
		if v, ok := version.(int); ok {
			return v
		}
		// Handle float64 case (JSON unmarshaling might convert int to float64).
		if v, ok := version.(float64); ok {
			return int(v)
		}
		// Handle string case (in case it's stored as string).
		if vStr, ok := version.(string); ok {
			var v int
			if _, err := fmt.Sscanf(vStr, "%d", &v); err == nil {
				return v
			}
		}
		return 0
	}

	// Check for consistent iterationVersion field at top level.
	if iterationVersion, ok := inputs[fieldIterationVersion]; ok {
		return extractVersionFromValue(iterationVersion)
	}

	return 0
}

// buildConsolidatorInputs builds the input map for the consolidator module.
func (w *ParallelEvaluationWorkflow) buildConsolidatorInputs(
	individualEvals []*evaluation.IndividualEvaluation,
	agentScores map[string]float64,
	weightedScore float64,
	contentVersion int,
) map[string]interface{} {
	var feedbacksBuilder strings.Builder
	for _, eval := range individualEvals {
		feedbacksBuilder.WriteString(fmt.Sprintf("=== %s (%.1f/10) ===\n", eval.AgentName, eval.Score))
		feedbacksBuilder.WriteString(eval.Feedback)
		feedbacksBuilder.WriteString("\n\n")
	}

	var scoresBuilder strings.Builder
	for agent, score := range agentScores {
		scoresBuilder.WriteString(fmt.Sprintf("- %s: %.1f/10\n", agent, score))
	}

	consolidatorInputs := map[string]interface{}{
		w.fieldNames.IndividualFeedbacks: feedbacksBuilder.String(),
		w.fieldNames.AgentScores:         scoresBuilder.String(),
		w.fieldNames.WeightedScore:       weightedScore,
	}

	if contentVersion > 0 {
		consolidatorInputs[w.fieldNames.IterationVersion] = contentVersion
	}

	return consolidatorInputs
}

// consolidateFeedbacks uses consolidator to merge all feedbacks.
func (w *ParallelEvaluationWorkflow) consolidateFeedbacks(
	ctx context.Context,
	individualEvals []*evaluation.IndividualEvaluation,
	agentScores map[string]float64,
	weightedScore float64,
	contentVersion int,
) (string, error) {
	if w.consolidator.Module == nil {
		return "", fmt.Errorf("consolidator not initialized")
	}

	consolidatorInputs := w.buildConsolidatorInputs(individualEvals, agentScores, weightedScore, contentVersion)

	result, err := w.consolidator.Process(ctx, consolidatorInputs)
	if err != nil {
		return "", err
	}

	consolidatedFeedback, ok := result[w.fieldNames.ConsolidatedFeedback].(string)
	if !ok || consolidatedFeedback == "" {
		return "", fmt.Errorf("%s", "consolidator returned empty or invalid feedback")
	}

	return consolidatedFeedback, nil
}

// consolidateFeedbacksStream uses consolidator to merge all feedbacks with streaming support.
func (w *ParallelEvaluationWorkflow) consolidateFeedbacksStream(
	ctx context.Context,
	individualEvals []*evaluation.IndividualEvaluation,
	agentScores map[string]float64,
	weightedScore float64,
	contentVersion int,
	eventChan streaming.EventChannel,
) (string, error) {
	if w.consolidator.Module == nil {
		return "", fmt.Errorf("%s", "consolidator not initialized")
	}

	select {
	case eventChan <- w.consolidator.StartEvent():
	case <-ctx.Done():
		return "", ctx.Err()
	}

	consolidatorInputs := w.buildConsolidatorInputs(individualEvals, agentScores, weightedScore, contentVersion)

	handler := w.consolidator.StreamHandler(eventChan)

	result, err := w.consolidator.Process(
		ctx,
		consolidatorInputs,
		core.WithStreamHandler(handler),
	)

	select {
	case eventChan <- w.consolidator.EndEvent(result, err):
	case <-ctx.Done():
	}

	if err != nil {
		return "", err
	}

	consolidatedFeedback, ok := result[w.fieldNames.ConsolidatedFeedback].(string)
	if !ok || consolidatedFeedback == "" {
		return "", fmt.Errorf("%s", "consolidator returned empty or invalid feedback")
	}

	return consolidatedFeedback, nil
}

func (w *ParallelEvaluationWorkflow) logEvaluatorPayloadSizes(ctx context.Context, roleKey evaluation.EvaluatorKey, inputs map[string]interface{}, parseOK bool) {
	if w.logger == nil {
		return
	}
	genInBytes, genOutBytes := evaluationPayloadByteSizes(inputs)
	fields := map[string]interface{}{
		"evaluator":              roleKey.String(),
		"generator_input_bytes":  genInBytes,
		"generator_output_bytes": genOutBytes,
		"parse_ok":               parseOK,
	}
	phase := compositionPhaseFromEvalInputs(inputs)
	if phase != "" {
		fields["composition_phase"] = phase
	}
	w.logger.WithFields(fields).Info("Evaluation agent payload")
	if c := runreport.CollectorFromContext(ctx); c != nil {
		c.RecordEvaluator(roleKey.String(), phase, parseOK, fields)
	}
}

func evaluationPayloadByteSizes(inputs map[string]interface{}) (generatorInputBytes, generatorOutputBytes int) {
	if inputs == nil {
		return 0, 0
	}
	if genIn, ok := inputs[stropdspy.FieldGeneratorInput]; ok {
		generatorInputBytes = jsonMapByteSize(genIn)
	}
	if genOut, ok := inputs[stropdspy.FieldGeneratorOutput]; ok {
		generatorOutputBytes = jsonMapByteSize(genOut)
	}
	return generatorInputBytes, generatorOutputBytes
}

func compositionPhaseFromEvalInputs(inputs map[string]interface{}) string {
	genIn, ok := inputs[stropdspy.FieldGeneratorInput].(map[string]interface{})
	if !ok {
		return ""
	}
	phase, ok := genIn["composition_phase"].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(phase)
}

func jsonMapByteSize(value interface{}) int {
	b, err := json.Marshal(value)
	if err != nil {
		return 0
	}
	return len(b)
}
