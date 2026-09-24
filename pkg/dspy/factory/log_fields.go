package factory

// Structured log field keys shared across factory wrappers and setup.
const (
	logFieldModel            = "model"
	logFieldModule           = "module"
	logFieldInterceptorCount = "interceptor_count"
	logFieldMaxOutputTokens  = "max_output_tokens"
	logFieldExpectedModelID  = "expected_model_id"
)

// Provider API schema names used when creating LLMs.
const (
	apiSchemaOpenAI = "openai"
	apiSchemaGoogle = "google"
)
