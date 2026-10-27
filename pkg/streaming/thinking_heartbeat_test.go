package streaming

import (
	"testing"
	"time"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStartThinkingHeartbeat_emitsWhileBlocked(t *testing.T) {
	t.Parallel()
	ch := make(EventChannel, 4)

	hb := StartThinkingHeartbeat(ch, GeneratorActor("G"), 10*time.Millisecond)
	defer hb.Stop()

	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		select {
		case ev := <-ch:
			if ev.Type == EventTypeInfo && ev.Content == ThinkingHeartbeatContent {
				return
			}
		default:
			time.Sleep(2 * time.Millisecond)
		}
	}
	t.Fatal("expected thinking heartbeat")
}

func TestStartThinkingHeartbeat_stopOnFirstChunk(t *testing.T) {
	t.Parallel()
	ch := make(EventChannel, 8)
	actor := GeneratorActor("G")
	hb := StartThinkingHeartbeat(ch, actor, 5*time.Millisecond)
	handler := StreamHandler(actor, ch)

	require.NoError(t, handler(core.StreamChunk{Content: "tok"}))
	hb.Stop()
	time.Sleep(30 * time.Millisecond)
	for len(ch) > 0 {
		ev := <-ch
		assert.NotEqual(t, ThinkingHeartbeatContent, ev.Content)
	}
}

func TestStartThinkingHeartbeat_stopOnDoneWithoutChunks(t *testing.T) {
	t.Parallel()
	ch := make(EventChannel, 8)
	actor := GeneratorActor("G")
	hb := StartThinkingHeartbeat(ch, actor, 5*time.Millisecond)
	handler := StreamHandler(actor, ch)

	require.NoError(t, handler(core.StreamChunk{Done: true}))
	hb.Stop()
	time.Sleep(30 * time.Millisecond)
	for len(ch) > 0 {
		ev := <-ch
		assert.NotEqual(t, ThinkingHeartbeatContent, ev.Content)
	}
}

func TestStartThinkingHeartbeat_tickCap(t *testing.T) {
	t.Parallel()
	ch := make(EventChannel, 256)
	hb := StartThinkingHeartbeat(ch, GeneratorActor("G"), time.Millisecond)
	time.Sleep(150 * time.Millisecond)
	hb.Stop()
	count := 0
	for len(ch) > 0 {
		ev := <-ch
		if ev.Content == ThinkingHeartbeatContent {
			count++
		}
	}
	assert.LessOrEqual(t, count, thinkingHeartbeatMaxTicks)
}

func TestStartThinkingHeartbeat_nilChannelNoOp(t *testing.T) {
	t.Parallel()
	hb := StartThinkingHeartbeat(nil, GeneratorActor("G"), time.Millisecond)
	hb.Stop()
}

func TestStartThinkingHeartbeat_closedChannelDoesNotPanic(t *testing.T) {
	t.Parallel()
	ch := make(EventChannel, 1)
	hb := StartThinkingHeartbeat(ch, GeneratorActor("G"), time.Millisecond)
	time.Sleep(8 * time.Millisecond)
	close(ch)
	time.Sleep(20 * time.Millisecond)
	hb.Stop()
}
