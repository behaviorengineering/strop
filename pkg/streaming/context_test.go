package streaming

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestContextWithEventChannel_RoundTrip(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ch := make(EventChannel, 2)
	ctx = ContextWithEventChannel(ctx, ch)
	got := EventChannelFromContext(ctx)
	if got != ch {
		t.Fatalf("expected same channel pointer")
	}
	if EventChannelFromContext(context.Background()) != nil {
		t.Fatalf("expected nil without attachment")
	}
}

func TestEmitInfo_NonBlocking(t *testing.T) {
	t.Parallel()
	ch := make(EventChannel, 1)
	ctx := ContextWithEventChannel(context.Background(), ch)
	EmitInfo(ctx, "Content Strategist Evaluator", "scoring")
	select {
	case ev := <-ch:
		if ev.Type != EventTypeInfo {
			t.Fatalf("got type %q", ev.Type)
		}
		if ev.ModuleName != "Content Strategist Evaluator" || ev.Content != "scoring" {
			t.Fatalf("unexpected event: %+v", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for info event")
	}
}

func TestEmitWarning_NonBlocking(t *testing.T) {
	t.Parallel()
	ch := make(EventChannel, 1)
	ctx := ContextWithEventChannel(context.Background(), ch)
	EmitWarning(ctx, "TestModule", "something went wrong")
	select {
	case ev := <-ch:
		if ev.Type != EventTypeWarning {
			t.Fatalf("got type %q", ev.Type)
		}
		if ev.ModuleName != "TestModule" || ev.Content == "" {
			t.Fatalf("unexpected event: %+v", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for warning event")
	}
}

func TestTruncateRunes(t *testing.T) {
	t.Parallel()
	if got := truncateRunes("hello", 10); got != "hello" {
		t.Fatalf("got %q", got)
	}
	long := "abcdefghijklmnop"
	if got := truncateRunes(long, 5); !strings.HasSuffix(got, "…") || !strings.HasPrefix(got, "abcde") {
		t.Fatalf("expected first 5 runes plus ellipsis, got %q", got)
	}
}
