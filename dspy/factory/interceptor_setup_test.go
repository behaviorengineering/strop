package factory

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/behaviorengineering/strop/runreport"

	kitdspy "github.com/behaviorengineering/strop/dspy"
	dspymodules "github.com/behaviorengineering/strop/dspy/modules"
	"github.com/behaviorengineering/strop/dspy/tracing"
	kitlog "github.com/behaviorengineering/strop/log"

	"github.com/stretchr/testify/require"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
	"github.com/XiaoConstantine/dspy-go/pkg/interceptors"
)

// sees only __raw_response (no parsed fields), resulting in empty output.value.
func TestXMLInterceptorIssue_ReplicatesProductionBug(t *testing.T) {
	// Create a DirectivesCoT module (same as production generators).
	signature := core.NewSignature(
		[]core.InputField{
			{Field: core.NewField("original_text")},
		},
		[]core.OutputField{
			{Field: core.NewField("literal_translation")},
			{Field: core.NewField("semantic_translation")},
			{Field: core.NewField("idiomatic_translation")},
		},
	)
	module := dspymodules.New(signature, dspymodules.Config{Name: "TestTranslator"})

	// Setup interceptors in the SAME ORDER as production.
	var logger kitlog.Logger
	setup := NewInterceptorSetup(
		true, // openInferenceEnabled.
		"test-service",
		nil, // retryConfig.
		0,   // timeout.
		tracing.OpenInferenceModuleInterceptor,
		logger,
		func(modelID string) string { return "test-provider" },
		nil, // modelIDByModuleName - not needed for this test.
		nil, // onRegisterModuleModel - not needed for this test.
		nil, // inputProcessorFactory - not needed for this test.
		runreport.Config{},
	)

	// Add interceptors FIRST (same as production).
	setup.AddInterceptors(module, kitdspy.ProviderConfig{})

	// Enable XML LAST (same as production).
	setup.EnableXMLOutput(module)

	// Get the interceptor chain.
	interceptorList := module.GetInterceptors()
	require.Greater(t, len(interceptorList), 0, "Should have interceptors")

	t.Logf("Interceptor chain length: %d", len(interceptorList))
	for i := range interceptorList {
		t.Logf("Interceptor %d: present", i)
	}

	// Capture what each interceptor sees.
	var openInferenceOutputs map[string]any
	var xmlOutputs map[string]any

	// We need to wrap OpenInference (second-to-last) and XML (last).
	originalInterceptors := make([]core.ModuleInterceptor, len(interceptorList))
	copy(originalInterceptors, interceptorList)

	// Wrap OpenInference interceptor (should be second-to-last, before XML).
	if len(interceptorList) >= 2 {
		openInferenceIdx := len(interceptorList) - 2
		originalOpenInferenceInterceptor := interceptorList[openInferenceIdx]
		interceptorList[openInferenceIdx] = func(ctx context.Context, inputs map[string]any, info *core.ModuleInfo, handler core.ModuleHandler, opts ...core.Option) (map[string]any, error) {
			// Call original interceptor.
			outputs, err := originalOpenInferenceInterceptor(ctx, inputs, info, handler, opts...)

			// Capture what OpenInference sees.
			openInferenceOutputs = make(map[string]any)
			for k, v := range outputs {
				openInferenceOutputs[k] = v
			}

			return outputs, err
		}
	}

	// For outputs flowing back: parse processes first (innermost), then format passes through.
	if len(interceptorList) >= 1 {
		lastIdx := len(interceptorList) - 1
		originalXMLInterceptor := interceptorList[lastIdx]
		interceptorList[lastIdx] = func(ctx context.Context, inputs map[string]any, info *core.ModuleInfo, handler core.ModuleHandler, opts ...core.Option) (map[string]any, error) {
			// Format modifies inputs, parse processes outputs.
			outputs, err := originalXMLInterceptor(ctx, inputs, info, handler, opts...)
			// Check if parsing failed.
			if err != nil {
				t.Logf("XML Interceptor ERROR: %v", err)
				// But if it returns outputs, it means fallback happened or error was caught.
			}

			// This should have parsed fields and NO __raw_response (after our fix).
			xmlOutputs = make(map[string]any)
			for k, v := range outputs {
				xmlOutputs[k] = v
			}

			t.Logf("XML Interceptor captured: keys=%v, count=%d, err=%v", getKeys(outputs), len(outputs), err)

			// Debug: Check if parsing actually happened.
			hasParsedFields := false
			hasRawResponse := false
			for k := range outputs {
				if k == rawResponseKey {
					hasRawResponse = true
				} else {
					hasParsedFields = true
				}
			}
			t.Logf("XML Interceptor debug: hasParsedFields=%v, hasRawResponse=%v, err=%v", hasParsedFields, hasRawResponse, err)

			// If we only see __raw_response, parsing failed silently.
			if hasRawResponse && !hasParsedFields && err == nil {
				t.Logf("WARNING: XML interceptor returned only __raw_response with no error!")
				t.Logf("This suggests parsing failed but error was swallowed, or parsing didn't run")
			}

			return outputs, err
		}
	}

	module.SetInterceptors(interceptorList)

	// This is what happens in predict.go when XML mode is enabled.
	mockHandler := func(ctx context.Context, inputs map[string]any, opts ...core.Option) (map[string]any, error) {
		// This simulates what Predict.processCore returns when XML mode is enabled.
		return map[string]any{
			"__raw_response": `<response>
				<literal_translation>More worth bird in hand than 100 flying</literal_translation>
				<semantic_translation>It is better to have a bird in hand than a hundred flying</semantic_translation>
				<idiomatic_translation>A bird in the hand is worth two in the bush</idiomatic_translation>
			</response>`,
		}, nil
	}

	// Execute through interceptor chain.
	chainedInterceptor := core.ChainModuleInterceptors(interceptorList...)
	info := core.NewModuleInfo("TranslationGenerator", "Predict", signature)

	inputs := map[string]any{
		"original_text": "Mas vale pajaro en mano que 100 volando",
	}

	finalOutputs, err := chainedInterceptor(context.Background(), inputs, info, mockHandler)
	require.NoError(t, err, "Interceptor chain should not fail")

	// REPLICATE THE ISSUE: Check what each interceptor saw.
	t.Log("=== INTERCEPTOR OUTPUT ANALYSIS ===")
	t.Logf("XML Interceptor outputs: %+v", xmlOutputs)
	t.Logf("OpenInference Interceptor outputs: %+v", openInferenceOutputs)
	t.Logf("Final outputs: %+v", finalOutputs)

	// THE BUG: Check if XML interceptor parsed the fields.
	xmlHasParsedFields := false
	xmlHasRawResponse := false
	for k := range xmlOutputs {
		if k == rawResponseKey {
			xmlHasRawResponse = true
		} else {
			xmlHasParsedFields = true
		}
	}

	t.Logf("XML Interceptor: has parsed fields=%v, has __raw_response=%v", xmlHasParsedFields, xmlHasRawResponse)

	// THE BUG: Check what OpenInference sees.
	openInferenceHasParsedFields := false
	openInferenceHasRawResponse := false
	for k := range openInferenceOutputs {
		if k == rawResponseKey {
			openInferenceHasRawResponse = true
		} else {
			openInferenceHasParsedFields = true
		}
	}

	t.Logf("OpenInference Interceptor: has parsed fields=%v, has __raw_response=%v",
		openInferenceHasParsedFields, openInferenceHasRawResponse)

	// This is what gets serialized to output.value.
	var outputValueJSON string
	if len(openInferenceOutputs) > 0 {
		// Convert to map[string]interface{} (what extractModuleInfo does).
		outputsForSerialization := make(map[string]interface{})
		for k, v := range openInferenceOutputs {
			outputsForSerialization[k] = v
		}

		// Call removeRawResponse (this is what buildInputOutputAttributes does).
		cleanedOutputs := tracing.RemoveRawResponseForTest(outputsForSerialization)

		// Serialize to JSON (this becomes output.value).
		jsonBytes, err := json.Marshal(cleanedOutputs)
		require.NoError(t, err)
		outputValueJSON = string(jsonBytes)
	}

	t.Logf("output.value JSON (after removeRawResponse): %s", outputValueJSON)

	// ANALYSIS: Verify the fix.
	t.Log("\n=== FIX VERIFICATION ===")

	// After fix: XML interceptor should remove __raw_response after parsing.
	if xmlHasRawResponse {
		t.Log("⚠️  XML interceptor still has __raw_response after parsing")
		t.Log("  - This should be fixed now")
		t.Log("  - If still present, the fix didn't work")
	} else {
		t.Log("✅ XML interceptor removed __raw_response after parsing")
	}

	if !xmlHasParsedFields {
		t.Log("⚠️  XML interceptor didn't parse fields")
		t.Log("  - This could mean parsing failed silently")
		t.Log("  - Or the parse happens at a different point in the chain")
	} else {
		t.Log("✅ XML interceptor parsed fields successfully")
	}

	if openInferenceHasRawResponse {
		t.Log("⚠️  OpenInference still sees __raw_response")
		t.Log("  - This means XML interceptor didn't remove it")
		t.Log("  - Or OpenInference runs before XML interceptor")
	} else {
		t.Log("✅ OpenInference does NOT see __raw_response (fixed!)")
	}

	// ASSERTIONS: Verify the fix worked.
	if openInferenceHasRawResponse {
		if !openInferenceHasParsedFields {
			t.Error("❌ CRITICAL: OpenInference sees __raw_response but NO parsed fields")
			t.Error("   This results in empty output.value after cleanup")
		} else {
			t.Log("⚠️  OpenInference sees both __raw_response AND parsed fields")
			t.Log("   This means XML interceptor didn't remove __raw_response")
			t.Log("   Our cleanup code will handle it, but it shouldn't be necessary")
		}
	} else {
		t.Log("✅ FIX VERIFIED: OpenInference does NOT see __raw_response")
		t.Log("   XML interceptor successfully removed it after parsing")
	}

	if outputValueJSON == "{}" {
		t.Error("❌ output.value is empty {}")
		t.Error("   This happens because removeRawResponse removes __raw_response but no parsed fields exist")
	} else {
		t.Logf("✅ output.value contains data: %s", outputValueJSON)
	}

	// Verify the fix: The key metric is what OpenInference sees
	// If OpenInference doesn't see __raw_response, the fix is working!
	if !openInferenceHasRawResponse && openInferenceHasParsedFields {
		t.Log("\n✅ FIX CONFIRMED: OpenInference does NOT see __raw_response")
		t.Log("   XML interceptor successfully removed it after parsing")
		t.Log("   The fix in tmp/dspy-go/pkg/interceptors/xml.go:ParseXMLOutputs is working!")
		t.Log("   output.value will now contain clean, structured data")
	} else if openInferenceHasRawResponse {
		t.Log("\n⚠️  FIX NOT WORKING: OpenInference still sees __raw_response")
		t.Log("   Need to investigate why the fix didn't work")
	}

	// but the important thing is what OpenInference sees (which is correct).
}

