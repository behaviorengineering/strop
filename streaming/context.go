package streaming

import (
	"context"
	"fmt"
	"time"

	"github.com/behaviorengineering/strop/runreport"
)

// eventChannelCtxKey carries an optional EventChannel for TUI warnings during module execution
// (e.g. structured output parse failures before retry). See ContextWithEventChannel.
type eventChannelCtxKey struct{}

// ContextWithEventChannel attaches ch to ctx so interceptors can emit non-blocking UI warnings.
// If ch is nil, returns ctx unchanged.
func ContextWithEventChannel(ctx context.Context, ch EventChannel) context.Context {
	if ch == nil {
		return ctx
	}
	return context.WithValue(ctx, eventChannelCtxKey{}, ch)
}

// EventChannelFromContext returns the channel set by ContextWithEventChannel, or nil.
func EventChannelFromContext(ctx context.Context) EventChannel {
	if ctx == nil {
		return nil
	}
	v := ctx.Value(eventChannelCtxKey{})
	if v == nil {
		return nil
	}
	ch, _ := v.(EventChannel)
	return ch
}

const maxWarningContentRunes = 320

// EmitWarning sends a non-blocking warning to the TUI when an event channel is on ctx.
// Drops the event if the channel buffer is full (same policy as StreamHandler).
func EmitWarning(ctx context.Context, moduleName, content string) {
	if c := runreport.CollectorFromContext(ctx); c != nil && content != "" {
		c.RecordWarning(moduleName, truncateRunes(content, maxWarningContentRunes))
	}
	ch := EventChannelFromContext(ctx)
	if ch == nil || content == "" {
		return
	}
	content = truncateRunes(content, maxWarningContentRunes)
	select {
	case ch <- InferenceEvent{
		Type:       EventTypeWarning,
		ModuleName: moduleName,
		Content:    content,
		Timestamp:  time.Now(),
	}:
	default:
	}
}

// EmitInfo sends a non-blocking info event to the TUI when an event channel is on ctx.
// Drops the event if the channel buffer is full (same policy as StreamHandler).
func EmitInfo(ctx context.Context, moduleName, content string) {
	ch := EventChannelFromContext(ctx)
	if ch == nil || content == "" {
		return
	}
	select {
	case ch <- InferenceEvent{
		Type:       EventTypeInfo,
		ModuleName: moduleName,
		Content:    content,
		Timestamp:  time.Now(),
	}:
	default:
	}
}

// EmitWarningf formats and sends a warning via EmitWarning.
func EmitWarningf(ctx context.Context, moduleName, format string, args ...interface{}) {
	EmitWarning(ctx, moduleName, fmt.Sprintf(format, args...))
}

func truncateRunes(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	var n int
	for i := range s {
		if n >= maxRunes {
			return s[:i] + "…"
		}
		n++
	}
	return s
}
