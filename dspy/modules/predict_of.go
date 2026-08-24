package modules

import (
	"fmt"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
	dspymod "github.com/XiaoConstantine/dspy-go/pkg/modules"
)

// PredictOf returns the inner Predict for DirectivesCoT or bare Predict.
func PredictOf(module core.Module) (*dspymod.Predict, error) {
	if module == nil {
		return nil, fmt.Errorf("module is nil")
	}
	switch m := module.(type) {
	case *DirectivesCoT:
		if m.Predict == nil {
			return nil, fmt.Errorf("DirectivesCoT has nil Predict")
		}
		return m.Predict, nil
	case *dspymod.Predict:
		return m, nil
	default:
		return nil, fmt.Errorf("unsupported module type %T (want DirectivesCoT or Predict)", module)
	}
}

// AsInterceptable returns module as InterceptableModule when possible.
func AsInterceptable(module core.Module) (core.InterceptableModule, error) {
	if m, ok := module.(core.InterceptableModule); ok {
		return m, nil
	}
	return nil, fmt.Errorf("module %T is not InterceptableModule", module)
}
