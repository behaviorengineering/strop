package orchestration

import (
	"context"
	"errors"
	"strings"
)

// ClassifyFailure maps an error to a FailureClass for trajectory and retry policy.
func ClassifyFailure(err error) FailureClass {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.Canceled) {
		return FailureCancelled
	}
	msg := strings.ToLower(err.Error())
	if errors.Is(err, context.DeadlineExceeded) ||
		strings.Contains(msg, "deadline exceeded") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "429") ||
		strings.Contains(msg, "too many requests") ||
		strings.Contains(msg, "rate limit") ||
		strings.Contains(msg, "503") ||
		strings.Contains(msg, "502") ||
		strings.Contains(msg, "unavailable") ||
		strings.Contains(msg, "circuit") ||
		strings.Contains(msg, "breaker") {
		return FailureTransientInfra
	}
	if strings.Contains(msg, "validation") ||
		strings.Contains(msg, "gate") ||
		(strings.Contains(msg, "failed after") &&
			(strings.Contains(msg, "attempt") || strings.Contains(msg, "score"))) {
		return FailureSemanticGate
	}
	return FailureHard
}

// FailureClassForGateResult classifies a successful generate with a failed gate (no error).
func FailureClassForGateResult(passed bool) FailureClass {
	if passed {
		return ""
	}
	return FailureSemanticGate
}
