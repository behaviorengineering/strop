package tracing

import (
	"context"
	"fmt"

	"github.com/behaviorengineering/strop/dspy/rawresponse"
	kitlog "github.com/behaviorengineering/strop/log"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Constants for default values and magic strings.
const (
	defaultServiceName       = "github.com/behaviorengineering/strop"
	defaultModuleName        = "unknown"
	defaultModuleType        = "unknown"
	defaultVersion           = "1.0.0"
	openInferenceSpanKindLLM = "LLM"
	moduleTypeGenerator      = "generator"
	moduleTypeEvaluator      = "evaluator"
	moduleTypeModule         = "module"
	iterationVersionKey      = "iterationVersion"
)

// OpenInferenceModuleInterceptor creates an interceptor that exports execution traces to OpenInference.
// It creates both OpenTelemetry spans (for export) and DSPy spans (for internal tracking).
// enabled indicates whether OpenInference tracing is enabled.
// serviceName is the service name used for OpenTelemetry tracer (should match the service name used in InitOTEL).
// providerLookup is a function that maps model IDs to provider names (e.g., "openai", "anthropic", "google").
// modelIDByModuleName is an optional fallback that maps module display name to model ID when ExecutionState does not contain it.
func OpenInferenceModuleInterceptor(enabled bool, serviceName string, logger kitlog.Logger, providerLookup func(modelID string) string, modelIDByModuleName func(moduleName string) string) core.ModuleInterceptor {
	if !enabled {
		// Return a no-op interceptor if disabled.
		return func(ctx context.Context, inputs map[string]any, info *core.ModuleInfo, handler core.ModuleHandler, opts ...core.Option) (map[string]any, error) {
			return handler(ctx, inputs, opts...)
		}
	}
	// Default service name if not provided.
	if serviceName == "" {
		serviceName = defaultServiceName
	}
	return func(ctx context.Context, inputs map[string]any, info *core.ModuleInfo, handler core.ModuleHandler, opts ...core.Option) (map[string]any, error) {
		// Extract module metadata.
		metadata := extractModuleMetadata(info, inputs)

		// Debug logging to verify interceptor is being called.
		if logger != nil {
			logger.WithFields(map[string]interface{}{
				"module_name": metadata.name,
				"module_type": metadata.moduleType,
				"span_name":   metadata.spanName,
			}).Debug("OpenInference interceptor: Creating span for module")
		}

		// Create spans (handles chain context resolution and span creation).
		spans := createSpans(ctx, metadata, serviceName)
		defer spans.otelSpan.End()
		defer core.EndSpan(spans.ctx)

		// Debug logging to verify span was created.
		if logger != nil {
			spanCtx := spans.otelSpan.SpanContext()
			logger.WithFields(map[string]interface{}{
				"module_name": metadata.name,
				"span_name":   metadata.spanName,
				"trace_id":    spanCtx.TraceID().String(),
				"span_id":     spanCtx.SpanID().String(),
			}).Debug("OpenInference interceptor: Span created successfully")
		}

		// Add initial annotations to spans.
		addInitialAnnotations(spans, metadata, inputs)

		// Debug: Check ExecutionState before handler call.
		if logger != nil {
			if state := core.GetExecutionState(spans.ctx); state != nil {
				logger.WithFields(map[string]interface{}{
					"module":          metadata.name,
					"before_model_id": state.GetModelID(),
					"state_ptr":       fmt.Sprintf("%p", state),
					"has_token_usage": state.GetTokenUsage() != nil,
				}).Debug("ExecutionState before handler call")
			} else {
				logger.WithField("module", metadata.name).Debug("No ExecutionState before handler call")
			}
		}

		// Execute handler.
		result, err := handler(spans.ctx, inputs, opts...)

		// Debug: Check ExecutionState after handler call.
		if logger != nil {
			if state := core.GetExecutionState(spans.ctx); state != nil {
				// Also check the original context to see if ExecutionState changed.
				originalState := core.GetExecutionState(ctx)
				originalStatePtr := "nil"
				if originalState != nil {
					originalStatePtr = fmt.Sprintf("%p", originalState)
				}

				logger.WithFields(map[string]interface{}{
					"module":             metadata.name,
					"after_model_id":     state.GetModelID(),
					"state_ptr":          fmt.Sprintf("%p", state),
					"original_state_ptr": originalStatePtr,
					"state_changed":      originalState != nil && originalState != state,
					"has_token_usage":    state.GetTokenUsage() != nil,
				}).Debug("ExecutionState after handler call")
			} else {
				logger.WithField("module", metadata.name).Debug("No ExecutionState after handler call")
			}
		}

		// Process result and add OpenInference attributes.
		return handleResult(handleResultParams{
			ctx:                 spans.ctx,
			result:              result,
			err:                 err,
			dspySpan:            spans.dspySpan,
			otelSpan:            spans.otelSpan,
			logger:              logger,
			moduleName:          metadata.name,
			moduleType:          metadata.moduleType,
			inputs:              inputs,
			providerLookup:      providerLookup,
			modelIDByModuleName: modelIDByModuleName,
			info:                info,
		})
	}
}

// moduleMetadata groups all module-related metadata for easier passing.
type moduleMetadata struct {
	name           string
	moduleType     string
	version        string
	contentVersion int
	spanName       string
}

// extractModuleMetadata extracts all module metadata from ModuleInfo and inputs.
func extractModuleMetadata(info *core.ModuleInfo, inputs map[string]any) moduleMetadata {
	metadata := moduleMetadata{
		name:       defaultModuleName,
		moduleType: defaultModuleType,
		version:    defaultVersion,
	}

	if info != nil {
		metadata.name = info.ModuleName
		metadata.moduleType = info.ModuleType
		metadata.version = info.Version
	}

	// Extract content version (refinement iteration) from inputs.
	metadata.contentVersion = extractContentVersion(inputs)

	// Create a more descriptive span name based on module type, name, and content version.
	metadata.spanName = buildSpanName(metadata.moduleType, metadata.name, metadata.contentVersion)

	return metadata
}

// spanContext groups span-related state for easier passing.
type spanContext struct {
	otelSpan trace.Span
	dspySpan *core.Span
	ctx      context.Context
}

// - Evaluators called from workflow → children of workflow span.
func resolveChainContext(ctx context.Context) context.Context {
	// OpenTelemetry automatically tracks the active span in the context.
	if span := trace.SpanFromContext(ctx); span != nil && span.SpanContext().IsValid() {
		// Chain spans are stored as context values, so we can compare span IDs.
		_, hasChain := getChainContext(ctx)
		if hasChain {
			// Check if we can get the chain span from context value to compare.
			if chainCtx, ok := ctx.Value(chainSpanCtxKey{}).(context.Context); ok {
				if chainSpan := trace.SpanFromContext(chainCtx); chainSpan != nil {
					// Compare span contexts - if they're different, active span is a workflow span.
					if span.SpanContext().SpanID() != chainSpan.SpanContext().SpanID() {
						// Use current context so modules become children of workflow span.
						return ctx
					}
					// Active span is the chain span itself, fall through to use chain context.
				}
			}
		} else {
			// Use current context.
			return ctx
		}
	}

	// If no active span or active span is chain span, check for chain span context (for modules running directly in chains).
	if _, hasChain := getChainContext(ctx); hasChain {
		// Use package-level chainSpanCtxKey from chain_context.go.
		if existingCtx, ok := ctx.Value(chainSpanCtxKey{}).(context.Context); ok {
			// Chain span already created by WithChainContext, reuse its context.
			return existingCtx
		}
	}

	// If neither active span nor chain span exists, use original context.
	return ctx
}

// createSpans creates both OpenTelemetry and DSPy spans for module execution.
func createSpans(ctx context.Context, metadata moduleMetadata, serviceName string) spanContext {
	// Resolve parent context (chain span if present, otherwise original context).
	parentCtx := resolveChainContext(ctx)

	// Build initial span attributes.
	attrs := []attribute.KeyValue{
		attribute.String("module.name", metadata.name),
		attribute.String("module.type", metadata.moduleType),
		attribute.String("module.version", metadata.version),
		// Set openinference.span.kind to "LLM" for proper categorization in Arize.
		attribute.String("openinference.span.kind", openInferenceSpanKindLLM),
	}
	// Add content version if available.
	if metadata.contentVersion > 0 {
		attrs = append(attrs, attribute.Int("content.version", metadata.contentVersion))
	}

	// Create OpenTelemetry span (child of chain span if present, otherwise child of parent trace).
	tracer := otel.Tracer(serviceName)
	ctx, otelSpan := tracer.Start(parentCtx, metadata.spanName,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attrs...),
	)

	// Note: StartSpanWithContext may not be exported, so we use StartSpan.
	ctx, dspySpan := core.StartSpan(ctx, metadata.spanName)

	return spanContext{
		otelSpan: otelSpan,
		dspySpan: dspySpan,
		ctx:      ctx,
	}
}

