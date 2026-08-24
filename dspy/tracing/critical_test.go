package tracing

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	kitlog "github.com/behaviorengineering/strop/log"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace/noop"
)

// This is critical because wrong span names break observability and trace grouping.
func TestBuildSpanName(t *testing.T) {
	tests := []struct {
		name           string
		moduleType     string
		moduleName     string
		contentVersion int
		expected       string
	}{
		// Default cases.
		{
			name:           "default module and type, no version",
			moduleType:     defaultModuleType,
			moduleName:     defaultModuleName,
			contentVersion: 0,
			expected:       "module.process",
		},
		{
			name:           "default module and type, with version",
			moduleType:     defaultModuleType,
			moduleName:     defaultModuleName,
			contentVersion: 3,
			expected:       "module.process.v3",
		},
		{
			name:           "default module, known type, no version",
			moduleType:     moduleTypeGenerator,
			moduleName:     defaultModuleName,
			contentVersion: 0,
			expected:       "generator.process",
		},
		{
			name:           "default module, known type, with version",
			moduleType:     moduleTypeGenerator,
			moduleName:     defaultModuleName,
			contentVersion: 2,
			expected:       "generator.process.v2",
		},
		// Type inference from name.
		{
			name:           "infer generator from name suffix",
			moduleType:     defaultModuleType,
			moduleName:     "TranslationGenerator",
			contentVersion: 0,
			expected:       "generator.TranslationGenerator",
		},
		{
			name:           "infer evaluator from name contains Evaluator",
			moduleType:     defaultModuleType,
			moduleName:     "LinkedIn Professional Evaluator",
			contentVersion: 2,
			expected:       "evaluator.LinkedInProfessional.v2",
		},
		{
			name:           "infer evaluator from name contains Consolidator",
			moduleType:     defaultModuleType,
			moduleName:     "Feedback Consolidator",
			contentVersion: 1,
			expected:       "evaluator.Feedback.v1", // cleanModuleName removes "Consolidator" suffix.
		},
		{
			name:           "infer module for unknown name",
			moduleType:     defaultModuleType,
			moduleName:     "SomeOtherModule",
			contentVersion: 0,
			expected:       "module.SomeOtherModule",
		},
		// Known type with name.
		{
			name:           "known generator type with name",
			moduleType:     moduleTypeGenerator,
			moduleName:     "TranslationGenerator",
			contentVersion: 3,
			expected:       "generator.TranslationGenerator.v3",
		},
		{
			name:           "known evaluator type with name",
			moduleType:     moduleTypeEvaluator,
			moduleName:     "LinkedIn Professional Evaluator",
			contentVersion: 2,
			expected:       "evaluator.LinkedInProfessional.v2",
		},
		// Name cleaning edge cases.
		{
			name:           "name with spaces and Evaluator suffix",
			moduleType:     moduleTypeEvaluator,
			moduleName:     "LinkedIn Professional Evaluator",
			contentVersion: 0,
			expected:       "evaluator.LinkedInProfessional",
		},
		{
			name:           "name with Consolidator suffix",
			moduleType:     moduleTypeEvaluator,
			moduleName:     "Feedback Consolidator",
			contentVersion: 0,
			expected:       "evaluator.Feedback", // cleanModuleName removes "Consolidator" suffix.
		},
		{
			name:           "name without suffix",
			moduleType:     moduleTypeGenerator,
			moduleName:     "TranslationGenerator",
			contentVersion: 0,
			expected:       "generator.TranslationGenerator",
		},
		// Edge cases.
		{
			name:           "empty module name (not defaultModuleName, so processed as-is)",
			moduleType:     moduleTypeGenerator,
			moduleName:     "",
			contentVersion: 0,
			expected:       "generator.", // Empty string is cleaned to empty, not treated as default.
		},
		{
			name:           "negative version (should be ignored)",
			moduleType:     moduleTypeGenerator,
			moduleName:     "TranslationGenerator",
			contentVersion: -1,
			expected:       "generator.TranslationGenerator",
		},
		{
			name:           "very large version",
			moduleType:     moduleTypeGenerator,
			moduleName:     "TranslationGenerator",
			contentVersion: 999,
			expected:       "generator.TranslationGenerator.v999",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildSpanName(tt.moduleType, tt.moduleName, tt.contentVersion)
			assert.Equal(t, tt.expected, result, "Span name mismatch")
		})
	}
}