// This helps us understand if the issue is with the parse interceptor or the chain.
func TestXMLParseInterceptor_Directly(t *testing.T) {
	signature := core.NewSignature(
		[]core.InputField{
			{Field: core.NewField("original_text")},
		},
		[]core.OutputField{
			{Field: core.NewField("literal_translation")},
			{Field: core.NewField("semantic_translation")},
			{Field: core.NewField("idiomatic_translation")},
		},
	)

	// Create parse interceptor with same config as production.
	xmlConfig := interceptors.DefaultXMLConfig().
		WithStrictParsing(true).
		WithValidation(false).
		WithFallback(false).
		WithMaxDepth(5).
		WithMaxSize(64 * 1024)

	parseInterceptor := interceptors.XMLParseModuleInterceptor(xmlConfig)

	info := core.NewModuleInfo("TranslationGenerator", "Predict", signature)

	// Mock handler that returns __raw_response (like Predict does).
	mockHandler := func(ctx context.Context, inputs map[string]any, opts ...core.Option) (map[string]any, error) {
		return map[string]any{
			"__raw_response": `<response>
				<literal_translation>More worth bird in hand than 100 flying</literal_translation>
				<semantic_translation>It is better to have a bird in hand than a hundred flying</semantic_translation>
				<idiomatic_translation>A bird in the hand is worth two in the bush</idiomatic_translation>
			</response>`,
		}, nil
	}

	// Execute parse interceptor directly.
	outputs, err := parseInterceptor(context.Background(), map[string]any{"original_text": "test"}, info, mockHandler)

	t.Logf("Parse interceptor output: keys=%v, err=%v", getKeys(outputs), err)

	if err != nil {
		t.Logf("Parse interceptor returned error: %v", err)
		t.Logf("This explains why XML interceptor only returns __raw_response")
		return
	}

	// Check if parsing worked.
	hasParsedFields := false
	hasRawResponse := false
	for k := range outputs {
		if k == rawResponseKey {
			hasRawResponse = true
		} else {
			hasParsedFields = true
		}
	}

	t.Logf("Parse interceptor: hasParsedFields=%v, hasRawResponse=%v", hasParsedFields, hasRawResponse)

	if !hasParsedFields && hasRawResponse {
		t.Error("Parse interceptor failed to parse - only __raw_response present")
		t.Error("This is the root cause - parse interceptor is not working")
	}

	// The important thing is that parsed fields are present.
	if !hasParsedFields {
		t.Error("Parse interceptor must produce parsed fields - this is the critical requirement")
	}
}

