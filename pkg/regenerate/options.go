package regenerate

import "context"

// RegenerateOptions is the shared contract for "run again with optional feedback" across pipelines.
// Use it at service entry points (e.g. AnalyzeChapters, AnalyzeSpeakers) so the refinement loop
// can respect Force and use Message as the first iteration's previous feedback.
type RegenerateOptions struct {
	Force    bool   // Allow re-run over cap or when already analyzed.
	Message  string // Optional short disagree comment for the first iteration; empty means use last evaluation feedback or none.
	Context  string // Optional full edited artifact (proposal) used as the regenerate seed; empty means use last stored output.
	Research bool   // Prefer research-backed feedback analysis when the app supports it.
}

type optionsKey struct{}
type researchModeKey struct{}

// WithOptions returns a context that carries opts.
// A nil ctx is returned unchanged so callers do not receive a new root context.
func WithOptions(ctx context.Context, opts RegenerateOptions) context.Context {
	if ctx == nil {
		return nil
	}
	return context.WithValue(ctx, optionsKey{}, opts)
}

// FromContext returns the RegenerateOptions from ctx, or the zero value when unset.
func FromContext(ctx context.Context) RegenerateOptions {
	if ctx == nil {
		return RegenerateOptions{}
	}
	v, ok := ctx.Value(optionsKey{}).(RegenerateOptions)
	if !ok {
		return RegenerateOptions{}
	}
	return v
}

// WithResearchMode marks ctx so FeedbackNormalizer implementations can prefer research analysis.
// A nil ctx is returned unchanged.
func WithResearchMode(ctx context.Context, research bool) context.Context {
	if ctx == nil {
		return nil
	}
	return context.WithValue(ctx, researchModeKey{}, research)
}

// ResearchModeFromContext reports whether WithResearchMode was set true.
func ResearchModeFromContext(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v, ok := ctx.Value(researchModeKey{}).(bool)
	if !ok {
		return false
	}
	return v
}
