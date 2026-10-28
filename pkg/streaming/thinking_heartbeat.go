package streaming

import (
	"sync"
	"time"
)

const (
	// ThinkingHeartbeatContent is the NDJSON info content for request-in-flight liveness.
	ThinkingHeartbeatContent = "thinking"
	// ThinkingHeartbeatInterval is how often to emit while an HTTP stream call is open.
	ThinkingHeartbeatInterval = 5 * time.Second
	thinkingHeartbeatMaxTicks = 120
)

// ThinkingHeartbeat emits periodic info events until stopped.
type ThinkingHeartbeat struct {
	stopOnce sync.Once
	done     chan struct{}
}

// StartThinkingHeartbeat ticks at interval until Stop or max ticks. No-op when eventChan is nil.
func StartThinkingHeartbeat(eventChan EventChannel, actor Actor, interval time.Duration) *ThinkingHeartbeat {
	hb := &ThinkingHeartbeat{done: make(chan struct{})}
	if eventChan == nil || interval <= 0 {
		return hb
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		ticks := 0
		for {
			select {
			case <-hb.done:
				return
			case <-ticker.C:
				ticks++
				if ticks > thinkingHeartbeatMaxTicks {
					return
				}
				ev := InferenceEvent{
					Type:       EventTypeInfo,
					Actor:      actor,
					ModuleName: actor.Label(),
					Content:    ThinkingHeartbeatContent,
					Timestamp:  time.Now().UTC(),
				}
				if !trySendHeartbeat(eventChan, hb.done, ev) {
					return
				}
			}
		}
	}()
	return hb
}

// trySendHeartbeat delivers ev without blocking. A closed eventChan panics on
// send even with default, so recover and treat that as stop. StreamHandler
// construction starts the ticker immediately. DSPy may return without invoking
// the handler callback, and the caller may close eventChan while this ticker
// still runs.
func trySendHeartbeat(eventChan EventChannel, done <-chan struct{}, ev InferenceEvent) (ok bool) {
	ok = true
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	select {
	case <-done:
		return false
	case eventChan <- ev:
		return true
	default:
		return true
	}
}

// Stop ends the heartbeat goroutine.
func (hb *ThinkingHeartbeat) Stop() {
	if hb == nil {
		return
	}
	hb.stopOnce.Do(func() { close(hb.done) })
}
