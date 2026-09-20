package concurrency

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestClassifyError(t *testing.T) {
	t.Parallel()
	if ClassifyError(nil) != OutcomeOK {
		t.Fatal("nil should be OK")
	}
	if ClassifyError(context.DeadlineExceeded) != OutcomeTrip {
		t.Fatal("deadline should trip")
	}
	if ClassifyError(errors.New("HTTP 429 too many requests")) != OutcomeTrip {
		t.Fatal("429 should trip")
	}
	if ClassifyError(errors.New("invalid mentions json")) != OutcomeSlow {
		t.Fatal("generic error should not trip")
	}
}

func TestLimiterFixedMode(t *testing.T) {
	t.Parallel()
	lim := NewLimiter(Config{Fixed: 3, Min: 1, Max: 8})
	if lim.Limit() != 3 {
		t.Fatalf("limit=%d", lim.Limit())
	}
	before := lim.Limit()
	lim.Record(time.Second, errors.New("429 rate limit"))
	if lim.Limit() != before {
		t.Fatalf("fixed mode changed limit to %d", lim.Limit())
	}
}

func TestLimiterRampAndTrip(t *testing.T) {
	t.Parallel()
	cfg := Config{
		Min:           1,
		Max:           8,
		SlowThreshold: time.Minute,
		SuccessBurst:  2,
		Cooldown:      time.Millisecond,
	}
	lim := NewLimiter(cfg)
	if lim.Limit() != 2 {
		t.Fatalf("initial limit=%d want 2", lim.Limit())
	}
	lim.Record(100*time.Millisecond, nil)
	lim.Record(100*time.Millisecond, nil)
	if lim.Limit() != 3 {
		t.Fatalf("after ramp limit=%d want 3", lim.Limit())
	}
	lim.Record(time.Second, errors.New("503 unavailable"))
	if lim.Limit() != 1 {
		t.Fatalf("after trip limit=%d want 1", lim.Limit())
	}
	time.Sleep(2 * time.Millisecond)
	lim.Record(100*time.Millisecond, nil)
	lim.Record(100*time.Millisecond, nil)
	if lim.Limit() != 2 {
		t.Fatalf("after cooldown ramp limit=%d want 2", lim.Limit())
	}
}

func TestLimiterSlowResponse(t *testing.T) {
	t.Parallel()
	lim := NewLimiter(Config{Min: 1, Max: 8, SlowThreshold: 10 * time.Millisecond, SuccessBurst: 1})
	start := lim.Limit()
	lim.Record(50*time.Millisecond, nil)
	if lim.Limit() >= start {
		t.Fatalf("slow should decrease limit: %d -> %d", start, lim.Limit())
	}
}

func TestRunPoolConcurrent(t *testing.T) {
	t.Parallel()
	const n = 12
	var peak atomic.Int32
	var cur atomic.Int32
	items := make([]int, n)
	for i := range items {
		items[i] = i
	}
	err := RunPool(context.Background(), items, Config{Fixed: 4, Min: 1, Max: 4}, func(ctx context.Context, _ int) error {
		v := cur.Add(1)
		for {
			old := peak.Load()
			if v <= old || peak.CompareAndSwap(old, v) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		cur.Add(-1)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if peak.Load() > 4 {
		t.Fatalf("peak in-flight=%d want <=4", peak.Load())
	}
}

func TestRunPoolFailFast(t *testing.T) {
	t.Parallel()
	boom := errors.New("503 unavailable")
	var started atomic.Int32
	items := make([]int, 8)
	err := RunPool(context.Background(), items, Config{Fixed: 2}, func(ctx context.Context, i int) error {
		started.Add(1)
		if i == 0 {
			return boom
		}
		time.Sleep(100 * time.Millisecond)
		return nil
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != boom.Error() {
		t.Fatalf("err=%v", err)
	}
}

func TestAcquireRespectsContext(t *testing.T) {
	t.Parallel()
	lim := NewLimiter(Config{Fixed: 1})
	ctx, cancel := context.WithCancel(context.Background())
	if err := lim.Acquire(ctx); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := lim.Acquire(ctx)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Acquire err=%v", err)
		}
	}()
	time.Sleep(10 * time.Millisecond)
	cancel()
	wg.Wait()
	lim.Release()
}
