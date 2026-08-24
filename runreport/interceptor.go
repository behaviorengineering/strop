package runreport

import (
	"context"
	"time"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
)

// ModuleInterceptor records module invocations on the active run-report collector.
func ModuleInterceptor() core.ModuleInterceptor {
	return func(ctx context.Context, inputs map[string]any, info *core.ModuleInfo, handler core.ModuleHandler, opts ...core.Option) (map[string]any, error) {
		if !Enabled(ctx) || CollectorFromContext(ctx) == nil {
			return handler(ctx, inputs, opts...)
		}
		if !ConfigFromContext(ctx).RecordModuleCalls {
			return handler(ctx, inputs, opts...)
		}
		name := moduleName(info)
		start := time.Now()
		outputs, err := handler(ctx, inputs, opts...)
		ok := err == nil
		CollectorFromContext(ctx).RecordModule(name, ok, err, time.Since(start))
		return outputs, err
	}
}

func moduleName(info *core.ModuleInfo) string {
	if info == nil {
		return "module"
	}
	if info.ModuleName != "" {
		return info.ModuleName
	}
	return info.ModuleType
}
