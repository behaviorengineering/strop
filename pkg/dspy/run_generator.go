package dspy

import (
	"context"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
)

// RunModule runs optional beforeProcess then Process on a DSPy module (DirectivesCoT or Predict).
// beforeProcess may be nil.
func RunModule(
	ctx context.Context,
	module core.Module,
	inputs map[string]interface{},
	beforeProcess func(core.Module) error,
	opts ...core.Option,
) (map[string]interface{}, error) {
	if beforeProcess != nil {
		if err := beforeProcess(module); err != nil {
			return nil, err
		}
	}
	return module.Process(ctx, inputs, opts...)
}
