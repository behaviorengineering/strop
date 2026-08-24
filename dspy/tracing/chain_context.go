package tracing

import (
	"context"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// ChainType represents the type of content generation chain.
// Pipelines define their own chain type constants (see internal/pipelines/*/tracing).
type ChainType string

// chainContextKey is the context key for storing chain type.
type chainContextKey struct{}

// chainSpanKey is the context key for storing the chain span itself.
type chainSpanKey struct{}

// This must be defined at package level so all functions use the same type.
type chainSpanCtxKey struct{}

// ChainSpanCtxKeyType returns the type of the chain span context key.
// This is used by the workflow to check for chain context.
func ChainSpanCtxKeyType() interface{} {
	return chainSpanCtxKey{}
}

// WithChainContext adds chain type information to the context and creates the chain span.
// This allows the OpenInference interceptor to reuse the chain span across multiple modules.
// The chain span is created here (in the service) so it's available to both generators and evaluators.
// serviceName is the service name used for OpenTelemetry tracer (should match the service name used in InitOTEL).
func WithChainContext(ctx context.Context, chainType ChainType, serviceName string) context.Context {
	// Default service name if not provided.
	if serviceName == "" {
		serviceName = defaultServiceName
	}

	// This ensures the chain span exists before any modules are processed.
	tracer := otel.Tracer(serviceName)
	chainSpanName := string(chainType) + ".chain"
	chainCtx, chainSpan := tracer.Start(ctx, chainSpanName, //nolint:staticcheck // ctx is used as parameter to tracer.Start
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.String("chain.type", string(chainType)),
			attribute.String("openinference.span.kind", "CHAIN"),
		))

	// Store chain type in chainCtx (child contexts don't automatically inherit parent values).
	chainCtx = context.WithValue(chainCtx, chainContextKey{}, chainType)

	// Use package-level chainSpanCtxKey so all functions use the same type.
	chainCtx = context.WithValue(chainCtx, chainSpanCtxKey{}, chainCtx)
	chainCtx = context.WithValue(chainCtx, chainSpanKey{}, chainSpan)

	// for EndChainSpanIfPresent to find it.
	return chainCtx
}

// getChainContext retrieves the chain type from context, if present.
func getChainContext(ctx context.Context) (ChainType, bool) {
	chainType, ok := ctx.Value(chainContextKey{}).(ChainType)
	return chainType, ok
}

// EndChainSpanIfPresent ends the chain span if it exists in the context.
// This should be called at the end of service ProcessSaying methods to ensure
// the chain span is properly closed. If no chain span exists, this is a no-op.
// If an error occurred, it should be passed to set the span status appropriately.
// err is a pointer to the named return value so it can read the final error state.
func EndChainSpanIfPresent(ctx context.Context, err *error) {
	if chainSpan, ok := ctx.Value(chainSpanKey{}).(trace.Span); ok && chainSpan != nil {
		if err != nil && *err != nil {
			chainSpan.RecordError(*err)
			chainSpan.SetStatus(codes.Error, (*err).Error())
		} else {
			chainSpan.SetStatus(codes.Ok, "")
		}
		chainSpan.End()
	}
}

// MarkStoppingConditionOnChainSpan marks the chain span with information about why refinement stopped.
// This provides visual feedback in OpenTelemetry traces when an evaluation wasn't considered better.
// If no chain span exists, this is a no-op.
func MarkStoppingConditionOnChainSpan(ctx context.Context, version int, currentScore, previousScore float64, reason string) {
	if chainSpan, ok := ctx.Value(chainSpanKey{}).(trace.Span); ok && chainSpan != nil {
		chainSpan.SetAttributes(
			attribute.String("refinement.stopped", "true"),
			attribute.String("refinement.stopping_reason", reason),
			attribute.Int("refinement.stopped_at_version", version),
			attribute.Float64("refinement.current_score", currentScore),
			attribute.Float64("refinement.final_score", currentScore), // Final score is the current score.
		)

		// Handle different stopping reasons.
		if reason == "perfect_score" {
			// Perfect score case: both scores are the same (10.0).
			chainSpan.SetAttributes(
				attribute.Float64("refinement.previous_score", currentScore), // Same as current for perfect score.
			)
			chainSpan.AddEvent("Refinement stopped: perfect score achieved", trace.WithAttributes(
				attribute.String("reason", reason),
				attribute.Int("version", version),
				attribute.Float64("score", currentScore),
				attribute.Float64("final_score", currentScore),
			))
		} else {
			// Score decreased case.
			chainSpan.SetAttributes(
				attribute.Float64("refinement.previous_score", previousScore), // The accepted (higher) score.
				attribute.Float64("refinement.score_delta", currentScore-previousScore),
			)
			chainSpan.AddEvent("Refinement stopped: score decreased", trace.WithAttributes(
				attribute.String("reason", reason),
				attribute.Int("version", version),
				attribute.Float64("current_score", currentScore),   // Rejected score.
				attribute.Float64("previous_score", previousScore), // Accepted score.
				attribute.Float64("final_score", previousScore),    // Final accepted score.
			))
		}
	}
}

// MarkDemoSelectionOnChainSpan records near/contrast learning demo artifact IDs on the chain span.
// No-op when no chain span exists or both IDs are empty. Visible in Phoenix/OpenInference traces.
func MarkDemoSelectionOnChainSpan(ctx context.Context, job, step, nearID, contrastID string) {
	chainSpan, ok := ctx.Value(chainSpanKey{}).(trace.Span)
	if !ok || chainSpan == nil {
		return
	}
	nearID = strings.TrimSpace(nearID)
	contrastID = strings.TrimSpace(contrastID)
	if nearID == "" && contrastID == "" {
		return
	}
	job = strings.TrimSpace(job)
	step = strings.TrimSpace(step)

	attrs := make([]attribute.KeyValue, 0, 4)
	if job != "" {
		attrs = append(attrs, attribute.String("learning.demo.job", job))
	}
	if step != "" {
		attrs = append(attrs, attribute.String("learning.demo.step", step))
	}
	if nearID != "" {
		attrs = append(attrs, attribute.String("learning.demo.near_id", nearID))
	}
	if contrastID != "" {
		attrs = append(attrs, attribute.String("learning.demo.contrast_id", contrastID))
	}
	chainSpan.SetAttributes(attrs...)
	chainSpan.AddEvent("Learning demos applied", trace.WithAttributes(attrs...))
}