// This is critical because wrong version extraction breaks refinement tracking.
func TestExtractVersionFromValue(t *testing.T) {
	tests := []struct {
		name     string
		version  interface{}
		expected int
	}{
		{
			name:     "nil value",
			version:  nil,
			expected: 0,
		},
		{
			name:     "int value",
			version:  5,
			expected: 5,
		},
		{
			name:     "int zero",
			version:  0,
			expected: 0,
		},
		{
			name:     "int negative",
			version:  -1,
			expected: -1,
		},
		{
			name:     "float64 value (JSON unmarshaling)",
			version:  float64(3),
			expected: 3,
		},
		{
			name:     "float64 with decimal (should truncate)",
			version:  float64(3.7),
			expected: 3,
		},
		{
			name:     "float64 zero",
			version:  float64(0),
			expected: 0,
		},
		{
			name:     "string value",
			version:  "5",
			expected: 5,
		},
		{
			name:     "string zero",
			version:  "0",
			expected: 0,
		},
		{
			name:     "string with whitespace",
			version:  " 5 ",
			expected: 5,
		},
		{
			name:     "invalid string",
			version:  "not a number",
			expected: 0,
		},
		{
			name:     "empty string",
			version:  "",
			expected: 0,
		},
		{
			name:     "string with non-numeric prefix",
			version:  "abc123",
			expected: 0,
		},
		{
			name:     "string with negative number",
			version:  "-5",
			expected: -5,
		},
		// Edge cases that could cause bugs.
		{
			name:     "bool false (should return 0)",
			version:  false,
			expected: 0,
		},
		{
			name:     "bool true (should return 0)",
			version:  true,
			expected: 0,
		},
		{
			name:     "slice (should return 0)",
			version:  []int{1, 2, 3},
			expected: 0,
		},
		{
			name:     "map (should return 0)",
			version:  map[string]int{"version": 5},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractVersionFromValue(tt.version)
			assert.Equal(t, tt.expected, result, "Version extraction mismatch")
		})
	}
}

// This is critical because wrong version extraction breaks refinement tracking.
func TestExtractContentVersion(t *testing.T) {
	tests := []struct {
		name     string
		inputs   map[string]any
		expected int
	}{
		{
			name:     "nil inputs",
			inputs:   nil,
			expected: 0,
		},
		{
			name:     "empty inputs",
			inputs:   map[string]any{},
			expected: 0,
		},
		{
			name: "iterationVersion as int",
			inputs: map[string]any{
				iterationVersionKey: 3,
				"other":             "value",
			},
			expected: 3,
		},
		{
			name: "iterationVersion as float64",
			inputs: map[string]any{
				iterationVersionKey: float64(5),
			},
			expected: 5,
		},
		{
			name: "iterationVersion as string",
			inputs: map[string]any{
				iterationVersionKey: "7",
			},
			expected: 7,
		},
		{
			name: "iterationVersion zero (should return 0)",
			inputs: map[string]any{
				iterationVersionKey: 0,
			},
			expected: 0,
		},
		{
			name: "iterationVersion negative (filtered out by v > 0 check)",
			inputs: map[string]any{
				iterationVersionKey: -1,
			},
			expected: 0, // extractContentVersion filters out negative versions (v > 0 check).
		},
		{
			name: "missing iterationVersion key",
			inputs: map[string]any{
				"other": "value",
			},
			expected: 0,
		},
		{
			name: "invalid iterationVersion value",
			inputs: map[string]any{
				iterationVersionKey: "not a number",
			},
			expected: 0,
		},
		{
			name: "iterationVersion nil",
			inputs: map[string]any{
				iterationVersionKey: nil,
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractContentVersion(tt.inputs)
			assert.Equal(t, tt.expected, result, "Content version extraction mismatch")
		})
	}
}

