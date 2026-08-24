package modules

import (
	"context"
	"fmt"
	"strings"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
	dspymod "github.com/XiaoConstantine/dspy-go/pkg/modules"
)

// Config configures a DirectivesCoT module (Predict + directives_ack).
type Config struct {
	Name string

	// RetainRationaleAsTaskField keeps a signature-declared rationale output as a task field
	// after directives_ack (e.g. post skim 12-line composition plan). Stock ChainOfThought
	// auto-rationale is never used.
	RetainRationaleAsTaskField bool
}

// DirectivesCoT is the n8n replacement for stock modules.ChainOfThought.
// It wraps Predict, prepends directives_ack, and does not auto-inject rationale.
type DirectivesCoT struct {
	Predict *dspymod.Predict
	name    string
}

var (
	_ core.Module              = (*DirectivesCoT)(nil)
	_ core.InterceptableModule = (*DirectivesCoT)(nil)
	_ core.Composable          = (*DirectivesCoT)(nil)
)

// NewEvaluator builds a DirectivesCoT that retains a declared rationale task field.
// Use for evaluators, consolidators, and feedback analyzers that previously relied on
// stock ChainOfThought auto-prepending rationale.
func NewEvaluator(sig core.Signature, name string) *DirectivesCoT {
	return New(sig, Config{
		Name:                       name,
		RetainRationaleAsTaskField: true,
	})
}

// New builds a DirectivesCoT from a task signature and config.
// Callers pass task instruction already rendered (persona + prompts); New wraps the protocol
// and ensures directives_ack is the first output field.
func New(sig core.Signature, cfg Config) *DirectivesCoT {
	taskRules := strings.TrimSpace(sig.Instruction)
	sig.Instruction = DirectivesProtocol + "\n\n" + taskRules
	sig.Outputs = ensureDirectivesAckFirst(sig.Outputs, cfg.RetainRationaleAsTaskField)

	name := strings.TrimSpace(cfg.Name)
	if name == "" {
		name = "DirectivesCoT"
	}
	predict := dspymod.NewPredict(sig).WithName(name)

	return &DirectivesCoT{
		Predict: predict,
		name:    name,
	}
}

func ensureDirectivesAckFirst(outputs []core.OutputField, retainRationale bool) []core.OutputField {
	var rationaleField *core.OutputField
	cleaned := make([]core.OutputField, 0, len(outputs)+1)
	for _, f := range outputs {
		name := strings.TrimSpace(f.Field.Name)
		if strings.EqualFold(name, DirectivesAckField) {
			continue
		}
		if strings.EqualFold(name, "rationale") {
			if retainRationale {
				cp := f
				rationaleField = &cp
			}
			continue
		}
		cleaned = append(cleaned, f)
	}

	ack := core.OutputField{
		Field: core.NewField(
			DirectivesAckField,
			core.WithDescription("Step-by-step: restate instructions, VOICE/MUST/ANTI_PATTERN objectives, plan, and attention check before any task output"),
		),
	}
	out := append([]core.OutputField{ack}, cleaned...)
	if retainRationale && rationaleField != nil {
		// Place structured plan immediately after directives_ack, before reader-facing fields.
		rest := append([]core.OutputField{*rationaleField}, out[1:]...)
		out = append([]core.OutputField{out[0]}, rest...)
	}
	return out
}

// WithName sets the display name on the inner Predict.
func (c *DirectivesCoT) WithName(name string) *DirectivesCoT {
	c.name = name
	c.Predict.WithName(name)
	return c
}

// GetDisplayName implements naming for spans / interceptors.
func (c *DirectivesCoT) GetDisplayName() string {
	if c.Predict != nil && c.Predict.DisplayName != "" {
		return c.Predict.DisplayName
	}
	if c.name != "" {
		return c.name
	}
	return "DirectivesCoT"
}

// GetModuleType returns DirectivesCoT (not Predict / ChainOfThought).
func (c *DirectivesCoT) GetModuleType() string {
	return "DirectivesCoT"
}

