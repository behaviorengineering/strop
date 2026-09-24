package workflow

import (
	"context"
	"fmt"
	"time"

	"github.com/behaviorengineering/strop/pkg/dspy/actor"
	"github.com/behaviorengineering/strop/pkg/evaluation"
	"github.com/behaviorengineering/strop/pkg/evaluation/criteria"
	stroplog "github.com/behaviorengineering/strop/pkg/log"
	"github.com/behaviorengineering/strop/pkg/streaming"

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
