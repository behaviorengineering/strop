package streaming

import (
	"context"
	"testing"
	"time"
)

func TestSendProgress_deliversWhenBufferFull(t *testing.T) {
	t.Parallel()
	ch := make(EventChannel, 1)
	ch <- InferenceEvent{Type: EventTypeChunk, Content: "fill"}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go SendProgress(ctx, ch, "Composition phase fork (attempt 1/3)", "fork", 1, 3)
	<-ch // drain filler so progress can enqueue
	got := <-ch
	if got.Content != "Composition phase fork (attempt 1/3)" {
		t.Fatalf("content %q", got.Content)
	}
	if got.PlanStep != "fork" || got.Attempt != 1 || got.MaxAttempts != 3 {
		t.Fatalf("meta step=%q attempt=%d max=%d", got.PlanStep, got.Attempt, got.MaxAttempts)
	}
}

func TestSendInfo_dropsWhenBufferFull(t *testing.T) {
	t.Parallel()
	ch := make(EventChannel, 1)
	ch <- InferenceEvent{Type: EventTypeChunk, Content: "fill"}

	SendInfo(ch, "should drop")

	if len(ch) != 1 {
		t.Fatalf("expected buffer length 1, got %d", len(ch))
	}
}