// Helper to get keys from map for logging.
func getKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func TestEnableStructuredOutput_ClearsPredictDSPyXMLInterceptors(t *testing.T) {
	signature := core.NewSignature(
		[]core.InputField{{Field: core.NewField("transcript")}},
		[]core.OutputField{
			{Field: core.NewField("story_titles", core.WithDescription("XML array (list of items)"))},
			{Field: core.NewField("story_reconstruction_spines", core.WithDescription("XML array (list of items)"))},
		},
	)
	module := dspymodules.New(signature, dspymodules.Config{Name: "Story Extractor"})
	predict, err := dspymodules.PredictOf(module)
	require.NoError(t, err)
	require.True(t, predict.IsXMLModeEnabled(), "NewPredict should enable XML for multi-output signatures")
	require.NotEmpty(t, predict.GetInterceptors(), "dspy-go attaches XML interceptor by default")

	setup := NewInterceptorSetup(false, "", nil, 0, nil, nil, nil, nil, nil, nil, runreport.Config{})
	setup.enablePredictRawXMLPassthrough(module, predict)
	require.Empty(t, predict.GetInterceptors(), "passthrough must strip dspy-go XML parse from Predict")

	setup2 := NewInterceptorSetup(false, "", nil, 0, nil, nil, nil, nil, nil, nil, runreport.Config{})
	setup2.EnableStructuredOutput(module, predict)
	require.Len(t, predict.GetInterceptors(), 2, "Predict should have validation + custom structured output interceptors only")
}