// This is critical because wrong serialization breaks observability in Arize.
func TestBuildInputOutputAttributes(t *testing.T) {
	var logger kitlog.Logger

	tests := []struct {
		name           string
		moduleInfo     *OpenInferenceModuleInfo
		expectedAttrs  int // Minimum expected attributes.
		shouldHaveJSON bool
	}{
		{
			name: "inputs and outputs with valid data",
			moduleInfo: &OpenInferenceModuleInfo{
				Name:    "TestModule",
				Inputs:  map[string]interface{}{"field1": "value1"},
				Outputs: map[string]interface{}{"field2": "value2"},
			},
			expectedAttrs:  4, // input.value, input.mime_type, output.value, output.mime_type.
			shouldHaveJSON: true,
		},
		{
			name: "only inputs",
			moduleInfo: &OpenInferenceModuleInfo{
				Name:   "TestModule",
				Inputs: map[string]interface{}{"field1": "value1"},
			},
			expectedAttrs:  2, // input.value, input.mime_type.
			shouldHaveJSON: true,
		},
		{
			name: "only outputs",
			moduleInfo: &OpenInferenceModuleInfo{
				Name:    "TestModule",
				Outputs: map[string]interface{}{"field2": "value2"},
			},
			expectedAttrs:  2, // output.value, output.mime_type.
			shouldHaveJSON: true,
		},
		{
			name: "outputs with XML in response field",
			moduleInfo: &OpenInferenceModuleInfo{
				Name: "TestModule",
				Outputs: map[string]interface{}{
					"response": "<response><field1>value1</field1><field2>value2</field2></response>",
					"other":    "other_value",
				},
			},
			expectedAttrs:  2, // output.value, output.mime_type.
			shouldHaveJSON: true,
		},
		{
			name: "empty inputs and outputs",
			moduleInfo: &OpenInferenceModuleInfo{
				Name: "TestModule",
			},
			expectedAttrs:  0,
			shouldHaveJSON: false,
		},
		{
			name: "nil inputs and outputs",
			moduleInfo: &OpenInferenceModuleInfo{
				Name:    "TestModule",
				Inputs:  nil,
				Outputs: nil,
			},
			expectedAttrs:  0,
			shouldHaveJSON: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attrs := buildInputOutputAttributes(tt.moduleInfo, logger)

			assert.GreaterOrEqual(t, len(attrs), tt.expectedAttrs, "Should have at least expected attributes")

			if tt.shouldHaveJSON {
				// Verify JSON attributes exist.
				hasInputValue := false
				hasOutputValue := false
				hasInputMimeType := false
				hasOutputMimeType := false

				for _, attr := range attrs {
					key := string(attr.Key)
					if key == "input.value" {
						hasInputValue = true
						// Verify it's valid JSON.
						var val interface{}
						err := json.Unmarshal([]byte(attr.Value.AsString()), &val)
						assert.NoError(t, err, "input.value should be valid JSON")
					}
					if key == "output.value" {
						hasOutputValue = true
						// Verify it's valid JSON.
						var val interface{}
						err := json.Unmarshal([]byte(attr.Value.AsString()), &val)
						assert.NoError(t, err, "output.value should be valid JSON")
					}
					if key == "input.mime_type" {
						hasInputMimeType = true
						assert.Equal(t, "application/json", attr.Value.AsString())
					}
					if key == "output.mime_type" {
						hasOutputMimeType = true
						assert.Equal(t, "application/json", attr.Value.AsString())
					}
				}

				if len(tt.moduleInfo.Inputs) > 0 {
					assert.True(t, hasInputValue, "Should have input.value when inputs exist")
					assert.True(t, hasInputMimeType, "Should have input.mime_type when inputs exist")
				}
				if len(tt.moduleInfo.Outputs) > 0 {
					assert.True(t, hasOutputValue, "Should have output.value when outputs exist")
					assert.True(t, hasOutputMimeType, "Should have output.mime_type when outputs exist")
				}
			}
		})
	}
}

// The XML interceptor has already parsed XML into structured fields at the top level.
func TestBuildInputOutputAttributes_WithXML(t *testing.T) {
	var logger kitlog.Logger

	moduleInfo := &OpenInferenceModuleInfo{
		Name: "TestModule",
		Outputs: map[string]interface{}{
			"__raw_response": "<response><translation>Hello</translation><score>9.5</score></response>",
			"translation":    "Hello", // XML interceptor already parsed these fields.
			"score":          "9.5",
			"other":          "other_value",
		},
	}

	attrs := buildInputOutputAttributes(moduleInfo, logger)

	// Find output.value attribute.
	var outputValueAttr *attribute.KeyValue
	for i := range attrs {
		if string(attrs[i].Key) == "output.value" {
			outputValueAttr = &attrs[i]
			break
		}
	}

	require.NotNil(t, outputValueAttr, "Should have output.value attribute")

	// Parse the JSON.
	var outputData map[string]interface{}
	err := json.Unmarshal([]byte(outputValueAttr.Value.AsString()), &outputData)
	require.NoError(t, err, "output.value should be valid JSON")

	// Verify structured fields are preserved (XML interceptor already parsed them).
	assert.Equal(t, "Hello", outputData["translation"], "XML field 'translation' should be present")
	assert.Equal(t, "9.5", outputData["score"], "XML field 'score' should be present")
	assert.Equal(t, "other_value", outputData["other"], "Other fields should be preserved")
	// Verify __raw_response was removed.
	_, hasRawResponse := outputData["__raw_response"]
	assert.False(t, hasRawResponse, "__raw_response should be removed after cleanup")
}

