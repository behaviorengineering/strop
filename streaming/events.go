package streaming

import (
	"time"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
)

// EventType represents the type of streaming event.
type EventType string

const (
	EventTypeChunk       EventType = "chunk"
	EventTypeModuleStart EventType = "module_start"
	EventTypeModuleEnd   EventType = "module_end"
	EventTypeError       EventType = "error"
	EventTypeWarning     EventType = "warning" // Non-fatal issues (e.g. parse failed, retry may follow).
	EventTypeInfo        EventType = "info"    // System/info messages (e.g., "Translation version generated").
	EventTypeDone        EventType = "done"
)

type actorKind string

const (
	actorKindUnspecified  actorKind = ""
	actorKindGenerator    actorKind = "generator"
	actorKindEvaluator    actorKind = "evaluator"
	actorKindConsolidator actorKind = "consolidator"
	actorKindExpert       actorKind = "expert"
)

// Actor is who is running. Kind is unexported; constructors bake identity. Label is display only.
type Actor struct {
	kind  actorKind
	label string
}

// GeneratorActor is a generate-step actor. Label is the TUI/section name.
func GeneratorActor(label string) Actor {
	return Actor{kind: actorKindGenerator, label: label}
}

// EvaluatorActor is a scored evaluator persona.
func EvaluatorActor(label string) Actor {
	return Actor{kind: actorKindEvaluator, label: label}
}

// ConsolidatorActor merges evaluator or research output.
func ConsolidatorActor(label string) Actor {
	return Actor{kind: actorKindConsolidator, label: label}
}

// ExpertActor is a hidden feedback-analysis expert.
func ExpertActor(label string) Actor {
	return Actor{kind: actorKindExpert, label: label}
}

// Label is the display name. It is not identity.
func (a Actor) Label() string {
	return a.label
}

// Identity is kind plus label for deduplication (same label, different roles stay distinct).
func (a Actor) Identity() string {
	return string(a.kind) + "|" + a.label
}

// IsEvaluator reports whether this actor is a scored evaluator persona.
func (a Actor) IsEvaluator() bool {
	return a.kind == actorKindEvaluator
}

// IsExpert reports whether this actor is a hidden feedback-analysis expert.
func (a Actor) IsExpert() bool {
	return a.kind == actorKindExpert
}

// IsConsolidator reports whether this actor merges evaluator or research output.
func (a Actor) IsConsolidator() bool {
	return a.kind == actorKindConsolidator
}

// IsGenerator reports whether this actor is a generate step.
func (a Actor) IsGenerator() bool {
	return a.kind == actorKindGenerator
}

// StartEvent is ModuleStart for this actor.
func (a Actor) StartEvent() InferenceEvent {
	return InferenceEvent{
		Type:       EventTypeModuleStart,
		Actor:      a,
		ModuleName: a.label,
		Timestamp:  time.Now(),
	}
}

// EndEvent is ModuleEnd for this actor.
func (a Actor) EndEvent(outputs map[string]interface{}, err error) InferenceEvent {
	return InferenceEvent{
		Type:              EventTypeModuleEnd,
		Actor:             a,
		ModuleName:        a.label,
		StructuredOutputs: outputs,
		Error:             err,
		Timestamp:         time.Now(),
	}
}

// InferenceEvent represents a single streaming event.
type InferenceEvent struct {
	Type              EventType
	Actor             Actor
	ModuleName        string
	Content           string
	StructuredOutputs map[string]interface{} // Structured outputs from DSPy (after XML parsing).
	Timestamp         time.Time
	Error             error
	TokenUsage        *core.TokenInfo
}

// EventChannel is a channel for streaming inference events.
type EventChannel chan InferenceEvent

// EvaluatorStart builds a ModuleStart event for an evaluator.
func EvaluatorStart(label string) InferenceEvent {
	return EvaluatorActor(label).StartEvent()
}

// EvaluatorEnd builds a ModuleEnd event for an evaluator.
func EvaluatorEnd(label string, outputs map[string]interface{}, err error) InferenceEvent {
	return EvaluatorActor(label).EndEvent(outputs, err)
}

// GeneratorStart builds a ModuleStart event for a generator.
func GeneratorStart(label string) InferenceEvent {
	return GeneratorActor(label).StartEvent()
}

// GeneratorEnd builds a ModuleEnd event for a generator.
func GeneratorEnd(label string, outputs map[string]interface{}, err error) InferenceEvent {
	return GeneratorActor(label).EndEvent(outputs, err)
}

// ConsolidatorStart builds a ModuleStart event for a consolidator.
func ConsolidatorStart(label string) InferenceEvent {
	return ConsolidatorActor(label).StartEvent()
}

// ConsolidatorEnd builds a ModuleEnd event for a consolidator.
func ConsolidatorEnd(label string, outputs map[string]interface{}, err error) InferenceEvent {
	return ConsolidatorActor(label).EndEvent(outputs, err)
}

// ExpertStart builds a ModuleStart event for a feedback expert.
func ExpertStart(label string) InferenceEvent {
	return ExpertActor(label).StartEvent()
}

// ExpertEnd builds a ModuleEnd event for a feedback expert.
func ExpertEnd(label string, outputs map[string]interface{}, err error) InferenceEvent {
	return ExpertActor(label).EndEvent(outputs, err)
}

// SendInfo sends a non-blocking info event. No-op if eventChan is nil; drops if the buffer is full.
func SendInfo(eventChan EventChannel, content string) {
	if eventChan == nil || content == "" {
		return
	}
	select {
	case eventChan <- InferenceEvent{
		Type:      EventTypeInfo,
		Content:   content,
		Timestamp: time.Now(),
	}:
	default:
	}
}
