package factory

// Error message constants for consistent error reporting.
const (
	ErrProviderNil                = "provider cannot be nil"
	ErrInvalidProviderConfig      = "invalid provider configuration: %w"
	ErrLLMSetupFailed             = "failed to setup LLM for %s: %w"
	ErrModuleCreationFailed       = "failed to create %s module: %w"
	ErrConsolidatorPromptRequired = "consolidator prompt is required"
	ErrUnknownConsolidatorRole    = "unknown consolidator role"
	ErrUnknownEvaluationRole      = "unknown evaluation role: %s"
	ErrNoPromptForRole            = "no prompt found for role: %s"
	ErrModuleCreationForRole      = "failed to create module for %s: %w"
)
