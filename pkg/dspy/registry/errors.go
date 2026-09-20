package registry

// Error message constants for consistent error reporting.
const (
	ErrGeneratorNotFound         = "generator for task '%s' not found"
	ErrGeneratorNil              = "generator for task '%s' is nil"
	ErrEvaluatorsNotFound        = "evaluators for task '%s' not found"
	ErrEvaluatorsNil             = "evaluators for task '%s' is nil"
	ErrConsolidatorNotFound      = "consolidator for task '%s' not found"
	ErrConsolidatorNil           = "consolidator for task '%s' is nil"
	ErrWorkflowNotFound          = "workflow for task '%s' not found"
	ErrWorkflowNil               = "workflow for task '%s' is nil"
	ErrFeedbackAnalyzersNotFound = "feedback analyzers for task '%s' not found"
	ErrFeedbackAnalyzersNil      = "feedback analyzers for task '%s' is nil"
	ErrFeedbackFormatterNotFound = "feedback formatter for task '%s' not found"
	ErrFeedbackFormatterNil      = "feedback formatter for task '%s' is nil"
)
