package tracing

import (
	"encoding/json"
	"fmt"

	"github.com/behaviorengineering/strop/dspy/rawresponse"
	kitlog "github.com/behaviorengineering/strop/log"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// addOpenInferenceAttributes adds OpenInference-specific attributes to an OpenTelemetry span.
// This formats the module execution information according to OpenInference conventions.
func addOpenInferenceAttributes(otelSpan trace.Span, moduleInfo *OpenInferenceModuleInfo, logger kitlog.Logger, info *core.ModuleInfo, inputs map[string]any) error {
	if otelSpan == nil {
		return fmt.Errorf("OpenTelemetry span is nil")
	}

	// Build all attribute groups.
	attrs := buildModuleAttributes(moduleInfo)
	attrs = append(attrs, buildInputOutputAttributes(moduleInfo, logger)...)
	attrs = append(attrs, buildPromptTemplateAttributes(info, inputs, logger)...)
	attrs = append(attrs, buildLLMAttributes(moduleInfo, logger)...)

	// Set all attributes on the span.
	otelSpan.SetAttributes(attrs...)

	// The status is set correctly in handleResult() based on the err parameter.

	return nil
}

// buildModuleAttributes builds basic module metadata attributes.
func buildModuleAttributes(moduleInfo *OpenInferenceModuleInfo) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("module.name", moduleInfo.Name),
		attribute.String("module.type", moduleInfo.Type),
	}
}

// buildInputOutputAttributes builds attributes for input and output fields.
// According to OpenInference semantic conventions, inputs and outputs should be set as:
// - input.value: JSON-serialized input values
// - output.value: JSON-serialized output values
// - input.mime_type: MIME type of input (optional, defaults to "application/json")
// - output.mime_type: MIME type of output (optional, defaults to "application/json")
//
// On success, structured outputs are exported and __raw_response is stripped.
// On error (or when only raw text exists), __raw_response is kept so Phoenix shows Output.
func buildInputOutputAttributes(moduleInfo *OpenInferenceModuleInfo, logger kitlog.Logger) []attribute.KeyValue {
	var attrs []attribute.KeyValue

	// Serialize inputs to JSON for input.value.
	if len(moduleInfo.Inputs) > 0 {
		inputJSON, err := json.Marshal(moduleInfo.Inputs)
		if err != nil {
			if logger != nil {
				logger.WithError(err).WithField("module", moduleInfo.Name).Warn("Failed to serialize inputs to JSON")
			}
		} else {
			attrs = append(attrs, attribute.String("input.value", string(inputJSON)))
			attrs = append(attrs, attribute.String("input.mime_type", "application/json"))
		}
	}

	// Use structured outputs (already parsed by XML interceptor, same as application code uses).
	if len(moduleInfo.Outputs) > 0 {
		rawText, rawKey := rawresponse.TextFromInterface(moduleInfo.Outputs)
		// DEBUG: Log what we're seeing before cleanup.
		if logger != nil {
			logger.WithFields(map[string]interface{}{
				"module":           moduleInfo.Name,
				"output_keys":      getOutputKeys(moduleInfo.Outputs),
				"output_count":     len(moduleInfo.Outputs),
				"has_raw_response": rawKey != "",
				"raw_response_key": rawKey,
				"has_error":        moduleInfo.Error != nil,
			}).Debug("OpenInference captured outputs before cleanup")
		}

		outputsToSerialize := removeRawResponse(moduleInfo.Outputs)
		// Keep raw LLM text visible when the call failed or structured fields are absent.
		if rawText != "" && (moduleInfo.Error != nil || len(outputsToSerialize) == 0) {
			if outputsToSerialize == nil {
				outputsToSerialize = make(map[string]interface{})
			}
			outputsToSerialize[rawresponse.CanonicalKey] = rawText
		}

		// DEBUG: Log what we have after cleanup.
		if logger != nil {
			logger.WithFields(map[string]interface{}{
				"module":        moduleInfo.Name,
				"cleaned_keys":  getOutputKeys(outputsToSerialize),
				"cleaned_count": len(outputsToSerialize),
			}).Debug("OpenInference outputs after cleanup")
		}

		if len(outputsToSerialize) > 0 {
			outputJSON, err := json.Marshal(outputsToSerialize)
			if err != nil {
				if logger != nil {
					logger.WithError(err).WithField("module", moduleInfo.Name).Warn("Failed to serialize outputs to JSON")
				}
			} else {
				attrs = append(attrs, attribute.String("output.value", string(outputJSON)))
				attrs = append(attrs, attribute.String("output.mime_type", "application/json"))
			}
		}
	}

	return attrs
}

