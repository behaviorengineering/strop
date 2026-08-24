package factory

import (
	"fmt"
	"time"

	stropdspy "github.com/behaviorengineering/strop/dspy"
	dspymodules "github.com/behaviorengineering/strop/dspy/modules"
	stropso "github.com/behaviorengineering/strop/dspy/structured_output"
	stropvalidation "github.com/behaviorengineering/strop/dspy/validation"
	stroplog "github.com/behaviorengineering/strop/log"
	"github.com/behaviorengineering/strop/runreport"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
	"github.com/XiaoConstantine/dspy-go/pkg/interceptors"
	"github.com/XiaoConstantine/dspy-go/pkg/modules"
)

const (
	// maxXMLResponseSize is the maximum XML response size (64KB) for security.
	maxXMLResponseSize = 64 * 1024
	// maxXMLDepth is the maximum XML nesting depth for security.
	maxXMLDepth = 15
	// rawResponseKey is the canonical key used in tests and some assertions; runtime may also emit
	// strop/dspy/rawresponse.AlternateKey ("_raw_response") — parsing accepts both.
	rawResponseKey = "__raw_response"
)

// InterceptorSetup configures interceptors on modules.
type InterceptorSetup struct {
	openInferenceEnabled     bool
	openInferenceServiceName string
	retryConfig              *interceptors.RetryConfig
	moduleTimeout            time.Duration // Module execution timeout.
	// OpenInference interceptor factory - injected to avoid circular dependency.
	createOpenInferenceInterceptor func(enabled bool, serviceName string, logger stroplog.Logger, providerLookup func(modelID string) string, modelIDByModuleName func(moduleName string) string) core.ModuleInterceptor
	logger                         stroplog.Logger
	providerLookup                 func(modelID string) string
	modelIDByModuleName            func(moduleName string) string
	onRegisterModuleModel          func(moduleName, modelID string)
	runReports                     runreport.Config
	// Input processor factory - allows pipeline-specific input processing (validation, mutation, transformation).
	inputProcessorFactory        func(provider stropdspy.ProviderConfig) stropvalidation.InputProcessor
	outputValidators             map[string]stropvalidation.OutputValidator
	formatInstructionsSupplement stropso.FormatInstructionsSupplement
	adjustParseSignature         stropso.ParseSignatureAdjuster
	afterParse                   stropso.AfterParseHook
}

// NewInterceptorSetup creates a new interceptor setup helper.
func NewInterceptorSetup(
	openInferenceEnabled bool,
	openInferenceServiceName string,
	retryConfig *interceptors.RetryConfig,
	moduleTimeout time.Duration,
	createOpenInferenceInterceptor func(enabled bool, serviceName string, logger stroplog.Logger, providerLookup func(modelID string) string, modelIDByModuleName func(moduleName string) string) core.ModuleInterceptor,
	logger stroplog.Logger,
	providerLookup func(modelID string) string,
	modelIDByModuleName func(moduleName string) string,
	onRegisterModuleModel func(moduleName, modelID string),
	inputProcessorFactory func(provider stropdspy.ProviderConfig) stropvalidation.InputProcessor,
	runReports runreport.Config,
) *InterceptorSetup {
	return &InterceptorSetup{
		openInferenceEnabled:           openInferenceEnabled,
		openInferenceServiceName:       openInferenceServiceName,
		retryConfig:                    retryConfig,
		moduleTimeout:                  moduleTimeout,
		createOpenInferenceInterceptor: createOpenInferenceInterceptor,
		logger:                         logger,
		providerLookup:                 providerLookup,
		modelIDByModuleName:            modelIDByModuleName,
		onRegisterModuleModel:          onRegisterModuleModel,
		runReports:                     runReports.Defaults(),
		inputProcessorFactory:          inputProcessorFactory,
		outputValidators:               make(map[string]stropvalidation.OutputValidator),
	}
}

// RegisterOutputValidator registers a per-display-name output validator used during structured output setup.
func (s *InterceptorSetup) RegisterOutputValidator(displayName string, validator stropvalidation.OutputValidator) {
	if s == nil || displayName == "" || validator == nil {
		return
	}
	if s.outputValidators == nil {
		s.outputValidators = make(map[string]stropvalidation.OutputValidator)
	}
	s.outputValidators[displayName] = validator
}

// RegisterMandatoryFields registers ValidateMandatoryFields for the module display name.
func (s *InterceptorSetup) RegisterMandatoryFields(displayName string, fields []string) {
	s.RegisterOutputValidator(displayName, stropvalidation.ValidateMandatoryFields(fields))
}