// addInitialAnnotations adds initial annotations to both OpenTelemetry and DSPy spans.
func addInitialAnnotations(spans spanContext, metadata moduleMetadata, inputs map[string]any) {
	// Add module metadata to DSPy span.
	addDSPySpanAnnotations(spans.dspySpan, map[string]interface{}{
		"module_name": metadata.name,
		"module_type": metadata.moduleType,
		"version":     metadata.version,
	})

	// Link DSPy span to OpenTelemetry span via trace ID.
	if spans.dspySpan != nil && spans.otelSpan != nil {
		otelSpanCtx := spans.otelSpan.SpanContext()
		addDSPySpanAnnotations(spans.dspySpan, map[string]interface{}{
			"otel.trace_id": otelSpanCtx.TraceID().String(),
			"otel.span_id":  otelSpanCtx.SpanID().String(),
		})
	}

	// Add input information to spans.
	inputFieldNames := getFieldNames(inputs)
	addDSPySpanAnnotations(spans.dspySpan, map[string]interface{}{
		"input_fields": inputFieldNames,
		"inputs":       inputs,
	})
	spans.otelSpan.SetAttributes(
		attribute.String("input.fields", fmt.Sprintf("%v", inputFieldNames)),
	)
}

// handleResultParams groups all parameters for handleResult to reduce parameter count.
type handleResultParams struct {
	ctx                 context.Context
	result              map[string]any
	err                 error
	dspySpan            *core.Span
	otelSpan            trace.Span
	logger              kitlog.Logger
	moduleName          string
	moduleType          string
	inputs              map[string]any
	providerLookup      func(modelID string) string
	modelIDByModuleName func(moduleName string) string
	info                *core.ModuleInfo
}

