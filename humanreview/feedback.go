package humanreview

import (
	"context"
	"strings"

	"github.com/behaviorengineering/strop/regenerate"
)

// FeedbackNormalizer turns a human reject comment into the generator regen message.
// Implementations must not change Gate status. Nil is treated as PassthroughNormalizer.
type FeedbackNormalizer interface {
	Normalize(ctx context.Context, comment string) (string, error)
}

// PassthroughNormalizer trims the comment and returns it unchanged.
type PassthroughNormalizer struct{}

// Normalize implements FeedbackNormalizer.
func (PassthroughNormalizer) Normalize(_ context.Context, comment string) (string, error) {
	return strings.TrimSpace(comment), nil
}

// RegenOptionsFromComment normalizes comment and returns Force regen options.
// A nil normalizer uses PassthroughNormalizer. Context stays empty; use RegenOptionsFromReview
// when a Cursor-edited artifact should seed the next generator run.
func RegenOptionsFromComment(ctx context.Context, n FeedbackNormalizer, comment string) (regenerate.RegenerateOptions, error) {
	return RegenOptionsFromReview(ctx, n, comment, "")
}

// RegenOptionsFromReview normalizes comment and returns Force regen options with an optional artifact Context.
// Message is the short disagree comment. Context is the full edited proposal; empty means no seed override.
func RegenOptionsFromReview(ctx context.Context, n FeedbackNormalizer, comment, artifactContext string) (regenerate.RegenerateOptions, error) {
	if n == nil {
		n = PassthroughNormalizer{}
	}
	msg, err := n.Normalize(ctx, comment)
	if err != nil {
		return regenerate.RegenerateOptions{}, err
	}
	return regenerate.RegenerateOptions{
		Force:   true,
		Message: msg,
		Context: strings.TrimSpace(artifactContext),
	}, nil
}
