package factory

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/behaviorengineering/strop/pkg/streaming"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
	"github.com/XiaoConstantine/dspy-go/pkg/interceptors"
)

const identicalInferenceErrorCode = "identical_inference_error"

var (
	uuidRE        = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	reqIDRE       = regexp.MustCompile(`req-[a-z0-9]+`)
	hexRunRE      = regexp.MustCompile(`[0-9a-f]{8,}`)
	msTimeoutRE   = regexp.MustCompile(`\d{3,}ms`)
	volatileMsgRE = regexp.MustCompile(`(?i)\b(timeout|temporar|too many requests|retry-after)\b`)
)

// IdenticalInferenceError is returned when consecutive inner failures share the same fingerprint.
type IdenticalInferenceError struct {
	Op               string
	Message          string
	ErrorFingerprint string
	Cause            error
}

func (e *IdenticalInferenceError) Error() string {
	if e == nil {
		return identicalInferenceErrorCode
	}
	if e.Message != "" {
		return e.Message
	}
	return identicalInferenceErrorCode
}

func (e *IdenticalInferenceError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// Code returns the stable error code for this failure class.
func (e *IdenticalInferenceError) Code() string {
	return identicalInferenceErrorCode
}

// NewIdenticalErrorRetryInterceptor retries module execution and stops after two identical non-volatile errors.
func NewIdenticalErrorRetryInterceptor(config interceptors.RetryConfig) core.ModuleInterceptor {
	if config.MaxAttempts < 1 {
		panic("identical error retry: MaxAttempts must be >= 1")
	}
	return func(ctx context.Context, inputs map[string]any, info *core.ModuleInfo, handler core.ModuleHandler, opts ...core.Option) (map[string]any, error) {
		var lastErr error
		delay := config.Delay
		var consecutiveIdentical int
		var lastFingerprint string

		moduleName := ""
		if info != nil {
			moduleName = strings.TrimSpace(info.ModuleName)
		}

		for attempt := 1; attempt <= config.MaxAttempts; attempt++ {
			eventChan := streaming.EventChannelFromContext(ctx)
			streaming.SendCallProgress(ctx, eventChan, moduleName, attempt, config.MaxAttempts)

			result, err := handler(ctx, inputs, opts...)
			if err == nil {
				return result, nil
			}
			lastErr = err

			if isVolatileInferenceError(err) {
				consecutiveIdentical = 0
				lastFingerprint = ""
			} else {
				fp := inferenceErrorFingerprint(err)
				if fp == "" {
					consecutiveIdentical = 0
					lastFingerprint = ""
				} else if fp == lastFingerprint {
					consecutiveIdentical++
				} else {
					consecutiveIdentical = 1
					lastFingerprint = fp
				}
				if consecutiveIdentical >= 2 {
					emitNeedsOperator(ctx, eventChan, moduleName, fp, attempt, config.MaxAttempts)
					return nil, &IdenticalInferenceError{
						Op:               "IdenticalErrorRetryInterceptor",
						Message:          fmt.Sprintf("module %s stopped after %d identical inference errors", moduleName, consecutiveIdentical),
						ErrorFingerprint: fp,
						Cause:            err,
					}
				}
			}

			if attempt == config.MaxAttempts {
				break
			}

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}

			delay = time.Duration(float64(delay) * config.Backoff)
			if config.MaxBackoff > 0 && delay > config.MaxBackoff {
				delay = config.MaxBackoff
			}
		}

		name := moduleName
		if info != nil && name == "" {
			name = info.ModuleName
		}
		return nil, fmt.Errorf("module %s failed after %d attempts: %w", name, config.MaxAttempts, lastErr)
	}
}

func emitNeedsOperator(ctx context.Context, eventChan streaming.EventChannel, moduleName, fingerprint string, attempt, maxAttempts int) {
	if eventChan == nil {
		return
	}
	ev := streaming.InferenceEvent{
		Type:             streaming.EventTypeWarning,
		ModuleName:       moduleName,
		Timestamp:        time.Now(),
		NeedsOperator:    true,
		ErrorFingerprint: fingerprint,
		CallAttempt:      attempt,
		CallMaxAttempts:  maxAttempts,
	}
	select {
	case eventChan <- ev:
	case <-ctx.Done():
	}
}

func isVolatileInferenceError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	msg := strings.ToLower(err.Error())
	if volatileMsgRE.MatchString(msg) {
		return true
	}
	if strings.Contains(msg, "429") || strings.Contains(msg, "502") || strings.Contains(msg, "503") {
		return true
	}
	return false
}

func inferenceErrorFingerprint(err error) string {
	if err == nil {
		return ""
	}
	s := strings.ToLower(err.Error())
	s = uuidRE.ReplaceAllString(s, "")
	s = reqIDRE.ReplaceAllString(s, "")
	s = hexRunRE.ReplaceAllString(s, "")
	s = msTimeoutRE.ReplaceAllString(s, "")
	s = strings.TrimSpace(s)
	return s
}