func (s *InterceptorSetup) stropLogger() stroplog.Logger {
	if s == nil {
		return nil
	}
	return s.logger
}

// SetStructuredOutputHooks registers optional parse/format hooks (product-specific).
func (s *InterceptorSetup) SetStructuredOutputHooks(
	supplement stropso.FormatInstructionsSupplement,
	adjust stropso.ParseSignatureAdjuster,
	after stropso.AfterParseHook,
) {
	if s == nil {
		return
	}
	s.formatInstructionsSupplement = supplement
	s.adjustParseSignature = adjust
	s.afterParse = after
}

// ComposeParseSignatureAdjuster chains next after any existing parse-signature adjuster.
func (s *InterceptorSetup) ComposeParseSignatureAdjuster(next stropso.ParseSignatureAdjuster) {
	if s == nil || next == nil {
		return
	}
	prev := s.adjustParseSignature
	s.adjustParseSignature = func(info *core.ModuleInfo, inputs map[string]any, signature core.Signature) core.Signature {
		if prev != nil {
			signature = prev(info, inputs, signature)
		}
		return next(info, inputs, signature)
	}
}

// EnableStructuredOutput enables structured output parsing on a Predict-backed module.
// This uses our custom parser implementation which supports nested XML and is format-agnostic.
// If dspy-go's XML is already enabled, it will be replaced with our custom implementation.
func (s *InterceptorSetup) EnableStructuredOutput(module core.InterceptableModule, predict *modules.Predict) {
	if module == nil || predict == nil {
		return
	}
	displayName := interceptableDisplayName(module)
	// DEBUG: Log function entry.
	s.debugLogName(displayName, "EnableStructuredOutput called", map[string]interface{}{
		"xml_already_enabled": predict.IsXMLModeEnabled(),
	})

	existingInterceptors := module.GetInterceptors()
	if predict.IsXMLModeEnabled() && len(existingInterceptors) > 0 {
		s.debugLogName(displayName, "XML mode enabled but interceptor should have been removed earlier", map[string]interface{}{
			"interceptor_count": len(existingInterceptors),
		})
	}

	config := stropso.DefaultConfig().
		WithStrictParsing(true).
		WithValidation(false).
		WithFallback(false).
		WithMaxDepth(maxXMLDepth).
		WithMaxSize(maxXMLResponseSize)
	if s.formatInstructionsSupplement != nil {
		config = config.WithFormatInstructionsSupplement(s.formatInstructionsSupplement)
	}
	if s.adjustParseSignature != nil {
		config = config.WithAdjustParseSignature(s.adjustParseSignature)
	}
	if s.afterParse != nil {
		config = config.WithAfterParse(s.afterParse)
	}

	if lg := s.stropLogger(); lg != nil {
		config = config.WithLogger(lg)
		s.debugLogName(displayName, "Logger added to structured output config", map[string]interface{}{
			"logger_available": true,
		})
	} else {
		s.debugLogName(displayName, "No logger available for structured output config", map[string]interface{}{
			"logger_available": false,
		})
	}

	parser, err := stropso.GetParser(stropso.FormatXML, config)
	if err != nil {
		s.debugLogName(displayName, "Failed to get parser, falling back to dspy-go XML", map[string]interface{}{
			"error": err,
		})
		xmlConfig := interceptors.DefaultXMLConfig().
			WithStrictParsing(true).
			WithValidation(false).
			WithFallback(false).
			WithMaxDepth(maxXMLDepth).
			WithMaxSize(maxXMLResponseSize)
		predict.WithXMLOutput(xmlConfig)
		return
	}

	structuredInterceptor := stropso.StructuredOutputInterceptor(parser, config)

	interceptorsBefore := module.GetInterceptors()
	s.debugLogName(displayName, "Enabling structured output on module", map[string]interface{}{
		"interceptors_before": len(interceptorsBefore),
		"format":              parser.FormatName(),
	})

	currentInterceptors := module.GetInterceptors()

	s.enablePredictRawXMLPassthrough(module, predict)
	currentInterceptors = module.GetInterceptors()

	s.addValidationInterceptor(module, &currentInterceptors)

	currentInterceptors = append(currentInterceptors, structuredInterceptor)
	module.SetInterceptors(currentInterceptors)

	interceptorsAfter := module.GetInterceptors()
	s.debugLogName(displayName, "Structured output enabled on module", map[string]interface{}{
		"interceptors_after": len(interceptorsAfter),
		"format":             parser.FormatName(),
	})
}

