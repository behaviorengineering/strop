package streaming

import (
	"time"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
)

// StreamHandler creates a DSPy StreamHandler that sends events as the given actor.
func StreamHandler(actor Actor, eventChan EventChannel) core.StreamHandler {
	heartbeat := StartThinkingHeartbeat(eventChan, actor, ThinkingHeartbeatInterval)
	return func(chunk core.StreamChunk) error {
		if eventChan == nil {
			heartbeat.Stop()
			return nil
		}
		event := InferenceEvent{
			Actor:      actor,
			ModuleName: actor.Label(),
			Timestamp:  time.Now(),
			TokenUsage: chunk.Usage,
		}

		switch {
		case chunk.Error != nil:
			heartbeat.Stop()
			event.Type = EventTypeError
			event.Error = chunk.Error
		case chunk.Done:
			heartbeat.Stop()
			event.Type = EventTypeDone
		default:
			heartbeat.Stop()
			event.Type = EventTypeChunk
			event.Content = chunk.Content
		}

		select {
		case eventChan <- event:
			return nil
		default:
			return nil
		}
	}
}