func TestBuildInputOutputAttributes_KeepsRawOnError(t *testing.T) {
	var logger kitlog.Logger
	raw := `<response><broken`

	moduleInfo := &OpenInferenceModuleInfo{
		Name: "TestModule",
		Outputs: map[string]interface{}{
			"__raw_response": raw,
		},
		Error: fmt.Errorf("XML parsing failed: expected element name after <"),
	}

	attrs := buildInputOutputAttributes(moduleInfo, logger)

	var outputValueAttr *attribute.KeyValue
	for i := range attrs {
		if string(attrs[i].Key) == "output.value" {
			outputValueAttr = &attrs[i]
			break
		}
	}
	require.NotNil(t, outputValueAttr, "error spans should still have output.value")

	var outputData map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(outputValueAttr.Value.AsString()), &outputData))
	assert.Equal(t, raw, outputData["__raw_response"])
}

func TestBuildInputOutputAttributes_KeepsRawWhenOnlyRawPresent(t *testing.T) {
	var logger kitlog.Logger
	raw := "unparseable model text"

	moduleInfo := &OpenInferenceModuleInfo{
		Name: "TestModule",
		Outputs: map[string]interface{}{
			"__raw_response": raw,
		},
	}

	attrs := buildInputOutputAttributes(moduleInfo, logger)

	var outputValueAttr *attribute.KeyValue
	for i := range attrs {
		if string(attrs[i].Key) == "output.value" {
			outputValueAttr = &attrs[i]
			break
		}
	}
	require.NotNil(t, outputValueAttr)

	var outputData map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(outputValueAttr.Value.AsString()), &outputData))
	assert.Equal(t, raw, outputData["__raw_response"])
}

// This is critical because wrong attributes break cost tracking in Arize.
func TestBuildLLMAttributes(t *testing.T) {
	var logger kitlog.Logger

	tests := []struct {
		name          string
		moduleInfo    *OpenInferenceModuleInfo
		expectedAttrs int // Minimum expected attributes.
		checkModel    bool
		checkProvider bool
		checkTokens   bool
	}{
		{
			name: "complete LLM info",
			moduleInfo: &OpenInferenceModuleInfo{
				Name:             "TestModule",
				Model:            "gpt-4",
				Provider:         "openai",
				PromptTokens:     100,
				CompletionTokens: 50,
				TotalTokens:      150,
				Prompt:           "Test prompt",
				Response:         "Test response",
			},
			expectedAttrs: 8, // model_name, provider, 3 token counts, prompt, response, request.type.
			checkModel:    true,
			checkProvider: true,
			checkTokens:   true,
		},
		{
			name: "missing model name",
			moduleInfo: &OpenInferenceModuleInfo{
				Name:     "TestModule",
				Model:    "",
				Provider: "openai",
			},
			expectedAttrs: 2, // model_name (unknown), provider, request.type.
			checkModel:    true,
			checkProvider: true,
		},
		{
			name: "missing provider",
			moduleInfo: &OpenInferenceModuleInfo{
				Name:     "TestModule",
				Model:    "gpt-4",
				Provider: "",
			},
			expectedAttrs: 2, // model_name, provider (unknown), request.type.
			checkModel:    true,
			checkProvider: true,
		},
		{
			name: "only token counts",
			moduleInfo: &OpenInferenceModuleInfo{
				Name:             "TestModule",
				Model:            "gpt-4",
				Provider:         "openai",
				PromptTokens:     100,
				CompletionTokens: 50,
				TotalTokens:      150,
			},
			expectedAttrs: 6, // model_name, provider, 3 token counts, request.type.
			checkModel:    true,
			checkProvider: true,
			checkTokens:   true,
		},
		{
			name: "zero token counts (should not include)",
			moduleInfo: &OpenInferenceModuleInfo{
				Name:             "TestModule",
				Model:            "gpt-4",
				Provider:         "openai",
				PromptTokens:     0,
				CompletionTokens: 0,
				TotalTokens:      0,
			},
			expectedAttrs: 3, // model_name, provider, request.type (no token counts).
			checkModel:    true,
			checkProvider: true,
		},
		{
			name: "only prompt tokens",
			moduleInfo: &OpenInferenceModuleInfo{
				Name:         "TestModule",
				Model:        "gpt-4",
				Provider:     "openai",
				PromptTokens: 100,
			},
			expectedAttrs: 4, // model_name, provider, prompt tokens, request.type.
			checkModel:    true,
			checkProvider: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attrs := buildLLMAttributes(tt.moduleInfo, logger)

			assert.GreaterOrEqual(t, len(attrs), tt.expectedAttrs, "Should have at least expected attributes")

			// Verify required attributes.
			hasModelName := false
			hasProvider := false
			hasRequestType := false
			hasPromptTokens := false
			hasCompletionTokens := false
			hasTotalTokens := false

			for _, attr := range attrs {
				key := string(attr.Key)
				switch key {
				case "llm.model_name":
					hasModelName = true
					if tt.checkModel {
						if tt.moduleInfo.Model != "" {
							assert.Equal(t, tt.moduleInfo.Model, attr.Value.AsString())
						} else {
							assert.Equal(t, defaultModuleName, attr.Value.AsString())
						}
					}
				case "llm.provider":
					hasProvider = true
					if tt.checkProvider {
						if tt.moduleInfo.Provider != "" {
							assert.Equal(t, tt.moduleInfo.Provider, attr.Value.AsString())
						} else {
							assert.Equal(t, defaultModuleName, attr.Value.AsString())
						}
					}
				case "llm.request.type":
					hasRequestType = true
					assert.Equal(t, "completion", attr.Value.AsString())
				case "llm.token_count.prompt":
					hasPromptTokens = true
					if tt.checkTokens {
						assert.Equal(t, int64(tt.moduleInfo.PromptTokens), attr.Value.AsInt64())
					}
				case "llm.token_count.completion":
					hasCompletionTokens = true
					if tt.checkTokens {
						assert.Equal(t, int64(tt.moduleInfo.CompletionTokens), attr.Value.AsInt64())
					}
				case "llm.token_count.total":
					hasTotalTokens = true
					if tt.checkTokens {
						assert.Equal(t, int64(tt.moduleInfo.TotalTokens), attr.Value.AsInt64())
					}
				}
			}

			assert.True(t, hasModelName, "Should always have llm.model_name")
			assert.True(t, hasProvider, "Should always have llm.provider")
			assert.True(t, hasRequestType, "Should always have llm.request.type")

			if tt.checkTokens {
				if tt.moduleInfo.PromptTokens > 0 {
					assert.True(t, hasPromptTokens, "Should have prompt tokens when > 0")
				}
				if tt.moduleInfo.CompletionTokens > 0 {
					assert.True(t, hasCompletionTokens, "Should have completion tokens when > 0")
				}
				if tt.moduleInfo.TotalTokens > 0 {
					assert.True(t, hasTotalTokens, "Should have total tokens when > 0")
				}
			}
		})
	}
}

