package orchestration

import (
	"context"
	"errors"
	"strings"
	"time"
)

// DefaultBreakerBackoff is the wait used when ClassifyTransientStepError sees a 503 / breaker signal.
const DefaultBreakerBackoff = 30 * time.Second

// DefaultTransientBackoff is the short wait for 502 / timeout style failures.
const DefaultTransientBackoff = time.Second

// ClassifyTransientStepError is the default StepErrorClassifier.
//
// Semantics:
//   - context.Canceled: not retryable
//   - context.DeadlineExceeded / messages with deadline or timeout: retryable, short backoff
//   - 503 / circuit / breaker: retryable, DefaultBreakerBackoff (wait; do not hammer)
//   - 502 / unavailable / rate limit: retryable, short backoff
//   - other errors (including validation): retryable with no backoff so MaxAttempts can still heal flakes
func ClassifyTransientStepError(err error) StepErrorClass {
	if err == nil {
		return StepErrorClass{}
	}
	if errors.Is(err, context.Canceled) {
		return StepErrorClass{Retryable: false}
	}
	msg := strings.ToLower(err.Error())
	if errors.Is(err, context.DeadlineExceeded) ||
		strings.Contains(msg, "deadline exceeded") ||
		strings.Contains(msg, "timeout") {
		return StepErrorClass{Retryable: true, Backoff: DefaultTransientBackoff}
	}
	if strings.Contains(msg, "503") ||
		strings.Contains(msg, "circuit") ||
		strings.Contains(msg, "breaker") {
		return StepErrorClass{Retryable: true, Backoff: DefaultBreakerBackoff}
	}
	if strings.Contains(msg, "502") ||
		strings.Contains(msg, "unavailable") ||
		strings.Contains(msg, "rate limit") ||
		strings.Contains(msg, "too many requests") {
		return StepErrorClass{Retryable: true, Backoff: DefaultTransientBackoff}
	}
	// Validation and unknown errors: allow remaining attempts without sleep.
	return StepErrorClass{Retryable: true}
}