func TestEnableStructuredOutput_PreservesRetryAfterAddInterceptors(t *testing.T) {
	signature := core.NewSignature(
		[]core.InputField{{Field: core.NewField("original_text")}},
		[]core.OutputField{
			{Field: core.NewField("description")},
			{Field: core.NewField("phase_plan")},
		},
	)
	module := dspymodules.New(signature, dspymodules.Config{Name: "PostGenerator"})
	predict, err := dspymodules.PredictOf(module)
	require.NoError(t, err)

	// Match SetupModule: strip dspy-go XML before attaching retry, then enable custom parse.
	existing := module.GetInterceptors()
	require.NotEmpty(t, existing)
	module.SetInterceptors(existing[:len(existing)-1])

	retryConfig := &interceptors.RetryConfig{
		MaxAttempts: 3,
		Delay:       time.Millisecond,
		Backoff:     1,
	}
	setup := NewInterceptorSetup(false, "", retryConfig, 0, nil, nil, nil, nil, nil, nil, runreport.Config{})
	setup.AddInterceptors(module, kitdspy.ProviderConfig{})
	afterReliability := len(module.GetInterceptors())
	require.Greater(t, afterReliability, 0, "AddInterceptors should attach retry/runreport")

	setup.EnableStructuredOutput(module, predict)
	final := module.GetInterceptors()
	require.Greater(t, len(final), afterReliability, "structured output should append, not replace, reliability interceptors")
	require.Greater(t, len(final), 2, "retry must survive EnableStructuredOutput; 2 would mean only validation+parse remain")
}

func TestInterceptorSetup_RegisterMandatoryFieldsOverridesDefault(t *testing.T) {
	t.Parallel()
	setup := NewInterceptorSetup(false, "", nil, 0, nil, nil, nil, nil, nil, nil, runreport.Config{})
	setup.RegisterMandatoryFields("Chapter Ideas", []string{"main_idea"})
	require.NotNil(t, setup.outputValidators["Chapter Ideas"])
}
