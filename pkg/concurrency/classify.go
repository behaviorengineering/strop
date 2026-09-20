package concurrency

import (
	"context"
	"errors"
	"net"
	"strings"
)

// Outcome drives adaptive limit adjustments after one pool item completes.
type Outcome int

const (
	// OutcomeOK means the call succeeded within the slow threshold.
	OutcomeOK Outcome = iota
	// OutcomeSlow means success but high latency; do not ramp up.
	OutcomeSlow
	// OutcomeTrip means inference pressure; halve in-flight limit.
	OutcomeTrip
)

// ClassifyError maps an error to Trip; nil is OK (latency checked separately).
func ClassifyError(err error) Outcome {
	if err == nil {
		return OutcomeOK
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return OutcomeTrip
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return OutcomeTrip
	}
	msg := strings.ToLower(err.Error())
	tripHints := []string{
		"429",
		"rate limit",
		"too many requests",
		"503",
		"502",
		"504",
		"timeout",
		"timed out",
		"unavailable",
		"circuit",
		"breaker",
	}
	for _, hint := range tripHints {
		if strings.Contains(msg, hint) {
			return OutcomeTrip
		}
	}
	return OutcomeSlow
}
