package tracing

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestMarkDemoSelectionOnChainSpan_noChainSpan(t *testing.T) {
	t.Parallel()
	// Must not panic when chain span is absent.
	MarkDemoSelectionOnChainSpan(context.Background(), "job", "step", "near", "contrast")
}

func TestMarkDemoSelectionOnChainSpan_setsAttributes(t *testing.T) {
	sr := setupTestTracerProvider(t)
	ctx := WithChainContext(context.Background(), ChainType("post_generation"), testTracingServiceName)

	MarkDemoSelectionOnChainSpan(ctx, "youtube_chapters_generation", "chapters",
		"11111111-1111-1111-1111-111111111111",
		"22222222-2222-2222-2222-222222222222",
	)

	var err error
	EndChainSpanIfPresent(ctx, &err)

	span := requireSpan(t, tracetest.SpanStubsFromReadOnlySpans(sr.Ended()), "post_generation.chain")
	near, ok := attributeValue(span, "learning.demo.near_id")
	require.True(t, ok)
	assert.Equal(t, "11111111-1111-1111-1111-111111111111", near.AsString())

	contrast, ok := attributeValue(span, "learning.demo.contrast_id")
	require.True(t, ok)
	assert.Equal(t, "22222222-2222-2222-2222-222222222222", contrast.AsString())

	job, ok := attributeValue(span, "learning.demo.job")
	require.True(t, ok)
	assert.Equal(t, "youtube_chapters_generation", job.AsString())

	step, ok := attributeValue(span, "learning.demo.step")
	require.True(t, ok)
	assert.Equal(t, "chapters", step.AsString())

	require.Len(t, span.Events, 1)
	assert.Equal(t, "Learning demos applied", span.Events[0].Name)
}

func TestMarkDemoSelectionOnChainSpan_nearOnly(t *testing.T) {
	sr := setupTestTracerProvider(t)
	ctx := WithChainContext(context.Background(), ChainType("composition"), testTracingServiceName)

	MarkDemoSelectionOnChainSpan(ctx, "", "", "11111111-1111-1111-1111-111111111111", "")

	var err error
	EndChainSpanIfPresent(ctx, &err)

	span := requireSpan(t, tracetest.SpanStubsFromReadOnlySpans(sr.Ended()), "composition.chain")
	_, hasContrast := attributeValue(span, "learning.demo.contrast_id")
	assert.False(t, hasContrast)
}

func TestMarkDemoSelectionOnChainSpan_emptyIDsNoOp(t *testing.T) {
	sr := setupTestTracerProvider(t)
	ctx := WithChainContext(context.Background(), ChainType("composition"), testTracingServiceName)

	MarkDemoSelectionOnChainSpan(ctx, "job", "step", "", "  ")

	var err error
	EndChainSpanIfPresent(ctx, &err)

	span := requireSpan(t, tracetest.SpanStubsFromReadOnlySpans(sr.Ended()), "composition.chain")
	_, hasNear := attributeValue(span, "learning.demo.near_id")
	assert.False(t, hasNear)
}