// handleResult processes the handler result and exports to OpenInference.
func handleResult(params handleResultParams) (map[string]any, error) {
	// Record error in spans if execution failed.
	if params.err != nil {
		if params.dspySpan != nil {
			params.dspySpan.WithError(params.err)
		}
		// Record error as event AND set span status to ERROR.
		params.otelSpan.RecordError(params.err)
		params.otelSpan.SetStatus(codes.Error, params.err.Error())
	} else {
		// Mark span as successful if no error.
		params.otelSpan.SetStatus(codes.Ok, "")
	}

	// Cache field names to avoid duplicate computation.
	if params.result != nil {
		outputFieldNames := getFieldNames(params.result)
		addDSPySpanAnnotations(params.dspySpan, map[string]interface{}{
			"output_fields": outputFieldNames,
			"outputs":       params.result,
		})
		params.otelSpan.SetAttributes(
			attribute.String("output.fields", fmt.Sprintf("%v", outputFieldNames)),
		)
	}

	// Extract module information for OpenInference export.
	moduleInfo := extractModuleInfo(params.ctx, params.moduleName, params.moduleType, params.inputs, params.result, params.err, params.providerLookup, params.modelIDByModuleName, params.logger)

	// 3. Partial tracing data is better than no execution at all.
	if err := addOpenInferenceAttributes(params.otelSpan, moduleInfo, params.logger, params.info, params.inputs); err != nil {
		if params.logger != nil {
			params.logger.WithError(err).Warn("Failed to add OpenInference attributes to span")
		}
	}

	return params.result, params.err
}

