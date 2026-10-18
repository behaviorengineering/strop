package factory

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/behaviorengineering/strop/pkg/streaming"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
	"github.com/XiaoConstantine/dspy-go/pkg/interceptors"
)

func TestNewIdenticalErrorRetryInterceptor_panicsOnZeroMaxAttempts(t *testing.T) {
	t.Parallel()
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	NewIdenticalErrorRetryInterceptor(interceptors.RetryConfig{MaxAttempts: 0})
}

func TestIdenticalErrorRetry_stopsOnSecondIdenticalError(t *testing.T) {
	t.Parallel()
	ch := make(streaming.EventChannel, 8)
	ctx := streaming.ContextWithEventChannel(context.Background(), ch)

	cfg := interceptors.RetryConfig{
		MaxAttempts: 5,
		Delay:       time.Millisecond,
		Backoff:     1,
	}
	interceptor := NewIdenticalErrorRetryInterceptor(cfg)

	var attempts atomic.Int32
	streamErr := errors.New("openai: stream: malformed response: event contains a choice after the finish reason")
	handler := func(ctx context.Context, _ map[string]any, _ ...core.Option) (map[string]any, error) {
		attempts.Add(1)
		return nil, streamErr
	}
	info := core.NewModuleInfo("Essay Composition", "Predict", core.NewSignature(nil, nil))
	chained := core.ChainModuleInterceptors(interceptor)

	_, err := chained(ctx, map[string]any{}, info, handler)
	if err == nil {
		t.Fatal("expected error")
	}
	if attempts.Load() != 2 {
		t.Fatalf("attempts=%d want 2", attempts.Load())
	}
	var ident *IdenticalInferenceError
	if !errors.As(err, &ident) {
		t.Fatalf("expected IdenticalInferenceError, got %T: %v", err, err)
	}
	if !errors.Is(err, streamErr) {
		t.Fatalf("expected unwrap to stream error")
	}

	var sawNeedsOp bool
	for {
		select {
		case ev := <-ch:
			if ev.NeedsOperator {
				sawNeedsOp = true
			}
		default:
			goto done
		}
	}
done:
	if !sawNeedsOp {
		t.Fatal("expected NeedsOperator warning on channel")
	}
}

func TestIdenticalErrorRetry_volatileErrorsRetryToMax(t *testing.T) {
	t.Parallel()
	cfg := interceptors.RetryConfig{
		MaxAttempts: 3,
		Delay:       time.Millisecond,
		Backoff:     1,
	}
	interceptor := NewIdenticalErrorRetryInterceptor(cfg)

	var attempts atomic.Int32
	handler := func(ctx context.Context, _ map[string]any, _ ...core.Option) (map[string]any, error) {
		attempts.Add(1)
		return nil, context.DeadlineExceeded
	}
	info := core.NewModuleInfo("mod", "Predict", core.NewSignature(nil, nil))
	chained := core.ChainModuleInterceptors(interceptor)

	_, err := chained(context.Background(), map[string]any{}, info, handler)
	if err == nil {
		t.Fatal("expected error")
	}
	if attempts.Load() != 3 {
		t.Fatalf("attempts=%d want 3", attempts.Load())
	}
}
