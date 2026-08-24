package factory

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	stropdspy "github.com/behaviorengineering/strop/dspy"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
	"github.com/XiaoConstantine/dspy-go/pkg/interceptors"
)

func TestReliabilityInterceptors_attemptTimeoutLeavesRetryBudget(t *testing.T) {
	setup := &InterceptorSetup{
		moduleTimeout: 400 * time.Millisecond,
		retryConfig: &interceptors.RetryConfig{
			MaxAttempts: 3,
			Delay:       time.Millisecond,
			Backoff:     1,
		},
	}
	provider := stropdspy.ProviderConfig{Timeout: "80ms"}
	chain := setup.reliabilityInterceptors(provider)
	if len(chain) != 3 {
		t.Fatalf("got %d interceptors, want overall + retry + attempt", len(chain))
	}

	var attempts atomic.Int32
	handler := func(ctx context.Context, _ map[string]any, _ ...core.Option) (map[string]any, error) {
		n := attempts.Add(1)
		if n == 1 {
			<-ctx.Done()
			return nil, ctx.Err()
		}
		return map[string]any{"ok": true}, nil
	}
	info := core.NewModuleInfo("stall_then_ok", "Predict", core.NewSignature(nil, nil))
	chained := core.ChainModuleInterceptors(chain...)
	start := time.Now()
	out, err := chained(context.Background(), map[string]any{}, info, handler)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("retry should succeed after attempt timeout: %v", err)
	}
	if out["ok"] != true {
		t.Fatalf("got %+v", out)
	}
	if attempts.Load() != 2 {
		t.Fatalf("attempts=%d want 2", attempts.Load())
	}
	if elapsed > 300*time.Millisecond {
		t.Fatalf("elapsed %s; overall cap should not have been burned by the stall", elapsed)
	}
}

func TestReliabilityInterceptors_skipsDuplicateAttemptTimeout(t *testing.T) {
	setup := &InterceptorSetup{
		moduleTimeout: 360 * time.Second,
		retryConfig: &interceptors.RetryConfig{
			MaxAttempts: 2,
			Delay:       time.Millisecond,
			Backoff:     1,
		},
	}
	chain := setup.reliabilityInterceptors(stropdspy.ProviderConfig{})
	if len(chain) != 2 {
		t.Fatalf("got %d interceptors, want overall + retry when provider timeout equals module timeout", len(chain))
	}
}
