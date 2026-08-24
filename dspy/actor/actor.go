// Package actor wraps DSPy modules as typed pipeline objects: Generator, Evaluator,
// Consolidator, Expert. Identity is the wrapper type; Label is display only.
package actor

import (
	"context"
	"fmt"

	"github.com/XiaoConstantine/dspy-go/pkg/core"

	"github.com/behaviorengineering/strop/evaluation"
	"github.com/behaviorengineering/strop/streaming"
)

// Generator is a generate-step module.
type Generator struct {
	Label  string
	Module core.Module
}

// Actor returns the streaming identity for this generator.
func (g Generator) Actor() streaming.Actor {
	return streaming.GeneratorActor(g.Label)
}

// StartEvent is ModuleStart for this generator.
func (g Generator) StartEvent() streaming.InferenceEvent {
	return g.Actor().StartEvent()
}

// EndEvent is ModuleEnd for this generator.
func (g Generator) EndEvent(outputs map[string]interface{}, err error) streaming.InferenceEvent {
	return g.Actor().EndEvent(outputs, err)
}

// StreamHandler sends chunks as this generator.
func (g Generator) StreamHandler(ch streaming.EventChannel) core.StreamHandler {
	return streaming.StreamHandler(g.Actor(), ch)
}

// Process delegates to the wrapped DSPy module.
func (g Generator) Process(ctx context.Context, inputs map[string]interface{}, opts ...core.Option) (map[string]interface{}, error) {
	if g.Module == nil {
		return nil, fmt.Errorf("generator module is nil")
	}
	return g.Module.Process(ctx, inputs, opts...)
}

// Evaluator is a scored evaluator persona.
type Evaluator struct {
	Key    evaluation.EvaluatorKey
	Label  string
	Module core.Module
}

// Actor returns the streaming identity for this evaluator.
func (e Evaluator) Actor() streaming.Actor {
	return streaming.EvaluatorActor(e.Label)
}

// StartEvent is ModuleStart for this evaluator.
func (e Evaluator) StartEvent() streaming.InferenceEvent {
	return e.Actor().StartEvent()
}

// EndEvent is ModuleEnd for this evaluator.
func (e Evaluator) EndEvent(outputs map[string]interface{}, err error) streaming.InferenceEvent {
	return e.Actor().EndEvent(outputs, err)
}

// StreamHandler sends chunks as this evaluator.
func (e Evaluator) StreamHandler(ch streaming.EventChannel) core.StreamHandler {
	return streaming.StreamHandler(e.Actor(), ch)
}

// Process delegates to the wrapped DSPy module.
func (e Evaluator) Process(ctx context.Context, inputs map[string]interface{}, opts ...core.Option) (map[string]interface{}, error) {
	if e.Module == nil {
		return nil, fmt.Errorf("evaluator module is nil")
	}
	return e.Module.Process(ctx, inputs, opts...)
}

// Consolidator merges evaluator or research output.
type Consolidator struct {
	Key    evaluation.ConsolidatorKey
	Label  string
	Module core.Module
}

// Actor returns the streaming identity for this consolidator.
func (c Consolidator) Actor() streaming.Actor {
	return streaming.ConsolidatorActor(c.Label)
}

// StartEvent is ModuleStart for this consolidator.
func (c Consolidator) StartEvent() streaming.InferenceEvent {
	return c.Actor().StartEvent()
}

// EndEvent is ModuleEnd for this consolidator.
func (c Consolidator) EndEvent(outputs map[string]interface{}, err error) streaming.InferenceEvent {
	return c.Actor().EndEvent(outputs, err)
}

// StreamHandler sends chunks as this consolidator.
func (c Consolidator) StreamHandler(ch streaming.EventChannel) core.StreamHandler {
	return streaming.StreamHandler(c.Actor(), ch)
}

// Process delegates to the wrapped DSPy module.
func (c Consolidator) Process(ctx context.Context, inputs map[string]interface{}, opts ...core.Option) (map[string]interface{}, error) {
	if c.Module == nil {
		return nil, fmt.Errorf("consolidator module is nil")
	}
	return c.Module.Process(ctx, inputs, opts...)
}

// Expert is a hidden feedback-analysis expert.
type Expert struct {
	Key    evaluation.ExpertKey
	Label  string
	Module core.Module
}

// Actor returns the streaming identity for this expert.
func (e Expert) Actor() streaming.Actor {
	return streaming.ExpertActor(e.Label)
}

// StartEvent is ModuleStart for this expert.
func (e Expert) StartEvent() streaming.InferenceEvent {
	return e.Actor().StartEvent()
}

// EndEvent is ModuleEnd for this expert.
func (e Expert) EndEvent(outputs map[string]interface{}, err error) streaming.InferenceEvent {
	return e.Actor().EndEvent(outputs, err)
}

// StreamHandler sends chunks as this expert.
func (e Expert) StreamHandler(ch streaming.EventChannel) core.StreamHandler {
	return streaming.StreamHandler(e.Actor(), ch)
}

// Process delegates to the wrapped DSPy module.
func (e Expert) Process(ctx context.Context, inputs map[string]interface{}, opts ...core.Option) (map[string]interface{}, error) {
	if e.Module == nil {
		return nil, fmt.Errorf("expert module is nil")
	}
	return e.Module.Process(ctx, inputs, opts...)
}