// EnableXMLOutput is deprecated - use EnableStructuredOutput instead.
// Kept for backward compatibility but delegates to EnableStructuredOutput.
func (s *InterceptorSetup) EnableXMLOutput(module core.Module) {
	predict, err := dspymodules.PredictOf(module)
	if err != nil {
		return
	}
	interceptable, err := dspymodules.AsInterceptable(module)
	if err != nil {
		return
	}
	s.EnableStructuredOutput(interceptable, predict)
}

// enablePredictRawXMLPassthrough turns on Predict XML raw-response passthrough without keeping
// dspy-go's XMLModuleInterceptor (custom StructuredOutputInterceptor handles parsing).
//
// Do not ClearInterceptors: DirectivesCoT stores reliability interceptors on Predict, so a
// full clear after AddInterceptors would wipe retry/timeout/OpenInference. Strip only the
// dspy-go XML interceptor (the one WithXMLOutput just appended, or the sole default interceptor).
func (s *InterceptorSetup) enablePredictRawXMLPassthrough(module core.InterceptableModule, predict *modules.Predict) {
	if module == nil || predict == nil {
		return
	}
	// NewPredict enables XML by default for multi-output signatures, which attaches dspy-go's
	// XMLModuleInterceptor on Predict. That parser does not understand our list/array fields
	// (e.g. story_reconstruction_spines) and must not run before the custom parser.
	alreadyEnabled := predict.IsXMLModeEnabled()
	if !alreadyEnabled {
		xmlConfig := interceptors.DefaultXMLConfig().
			WithStrictParsing(true).
			WithValidation(false).
			WithFallback(false).
			WithMaxDepth(maxXMLDepth).
			WithMaxSize(maxXMLResponseSize)
		predict.WithXMLOutput(xmlConfig)
	}
	stripDSPyXMLInterceptor(module, alreadyEnabled)
	s.debugLogName(interceptableDisplayName(module), "Enabled Predict raw XML passthrough for custom structured output parser", map[string]interface{}{
		"interceptor_count":         len(module.GetInterceptors()),
		"predict_interceptor_count": len(predict.GetInterceptors()),
	})
}

// stripDSPyXMLInterceptor removes dspy-go's XML parse interceptor and leaves all others.
// When XML was just enabled, WithXMLOutput appended that interceptor last. When XML was
// already on and the module still has only that default interceptor, strip it. When
// AddInterceptors already ran (count > 1), last is retry/runreport — do not strip.
func stripDSPyXMLInterceptor(module core.InterceptableModule, xmlAlreadyEnabled bool) {
	if module == nil {
		return
	}
	existing := module.GetInterceptors()
	if len(existing) == 0 {
		return
	}
	if !xmlAlreadyEnabled || len(existing) == 1 {
		module.SetInterceptors(existing[:len(existing)-1])
	}
}

// addValidationInterceptor adds a validation interceptor to ensure all output fields are present and non-empty.
func (s *InterceptorSetup) addValidationInterceptor(module core.InterceptableModule, interceptorsList *[]core.ModuleInterceptor) {
	moduleName := interceptableDisplayName(module)
	validator := stropvalidation.ValidateMandatoryFields(nil)
	if s != nil && s.outputValidators != nil {
		if registered, ok := s.outputValidators[moduleName]; ok && registered != nil {
			validator = registered
		}
	}
	s.appendValidationInterceptor(module, interceptorsList, validator)
}

func (s *InterceptorSetup) appendValidationInterceptor(
	module core.InterceptableModule,
	interceptorsList *[]core.ModuleInterceptor,
	validator stropvalidation.OutputValidator,
) {
	var validationLogger stropvalidation.Logger
	if lg := s.stropLogger(); lg != nil {
		validationLogger = lg
	}
	validationInterceptor := stropvalidation.ValidationInterceptor(validator, validationLogger)

	*interceptorsList = append(*interceptorsList, validationInterceptor)

	if s.logger != nil {
		s.logger.WithFields(map[string]interface{}{
			"module": interceptableDisplayName(module),
		}).Debug("Validation interceptor added to module")
	}
}

// debugLogName logs a debug message if logger is available.
func (s *InterceptorSetup) debugLogName(moduleName, message string, fields map[string]interface{}) {
	if s.logger == nil {
		return
	}
	allFields := map[string]interface{}{
		"module": moduleName,
	}
	for k, v := range fields {
		allFields[k] = v
	}
	s.logger.WithFields(allFields).Debug(message)
}