// buildPromptTemplateAttributes builds prompt template attributes (OpenInference semantic conventions).
// See: https://arize.com/docs/phoenix/tracing/how-to-tracing/add-metadata/instrumenting-prompt-templates-and-prompt-variables
// Note: We don't include llm.prompt_template.variables here because input.value already contains all input data,
// making template variables redundant.
func buildPromptTemplateAttributes(info *core.ModuleInfo, inputs map[string]any, logger kitlog.Logger) []attribute.KeyValue {
	if info == nil || info.Signature.Instruction == "" {
		return nil
	}

	// Extract template from Signature.Instruction (this is the SystemPrompt).
	template := info.Signature.Instruction

	// Use module version as template version.
	templateVersion := info.Version
	if templateVersion == "" {
		templateVersion = defaultVersion
	}

	return []attribute.KeyValue{
		attribute.String("llm.prompt_template.template", template),
		attribute.String("llm.prompt_template.version", templateVersion),
	}
}

// buildLLMAttributes builds LLM-specific attributes for cost tracking (required by Arize).
// See: https://arize.com/docs/phoenix/tracing/how-to-tracing/cost-tracking#span-level-costs
func buildLLMAttributes(moduleInfo *OpenInferenceModuleInfo, logger kitlog.Logger) []attribute.KeyValue {
	var attrs []attribute.KeyValue

	// Set to "unknown" if not found (Arize will handle missing values).
	// Model/provider are set by our decorator on ExecutionState; parallel workflows must use
	// core.WithFreshExecutionState(ctx) per goroutine so each span has isolated state.
	modelName := moduleInfo.Model
	if modelName == "" {
		modelName = defaultModuleName
		if logger != nil {
			logger.WithField("module", moduleInfo.Name).Warn("Model name not found in ExecutionState - cost tracking may not work")
		}
	}
	attrs = append(attrs, attribute.String("llm.model_name", modelName))

	providerName := moduleInfo.Provider
	if providerName == "" {
		providerName = defaultModuleName
		if logger != nil {
			logger.WithFields(map[string]interface{}{
				"module": moduleInfo.Name,
				"model":  moduleInfo.Model,
			}).Warn("Provider not found in configuration mapping - cost tracking may not work. Ensure model is registered in ModuleRegistry.")
		}
	}
	attrs = append(attrs, attribute.String("llm.provider", providerName))

	// Add token counts if available (required for cost tracking).
	if moduleInfo.PromptTokens > 0 {
		attrs = append(attrs, attribute.Int("llm.token_count.prompt", moduleInfo.PromptTokens))
	}
	if moduleInfo.CompletionTokens > 0 {
		attrs = append(attrs, attribute.Int("llm.token_count.completion", moduleInfo.CompletionTokens))
	}
	if moduleInfo.TotalTokens > 0 {
		attrs = append(attrs, attribute.Int("llm.token_count.total", moduleInfo.TotalTokens))
	}

	// Additional LLM attributes (optional but useful).
	if moduleInfo.Prompt != "" {
		attrs = append(attrs, attribute.String("llm.request.prompt", moduleInfo.Prompt))
	}
	if moduleInfo.Response != "" {
		attrs = append(attrs, attribute.String("llm.response.content", moduleInfo.Response))
	}

	// Add request type.
	attrs = append(attrs, attribute.String("llm.request.type", "completion"))

	return attrs
}

// getOutputKeys extracts keys from a map for logging purposes.
func getOutputKeys(outputs map[string]interface{}) []string {
	keys := make([]string, 0, len(outputs))
	for k := range outputs {
		keys = append(keys, k)
	}
	return keys
}