// GetSignature delegates to Predict.
func (c *DirectivesCoT) GetSignature() core.Signature {
	return c.Predict.GetSignature()
}

// SetSignature resets the signature and re-applies directives_ack (drops stock rationale unless already retained in outputs).
func (c *DirectivesCoT) SetSignature(signature core.Signature) {
	taskRules := strings.TrimSpace(signature.Instruction)
	if !strings.Contains(taskRules, "STRUCTURED ATTENTION") {
		taskRules = DirectivesProtocol + "\n\n" + taskRules
	}
	signature.Instruction = taskRules
	// Preserve rationale if the caller put it back on the signature.
	retain := false
	for _, f := range signature.Outputs {
		if strings.EqualFold(f.Field.Name, "rationale") {
			retain = true
			break
		}
	}
	signature.Outputs = ensureDirectivesAckFirst(signature.Outputs, retain)
	c.Predict.SetSignature(signature)
}

// SetLLM delegates to Predict.
func (c *DirectivesCoT) SetLLM(llm core.LLM) {
	c.Predict.SetLLM(llm)
	if c.Predict != nil {
		c.Predict.LLM = llm
	}
}

// Clone deep-copies the module.
func (c *DirectivesCoT) Clone() core.Module {
	return &DirectivesCoT{
		Predict: c.Predict.Clone().(*dspymod.Predict),
		name:    c.name,
	}
}

// Compose chains this module with the next.
func (c *DirectivesCoT) Compose(next core.Module) core.Module {
	return core.NewModuleChain(c, next)
}

// GetSubModules returns the inner Predict.
func (c *DirectivesCoT) GetSubModules() []core.Module {
	return []core.Module{c.Predict}
}

// SetSubModules expects exactly one Predict.
func (c *DirectivesCoT) SetSubModules(mods []core.Module) {
	if len(mods) == 1 {
		if predict, ok := mods[0].(*dspymod.Predict); ok {
			c.Predict = predict
		}
	}
}

// Process runs the DirectivesCoT module.
func (c *DirectivesCoT) Process(ctx context.Context, inputs map[string]any, opts ...core.Option) (map[string]any, error) {
	displayName := c.GetDisplayName()
	ctx, span := core.StartSpanWithContext(ctx, "DirectivesCoT", displayName, map[string]any{
		"module_type": c.GetModuleType(),
	})
	defer core.EndSpan(ctx)

	span.WithAnnotation("inputs", inputs)
	outputs, err := c.Predict.Process(ctx, inputs, opts...)
	if err != nil {
		span.WithError(err)
		return nil, fmt.Errorf("directives cot: %w", err)
	}
	span.WithAnnotation("outputs", outputs)
	return outputs, nil
}

// ProcessWithInterceptors runs with interceptor support (JobRunner / factory path).
func (c *DirectivesCoT) ProcessWithInterceptors(ctx context.Context, inputs map[string]any, interceptors []core.ModuleInterceptor, opts ...core.Option) (map[string]any, error) {
	if interceptors == nil {
		interceptors = c.Predict.GetInterceptors()
	}
	info := core.NewModuleInfo(c.GetDisplayName(), c.GetModuleType(), c.GetSignature()).WithLLM(c.Predict.LLM)
	handler := func(ctx context.Context, inputs map[string]any, opts ...core.Option) (map[string]any, error) {
		return c.Process(ctx, inputs, opts...)
	}
	chained := core.ChainModuleInterceptors(interceptors...)
	return chained(ctx, inputs, info, handler, opts...)
}

// SetInterceptors delegates to Predict.
func (c *DirectivesCoT) SetInterceptors(interceptors []core.ModuleInterceptor) {
	c.Predict.SetInterceptors(interceptors)
}

// GetInterceptors delegates to Predict.
func (c *DirectivesCoT) GetInterceptors() []core.ModuleInterceptor {
	return c.Predict.GetInterceptors()
}

// ClearInterceptors delegates to Predict.
func (c *DirectivesCoT) ClearInterceptors() {
	c.Predict.ClearInterceptors()
}
