package dspy

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	dspylogging "github.com/XiaoConstantine/dspy-go/pkg/logging"
)

// AttachModuleTrace starts a dspy-go TraceSession JSONL under dir and puts it on ctx.
// Pair with factory InterceptorSetup (TracingInterceptor): without a session the interceptor is a no-op.
// Dir should be a durable work-story path (sibling of RLM TraceDir), not a disposable analysis temp.
// The returned close flushes and closes the session; call it when the host job finishes.
func AttachModuleTrace(ctx context.Context, dir string, metadata map[string]any) (context.Context, func() error, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return ctx, func() error { return nil }, fmt.Errorf("AttachModuleTrace: dir is required")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ctx, nil, fmt.Errorf("AttachModuleTrace: mkdir %s: %w", dir, err)
	}
	stamp := time.Now().UTC().Format("20060102-150405")
	path := filepath.Join(dir, fmt.Sprintf("module_%s.jsonl", stamp))
	session, err := dspylogging.StartTraceSession(ctx, path, metadata)
	if err != nil {
		return ctx, nil, fmt.Errorf("AttachModuleTrace: start session: %w", err)
	}
	ctx = dspylogging.WithTraceSession(ctx, session)
	return ctx, session.Close, nil
}