// extractModuleInfo extracts information from execution context for OpenInference export.
// providerLookup maps model IDs to provider names. modelIDByModuleName is an optional fallback when ExecutionState has no model ID.
func extractModuleInfo(ctx context.Context, moduleName, moduleType string, inputs, outputs map[string]any, execErr error, providerLookup func(modelID string) string, modelIDByModuleName func(moduleName string) string, logger kitlog.Logger) *OpenInferenceModuleInfo {
	moduleInfo := &OpenInferenceModuleInfo{
		Name:    moduleName,
		Type:    moduleType,
		Inputs:  convertMap(inputs),
		Outputs: convertMap(outputs),
		Error:   execErr,
	}

	// Extract model ID and token usage from ExecutionState.
	if state := core.GetExecutionState(ctx); state != nil {
		modelID := state.GetModelID()
		moduleInfo.Model = string(modelID)

		// Debug: Log ExecutionState details to understand why model ID might be missing.
		if moduleInfo.Model == "" {
			// This should not happen - ModelContextDecorator should set model ID when Generate() is called.
			// Log detailed state information to debug.
			if logger != nil {
				// Try to get expected model ID from module registry if providerLookup is available.
				expectedModelID := ""
				if providerLookup != nil {
					// but we can log that we're trying to diagnose.
					expectedModelID = "unknown - cannot determine without LLM instance"
				}

				logger.WithFields(map[string]interface{}{
					"module":            moduleName,
					"module_type":       moduleType,
					"has_state":         true,
					"state_type":        fmt.Sprintf("%T", state),
					"has_token_usage":   state.GetTokenUsage() != nil,
					"expected_model_id": expectedModelID,
				}).Debug("ExecutionState exists but model ID is empty - ModelContextDecorator may not have set it, or LLM.ModelID() returned empty string")
			}
		} else {
			// Model ID is set - log it for verification.
			if logger != nil {
				logger.WithFields(map[string]interface{}{
					"module":          moduleName,
					"model_id":        moduleInfo.Model,
					"has_token_usage": state.GetTokenUsage() != nil,
				}).Debug("Model ID successfully extracted from ExecutionState")
			}
		}

		// Model ID should be set by ModelContextDecorator when Generate() is called.
		if providerLookup != nil && moduleInfo.Model != "" {
			moduleInfo.Provider = providerLookup(moduleInfo.Model)
		}
		// This is intentional - provider must be configured, no inference fallback.

		if tokenUsage := state.GetTokenUsage(); tokenUsage != nil {
			moduleInfo.PromptTokens = tokenUsage.PromptTokens
			moduleInfo.CompletionTokens = tokenUsage.CompletionTokens
			moduleInfo.TotalTokens = tokenUsage.TotalTokens
		}

		// Extract prompt and response from span annotations if available.
		if span := state.GetCurrentSpan(); span != nil {
			if span.Annotations != nil {
				if prompt, ok := span.Annotations["prompt"].(string); ok {
					moduleInfo.Prompt = prompt
				}
				if response, ok := span.Annotations["response"].(string); ok {
					moduleInfo.Response = response
				}
			}
		}
	}

	// Fall back to raw LLM text in outputs (critical on XML parse failures).
	if moduleInfo.Response == "" {
		if text, _ := rawresponse.TextFromInterface(moduleInfo.Outputs); text != "" {
			moduleInfo.Response = text
		}
	}

	// Fallback: when ExecutionState did not provide model ID (e.g. context not propagated to LLM.Generate), use module registry.
	if moduleInfo.Model == "" && modelIDByModuleName != nil {
		moduleInfo.Model = modelIDByModuleName(moduleName)
	}
	if moduleInfo.Model != "" && providerLookup != nil && moduleInfo.Provider == "" {
		moduleInfo.Provider = providerLookup(moduleInfo.Model)
	}

	return moduleInfo
}

// OpenInferenceModuleInfo contains information about a module execution for OpenInference export.
type OpenInferenceModuleInfo struct {
	Name             string
	Type             string
	Inputs           map[string]interface{}
	Outputs          map[string]interface{}
	Model            string
	Provider         string // LLM provider (e.g., "openai", "anthropic", "google", "perplexity").
	Prompt           string
	Response         string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	Error            error
}
