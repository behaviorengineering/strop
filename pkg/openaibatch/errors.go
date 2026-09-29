package openaibatch

import (
	"errors"
	"fmt"
)

// HTTPError is a non-2xx response from the batch HTTP surface.
type HTTPError struct {
	Op         string
	StatusCode int
	Body       string
}

func (e *HTTPError) Error() string {
	if e == nil {
		return "openaibatch: http error"
	}
	msg := fmt.Sprintf("http %d", e.StatusCode)
	if e.Body != "" {
		msg += ": " + e.Body
	}
	if e.Op != "" {
		return fmt.Sprintf("openaibatch: %s: %s", e.Op, msg)
	}
	return fmt.Sprintf("openaibatch: %s", msg)
}

var errOverCap = errors.New("openaibatch: line exceeds max JSONL size")

// OverCapError means a single request line cannot fit under MaxJSONLBytes.
type OverCapError struct {
	CustomID string
	Bytes    int
	MaxBytes int
}

func (e *OverCapError) Error() string {
	if e == nil {
		return errOverCap.Error()
	}
	return fmt.Sprintf("openaibatch: custom_id %q encoded size %d exceeds max %d", e.CustomID, e.Bytes, e.MaxBytes)
}

func (e *OverCapError) Is(target error) bool {
	return target == errOverCap
}

func wrap(op string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("openaibatch: %s: %w", op, err)
}

func httpErr(op string, status int, body string) error {
	return wrap(op, &HTTPError{Op: op, StatusCode: status, Body: body})
}

// IsNotFound reports whether err is an HTTP 404 from this package.
func IsNotFound(err error) bool {
	return hasStatus(err, 404)
}

// IsUnauthorized reports whether err is an HTTP 401 from this package.
func IsUnauthorized(err error) bool {
	return hasStatus(err, 401)
}

// IsThrottle reports whether err is an HTTP 429 or 503 from this package.
func IsThrottle(err error) bool {
	return hasStatus(err, 429) || hasStatus(err, 503)
}

// IsOverCap reports whether err is a single-line JSONL cap violation.
func IsOverCap(err error) bool {
	if errors.Is(err, errOverCap) {
		return true
	}
	var oc *OverCapError
	return errors.As(err, &oc)
}

func hasStatus(err error, code int) bool {
	var he *HTTPError
	if errors.As(err, &he) {
		return he.StatusCode == code
	}
	return false
}
