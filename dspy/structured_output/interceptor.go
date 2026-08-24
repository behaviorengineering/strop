package structured_output

import (
	"context"
	"fmt"

	"github.com/behaviorengineering/strop/dspy/rawresponse"
	"github.com/behaviorengineering/strop/dspy/structured_output/xml"
	"github.com/behaviorengineering/strop/streaming"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
)

// StructuredOutputInterceptor creates a combined interceptor that both formats requests and parses responses.
// This follows the same pattern as dspy-go's XMLModuleInterceptor but is format-agnostic.
func StructuredOutputInterceptor(parser Parser, config Config) core.ModuleInterceptor {
	formatInterceptor := FormatInterceptor(parser, config)
	parseInterceptor := ParseInterceptor(parser, config)

	return core.ChainModuleInterceptors(formatInterceptor, parseInterceptor)
}

// FormatInterceptor modifies module inputs to request structured output format.
// This interceptor operates on the input side, injecting formatting instructions
// into the prompt to guide the LLM to produce structured output.
func FormatInterceptor(parser Parser, config Config) core.ModuleInterceptor {
	return func(ctx context.Context, inputs map[string]any, info *core.ModuleInfo, handler core.ModuleHandler, opts ...core.Option) (map[string]any, error) {
		// Create modified inputs with formatting instructions.
		modifiedInputs := make(map[string]any)
		for k, v := range inputs {
			modifiedInputs[k] = v
		}

		// Generate formatting instructions based on the signature.
		signature := parseSignatureForStructuredOutput(config, info, inputs)
		instructions, err := parser.GenerateInstructions(signature, config)
		if err != nil {
			return nil, err
		}
		if config.FormatInstructionsSupplement != nil {
			if phaseSupplement := config.FormatInstructionsSupplement(inputs); phaseSupplement != "" {
				instructions += "\n\n" + phaseSupplement
			}
		}

		// Inject formatting instructions into the appropriate input field.
		if err := parser.InjectInstructions(modifiedInputs, instructions, info.Signature); err != nil {
			return nil, err
		}

		// Call the next handler with modified inputs.
		results, err := handler(ctx, modifiedInputs, opts...)
		return results, err
	}
}

// ParseInterceptor extracts structured data from formatted responses.
// This interceptor operates on the output side, parsing formatted LLM responses
// into structured field values according to the module's signature.
func ParseInterceptor(parser Parser, config Config) core.ModuleInterceptor {
	return func(ctx context.Context, inputs map[string]any, info *core.ModuleInfo, handler core.ModuleHandler, opts ...core.Option) (map[string]any, error) {
		// Call the handler first to get the raw outputs.
		outputs, err := handler(ctx, inputs, opts...)
		if err != nil {
			return nil, err
		}

		// Log raw outputs for debugging.
		if config.Logger != nil {
			outputKeys := make([]string, 0, len(outputs))
			for k := range outputs {
				outputKeys = append(outputKeys, k)
			}
			config.Logger.WithFields(map[string]interface{}{
				"module":       info.ModuleName,
				"module_type":  info.ModuleType,
				"output_keys":  outputKeys,
				"output_count": len(outputs),
			}).Debug("Structured output parser: received raw outputs")

			// Log raw response if available (truncated for readability).
			if textStr, rawKey := rawresponse.TextFrom(outputs); rawKey != "" {
				preview := textStr
				if len(preview) > 500 {
					preview = preview[:500] + "... (truncated)"
				}
				config.Logger.WithFields(map[string]interface{}{
					"module":               info.ModuleName,
					"raw_response_key":     rawKey,
					"raw_response_length":  len(textStr),
					"raw_response_preview": preview,
				}).Debug("Structured output parser: raw response found")
			}
		}

		// Parse structured output from the outputs.
		parseSignature := parseSignatureForStructuredOutput(config, info, inputs)
		parsedOutputs, err := parser.ParseOutputs(ctx, outputs, parseSignature, config)
		if err != nil {
			streaming.EmitWarningf(ctx, info.ModuleName,
				"Structured output parse failed (another attempt may run if retries are enabled): %v", err)
			if config.Logger != nil {
				config.Logger.WithFields(map[string]interface{}{
					"module":      info.ModuleName,
					"module_type": info.ModuleType,
					"error":       err.Error(),
				}).Warn("Structured output parser: parsing failed")
			}
			if config.FallbackToText {
				// Return original outputs if parsing fails and fallback is enabled.
				if config.Logger != nil {
					config.Logger.WithFields(map[string]interface{}{
						"module": info.ModuleName,
					}).Debug("Structured output parser: falling back to raw outputs")
				}
				return outputs, nil
			}
			// Keep raw outputs with the error so OpenInference can attach them to the span.
			return outputs, err
		}

		// Log parsed outputs for debugging.
		if config.Logger != nil {
			parsedKeys := make([]string, 0, len(parsedOutputs))
			for k := range parsedOutputs {
				parsedKeys = append(parsedKeys, k)
			}
			config.Logger.WithFields(map[string]interface{}{
				"module":       info.ModuleName,
				"parsed_keys":  parsedKeys,
				"parsed_count": len(parsedOutputs),
			}).Debug("Structured output parser: parsing succeeded")
		}

		// Preserve canonical raw key so downstream code can try extraction fallbacks when a parsed field is missing.
		if _, srcKey := rawresponse.TextFrom(outputs); srcKey != "" {
			parsedOutputs[rawresponse.CanonicalKey] = outputs[srcKey]
		}
		if config.AfterParse != nil {
			config.AfterParse(inputs, parsedOutputs, info)
		}
		return parsedOutputs, nil
	}
}

func parseSignatureForStructuredOutput(config Config, info *core.ModuleInfo, inputs map[string]any) core.Signature {
	signature := info.Signature
	if config.AdjustParseSignature != nil {
		signature = config.AdjustParseSignature(info, inputs, signature)
	}
	return signature
}

// GetParser returns a parser instance for the specified format.
func GetParser(format Format, config Config) (Parser, error) {
	switch format {
	case FormatXML:
		return xml.NewXMLParser(), nil
	case FormatJSON:
		// Future: return json.NewJSONParser(), nil.
		return nil, fmt.Errorf("JSON format not yet implemented")
	default:
		return nil, fmt.Errorf("unsupported format: %s", format)
	}
}