// TestBuildLLMAttributes_WithNilLogger tests that nil logger doesn't cause panics.
func TestBuildLLMAttributes_WithNilLogger(t *testing.T) {
	moduleInfo := &OpenInferenceModuleInfo{
		Name:     "TestModule",
		Model:    "",
		Provider: "",
	}

	// Should not panic with nil logger.
	attrs := buildLLMAttributes(moduleInfo, nil)

	// Should still return attributes.
	assert.Greater(t, len(attrs), 0, "Should return attributes even with nil logger")
}

// TestAddOpenInferenceAttributes_NilSpan tests critical error handling.
func TestAddOpenInferenceAttributes_NilSpan(t *testing.T) {
	var logger kitlog.Logger
	moduleInfo := &OpenInferenceModuleInfo{
		Name: "TestModule",
	}

	err := addOpenInferenceAttributes(nil, moduleInfo, logger, nil, nil)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "nil")
}

// TestAddOpenInferenceAttributes_Integration tests the full attribute building pipeline.
func TestAddOpenInferenceAttributes_Integration(t *testing.T) {
	var logger kitlog.Logger
	tracer := noop.NewTracerProvider().Tracer("test")
	ctx := context.Background()
	_, span := tracer.Start(ctx, "test-span")

	moduleInfo := &OpenInferenceModuleInfo{
		Name:             "TestModule",
		Type:             "generator",
		Inputs:           map[string]interface{}{"input": "value"},
		Outputs:          map[string]interface{}{"output": "value"},
		Model:            "gpt-4",
		Provider:         "openai",
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
	}

	info := &core.ModuleInfo{
		Signature: core.Signature{
			Instruction: "You are a helpful assistant.",
		},
		Version: "1.0.0",
	}

	err := addOpenInferenceAttributes(span, moduleInfo, logger, info, map[string]any{"input": "value"})

	assert.NoError(t, err, "Should not error with valid span")
}
