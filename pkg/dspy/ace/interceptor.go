package ace

import (
	"context"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
)

// CatchInterceptor records Process errors onto the trajectory recorder on ctx.
// It never calls EndTrajectory. Attach outside RetryModuleInterceptor so transient
// failures that later succeed are not treated as pear-shaped.
// No-op when no recorder is on ctx.
func CatchInterceptor() core.ModuleInterceptor {
	return func(ctx context.Context, inputs map[string]any, info *core.ModuleInfo, handler core.ModuleHandler, opts ...core.Option) (map[string]any, error) {
		outputs, err := handler(ctx, inputs, opts...)
		if err == nil {
			return outputs, nil
		}
		rec := RecorderFromContext(ctx)
		if rec == nil {
			return outputs, err
		}
		rec.RecordStep("module_error", moduleName(info), err.Error(), nil, nil, err)
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
