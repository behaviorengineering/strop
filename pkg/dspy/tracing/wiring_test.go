package tracing

import (
	"context"
	"errors"
	"testing"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

const testTracingServiceName = "test-tracing-service"

func setupTestTracerProvider(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()

	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))

	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
		otel.SetTracerProvider(previous)
	})

	return sr
}

func spanByName(spans tracetest.SpanStubs, name string) (tracetest.SpanStub, bool) {
	for _, span := range spans {
		if span.Name == name {
			return span, true
		}
	}
	return tracetest.SpanStub{}, false
}

func requireSpan(t *testing.T, spans tracetest.SpanStubs, name string) tracetest.SpanStub {
	t.Helper()

	span, ok := spanByName(spans, name)
	require.True(t, ok, "expected span %q, got names: %v", name, spanNames(spans))
	return span
}

func spanNames(spans tracetest.SpanStubs) []string {
	names := make([]string, 0, len(spans))
	for _, span := range spans {
		names = append(names, span.Name)
	}
	return names
}

func attributeValue(span tracetest.SpanStub, key string) (attribute.Value, bool) {
	for _, attr := range span.Attributes {
		if string(attr.Key) == key {
			return attr.Value, true
		}
	}
	return attribute.Value{}, false
}

func startTestChainSpan(ctx context.Context, serviceName, operationName string) (context.Context, trace.Span) {
	tracer := otel.Tracer(serviceName)
	ctx, span := tracer.Start(ctx, operationName, trace.WithAttributes(
		attribute.String("openinference.span.kind", "CHAIN"),
	))
	ctx = core.WithExecutionState(ctx)
	return ctx, span
}

func endSpanWithStatus(span trace.Span, err *error) {
	if err != nil && *err != nil {
		span.RecordError(*err)
		span.SetStatus(codes.Error, (*err).Error())
	} else {
		span.SetStatus(codes.Ok, "")
	}
	span.End()
}

func TestTracingWiring_RootChainModuleHierarchy(t *testing.T) {
	sr := setupTestTracerProvider(t)

	ctx := context.Background()
	var err error

	ctx, rootSpan := startTestChainSpan(ctx, testTracingServiceName, "test.generate.translation")
	ctx = WithChainContext(ctx, ChainType("translation"), testTracingServiceName)

	interceptor := OpenInferenceModuleInterceptor(
		true,
		testTracingServiceName,
		nil,
		nil,
		nil,
	)

	mockHandler := func(ctx context.Context, inputs map[string]any, opts ...core.Option) (map[string]any, error) {
		return map[string]any{"translation": "A bird in the hand is worth two in the bush"}, nil
	}

	info := &core.ModuleInfo{
		ModuleName: "TranslationGenerator",
		ModuleType: moduleTypeGenerator,
		Version:    defaultVersion,
	}
	inputs := map[string]any{
		iterationVersionKey: 2,
		"original_text":     "Mas vale pajaro en mano",
	}

	_, err = interceptor(ctx, inputs, info, mockHandler)
	require.NoError(t, err)

	EndChainSpanIfPresent(ctx, &err)
	endSpanWithStatus(rootSpan, &err)

	spans := tracetest.SpanStubsFromReadOnlySpans(sr.Ended())
	require.GreaterOrEqual(t, len(spans), 3, "span names: %v", spanNames(spans))

	root := requireSpan(t, spans, "test.generate.translation")
	chain := requireSpan(t, spans, "translation.chain")
	module := requireSpan(t, spans, "generator.TranslationGenerator.v2")

	traceID := root.SpanContext.TraceID()
	assert.Equal(t, traceID, chain.SpanContext.TraceID())
	assert.Equal(t, traceID, module.SpanContext.TraceID())

	assert.Equal(t, chain.SpanContext.SpanID(), module.Parent.SpanID())
	assert.Equal(t, root.SpanContext.SpanID(), chain.Parent.SpanID())

	kind, ok := attributeValue(root, "openinference.span.kind")
	require.True(t, ok)
	assert.Equal(t, "CHAIN", kind.AsString())

	chainType, ok := attributeValue(chain, "chain.type")
	require.True(t, ok)
	assert.Equal(t, "translation", chainType.AsString())

	moduleKind, ok := attributeValue(module, "openinference.span.kind")
	require.True(t, ok)
	assert.Equal(t, openInferenceSpanKindLLM, moduleKind.AsString())

	moduleName, ok := attributeValue(module, "module.name")
	require.True(t, ok)
	assert.Equal(t, "TranslationGenerator", moduleName.AsString())

	assert.Equal(t, codes.Ok, root.Status.Code)
	assert.Equal(t, codes.Ok, chain.Status.Code)
	assert.Equal(t, codes.Ok, module.Status.Code)
}