func (s *InterceptorSetup) reliabilityInterceptors(provider stropdspy.ProviderConfig) []core.ModuleInterceptor {
	if s == nil {
		return nil
	}
	attemptTimeout := provider.GetTimeout(s.moduleTimeout)
	var chain []core.ModuleInterceptor
	if s.moduleTimeout > 0 {
		chain = append(chain, interceptors.TimeoutModuleInterceptor(s.moduleTimeout))
	}
	if s.retryConfig != nil {
		chain = append(chain, interceptors.RetryModuleInterceptor(*s.retryConfig))
	}
	if attemptTimeout > 0 && attemptTimeout != s.moduleTimeout {
		chain = append(chain, interceptors.TimeoutModuleInterceptor(attemptTimeout))
	}
	return chain
}

func interceptableDisplayName(module core.InterceptableModule) string {
	if module == nil {
		return ""
	}
	if named, ok := module.(interface{ GetDisplayName() string }); ok {
		return named.GetDisplayName()
	}
	return fmt.Sprintf("%T", module)
}

// AddInterceptors adds timeout, retry, and OpenInference interceptors to a module.
// If inputProcessorFactory is provided, it will be called with the provider config to create
// an input processing interceptor (validation, mutation, transformation).
// When OpenInference is enabled, registers module display name -> model ID for cost-tracking fallback.
func (s *InterceptorSetup) AddInterceptors(module core.InterceptableModule, provider stropdspy.ProviderConfig) {
	// Register module -> model for cost-tracking when ExecutionState does not contain model ID.
	if s.onRegisterModuleModel != nil && provider.Model != "" {
		s.onRegisterModuleModel(module.GetDisplayName(), provider.Model)
	}

	// Debug: Log function entry.
	if s.logger != nil {
		s.logger.WithFields(map[string]interface{}{
			"openinference_enabled": s.openInferenceEnabled,
			"has_factory":           s.createOpenInferenceInterceptor != nil,
			"has_logger":            s.logger != nil,
			"has_input_processor":   s.inputProcessorFactory != nil,
		}).Debug("AddInterceptors called")
	}

	// Get existing interceptors once to avoid race conditions.
	existingInterceptors := module.GetInterceptors()

	if s.logger != nil {
		s.logger.WithFields(map[string]interface{}{
			"existing_count": len(existingInterceptors),
		}).Debug("Current interceptor count before adding")
	}

	// Add input processing interceptor first (outermost - processes inputs before anything else).
	// This should be the first interceptor added so it executes last (innermost in reverse order).
	// The processor can validate, mutate, or transform inputs based on module and provider config.
	if s.inputProcessorFactory != nil {
		processor := s.inputProcessorFactory(provider)
		var processingLogger stropvalidation.Logger
		if lg := s.stropLogger(); lg != nil {
			processingLogger = lg
		}
		inputProcessingInterceptor := stropvalidation.InputProcessingInterceptor(processor, processingLogger)
		existingInterceptors = append(existingInterceptors, inputProcessingInterceptor)

		if s.logger != nil {
			s.logger.WithFields(map[string]interface{}{
				"module":     module.GetDisplayName(),
				"model":      provider.Model,
				"max_tokens": provider.MaxContextTokens,
			}).Debug("Input processing interceptor added to module")
		}
	}

	// Reliability: overall cap, then retry, then per-attempt timeout (innermost of these).
	// First interceptor is outermost. A stall must die on the attempt clock so retry
	// still has budget on the overall clock (Cloudflare 408s were eating dspy.timeout).
	existingInterceptors = append(existingInterceptors, s.reliabilityInterceptors(provider)...)

	// Add OpenInference interceptor if enabled.
	if s.openInferenceEnabled && s.createOpenInferenceInterceptor != nil {
		if s.logger != nil {
			s.logger.WithFields(map[string]interface{}{
				"enabled":      s.openInferenceEnabled,
				"service_name": s.openInferenceServiceName,
			}).Debug("Adding OpenInference interceptor to module")
		}
		openInferenceInterceptor := s.createOpenInferenceInterceptor(
			s.openInferenceEnabled,
			s.openInferenceServiceName,
			s.logger,
			s.providerLookup,
			s.modelIDByModuleName,
		)
		existingInterceptors = append(existingInterceptors, openInferenceInterceptor)
	} else if s.logger != nil {
		s.logger.WithFields(map[string]interface{}{
			"enabled":                 s.openInferenceEnabled,
			"has_interceptor_factory": s.createOpenInferenceInterceptor != nil,
		}).Debug("OpenInference interceptor NOT added (disabled or factory missing)")
	}

	existingInterceptors = append(existingInterceptors, runreport.ModuleInterceptor())

	module.SetInterceptors(existingInterceptors)

	// Debug: Log final count.
	if s.logger != nil {
		finalInterceptors := module.GetInterceptors()
		s.logger.WithFields(map[string]interface{}{
			"final_count": len(finalInterceptors),
		}).Debug("AddInterceptors completed - final interceptor count")
	}
}
