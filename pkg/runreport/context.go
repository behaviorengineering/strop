package runreport

import (
	"context"
	"sync"
)

type configKey struct{}
type collectorKey struct{}

var (
	configMu      sync.RWMutex
	defaultConfig = Config{}.Defaults()
)

// SetDefaultConfig stores the process-wide run-report settings used when a
// context has no explicit override. Hosts should set it once at startup.
// Concurrent readers and writers are synchronized. Tests that change it should
// restore the previous config so later tests do not inherit it.
func SetDefaultConfig(cfg Config) {
	configMu.Lock()
	defaultConfig = cfg.Defaults()
	configMu.Unlock()
}

// WithConfig attaches run-report settings to ctx for middleware and sessions.
func WithConfig(ctx context.Context, cfg Config) context.Context {
	return context.WithValue(ctx, configKey{}, cfg.Defaults())
}

// ConfigFromContext returns run-report config from ctx, or the app default.
func ConfigFromContext(ctx context.Context) Config {
	if ctx != nil {
		if v, ok := ctx.Value(configKey{}).(Config); ok {
			return v.Defaults()
		}
	}
	configMu.RLock()
	defer configMu.RUnlock()
	return defaultConfig
}

// Enabled reports whether run reports are active for this context.
func Enabled(ctx context.Context) bool {
	return ConfigFromContext(ctx).Enabled
}

func withCollector(ctx context.Context, c *Collector) context.Context {
	return context.WithValue(ctx, collectorKey{}, c)
}

// CollectorFromContext returns the active collector, or nil when reporting is off.
func CollectorFromContext(ctx context.Context) *Collector {
	if ctx == nil {
		return nil
	}
	c, ok := ctx.Value(collectorKey{}).(*Collector)
	if !ok {
		return nil
	}
	return c
}

// Record is a no-op when reporting is disabled or no collector is on ctx.
func Record(ctx context.Context, kind StepKind, message string, details map[string]interface{}) {
	if c := CollectorFromContext(ctx); c != nil {
		c.Record(kind, message, details)
	}
}