func TestTracingWiring_ErrorPropagatesToParentSpans(t *testing.T) {
	sr := setupTestTracerProvider(t)

	ctx := context.Background()
	var err error

	ctx, rootSpan := startTestChainSpan(ctx, testTracingServiceName, "test.generate.translation")
	ctx = WithChainContext(ctx, ChainType("translation"), testTracingServiceName)

	interceptor := OpenInferenceModuleInterceptor(true, testTracingServiceName, nil, nil, nil)

	handlerErr := errors.New("mock LLM failure")
	mockHandler := func(ctx context.Context, inputs map[string]any, opts ...core.Option) (map[string]any, error) {
		return nil, handlerErr
	}

	info := &core.ModuleInfo{
		ModuleName: "TranslationGenerator",
		ModuleType: moduleTypeGenerator,
		Version:    defaultVersion,
	}

	_, err = interceptor(ctx, map[string]any{iterationVersionKey: 1}, info, mockHandler)
	require.Error(t, err)

	EndChainSpanIfPresent(ctx, &err)
	endSpanWithStatus(rootSpan, &err)

	spans := tracetest.SpanStubsFromReadOnlySpans(sr.Ended())

	root := requireSpan(t, spans, "test.generate.translation")
	chain := requireSpan(t, spans, "translation.chain")
	module := requireSpan(t, spans, "generator.TranslationGenerator.v1")

	assert.Equal(t, codes.Error, module.Status.Code)
	assert.Equal(t, codes.Error, chain.Status.Code)
	assert.Equal(t, codes.Error, root.Status.Code)
}

func TestTracingWiring_InterceptorDisabled_SkipsModuleSpan(t *testing.T) {
	sr := setupTestTracerProvider(t)

	ctx := context.Background()
	var err error

	ctx, rootSpan := startTestChainSpan(ctx, testTracingServiceName, "test.generate.translation")
	ctx = WithChainContext(ctx, ChainType("translation"), testTracingServiceName)

	interceptor := OpenInferenceModuleInterceptor(false, testTracingServiceName, nil, nil, nil)

	mockHandler := func(ctx context.Context, inputs map[string]any, opts ...core.Option) (map[string]any, error) {
		return map[string]any{"translation": "ok"}, nil
	}

	info := &core.ModuleInfo{
		ModuleName: "TranslationGenerator",
		ModuleType: moduleTypeGenerator,
		Version:    defaultVersion,
	}

	_, err = interceptor(ctx, map[string]any{iterationVersionKey: 1}, info, mockHandler)
	require.NoError(t, err)

	EndChainSpanIfPresent(ctx, &err)
	endSpanWithStatus(rootSpan, &err)

	spans := tracetest.SpanStubsFromReadOnlySpans(sr.Ended())
	requireSpan(t, spans, "test.generate.translation")
	requireSpan(t, spans, "translation.chain")
	_, hasModuleSpan := spanByName(spans, "generator.TranslationGenerator.v1")
	assert.False(t, hasModuleSpan, "disabled interceptor should not create module spans")
}
