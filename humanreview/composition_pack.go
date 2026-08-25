package humanreview

import (
	"context"
)

// LearningCandidate is one extractable learning artifact before persistence.
type LearningCandidate struct {
	Type    string
	Content map[string]interface{}
}

// CompositionLearningPack is a LearningPack that can extract composition
// candidates and supply accountability context. Product packs implement this
// in the app; strop must not import product types.
type CompositionLearningPack interface {
	LearningPack

	// ExtractAfterApproval builds learning candidates for an approved composition evaluation.
	// Empty slice is valid (nothing to learn).
	ExtractAfterApproval(ctx context.Context, eval *HumanEvaluation) ([]LearningCandidate, error)

	// AccountabilityContext returns chain + approved composition maps for demo trials.
	AccountabilityContext(
		ctx context.Context,
		eval *HumanEvaluation,
	) (chain map[string]interface{}, approved map[string]interface{}, err error)

	// PreviewCandidate returns a short display string for merge/review UX.
	// Empty string is allowed.
	PreviewCandidate(content map[string]interface{}) string

	// ValidateCandidate reports whether a candidate may proceed to merge/create.
	// Nil implementation should not happen; packs must implement it.
	ValidateCandidate(candidate LearningCandidate) bool
}

// AsCompositionPack returns pack as CompositionLearningPack when possible.
func AsCompositionPack(pack LearningPack) (CompositionLearningPack, bool) {
	if pack == nil {
		return nil, false
	}
	cp, ok := pack.(CompositionLearningPack)
	return cp, ok
}
