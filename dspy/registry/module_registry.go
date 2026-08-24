package registry

import (
	"fmt"
	"sync"

	"github.com/behaviorengineering/strop/dspy/actor"
	"github.com/behaviorengineering/strop/dspy/workflow"
	"github.com/behaviorengineering/strop/evaluation"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
	"github.com/XiaoConstantine/dspy-go/pkg/modules"
)

// ConvertEvaluatorMap converts any string-like evaluator role map into a strongly typed evaluator map.
func ConvertEvaluatorMap[R ~string](m map[R]core.Module) map[evaluation.EvaluatorKey]core.Module {
	if m == nil {
		return nil
	}
	result := make(map[evaluation.EvaluatorKey]core.Module, len(m))
	for k, v := range m {
		result[evaluation.EvaluatorKey(k)] = v
	}
	return result
}

// ConvertExpertMap converts any string-like expert role map into typed expert objects.
func ConvertExpertMap[R ~string](m map[R]core.Module) map[evaluation.ExpertKey]actor.Expert {
	if m == nil {
		return nil
	}
	result := make(map[evaluation.ExpertKey]actor.Expert, len(m))
	for k, v := range m {
		key := evaluation.ExpertKey(k)
		result[key] = actor.Expert{
			Key:    key,
			Label:  key.String(),
			Module: v,
		}
	}
	return result
}

// ModuleRegistry manages storage and access to DSPy modules.
// This is a generic registry that works for any pipeline/task.
// It does NOT create modules - that's the factory's responsibility.
type ModuleRegistry struct {
	mu sync.RWMutex

	// Generic storage - maps task name to typed pipeline objects.
	generators         map[string]actor.Generator
	evaluators         map[string]map[evaluation.EvaluatorKey]actor.Evaluator
	consolidators      map[string]actor.Consolidator
	workflows          map[string]*workflow.ParallelEvaluationWorkflow
	feedbackAnalyzers  map[string]map[evaluation.ExpertKey]actor.Expert
	feedbackFormatters map[string]*modules.Predict

	// Maps model ID (string) to provider name (e.g., "openai", "anthropic", "google").
	modelToProvider map[string]string
	// Maps module display name to model ID for cost-tracking fallback when ExecutionState is not set.
	moduleToModel map[string]string
}

// NewModuleRegistry creates a new empty module registry.
func NewModuleRegistry() *ModuleRegistry {
	return &ModuleRegistry{
		generators:         make(map[string]actor.Generator),
		evaluators:         make(map[string]map[evaluation.EvaluatorKey]actor.Evaluator),
		consolidators:      make(map[string]actor.Consolidator),
		workflows:          make(map[string]*workflow.ParallelEvaluationWorkflow),
		feedbackAnalyzers:  make(map[string]map[evaluation.ExpertKey]actor.Expert),
		feedbackFormatters: make(map[string]*modules.Predict),
		modelToProvider:    make(map[string]string),
		moduleToModel:      make(map[string]string),
	}
}

// RegisterGenerator wraps a DSPy module as a Generator and stores it for a task.
func (r *ModuleRegistry) RegisterGenerator(task string, module core.Module) {
	r.RegisterGeneratorLabeled(task, task, module)
}

// RegisterGeneratorLabeled stores a generator with an explicit streaming label.
func (r *ModuleRegistry) RegisterGeneratorLabeled(task, label string, module core.Module) {
	if label == "" {
		label = task
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.generators[task] = actor.Generator{Label: label, Module: module}
}

// WrapEvaluators wraps evaluator modules using RoleInfo display names for streaming labels.
func WrapEvaluators(modules map[evaluation.EvaluatorKey]core.Module, roleInfo evaluation.RoleInfo) map[evaluation.EvaluatorKey]actor.Evaluator {
	if modules == nil {
		return nil
	}
	out := make(map[evaluation.EvaluatorKey]actor.Evaluator, len(modules))
	for k, m := range modules {
		label := k.String()
		if roleInfo != nil {
			if name := roleInfo.EvaluatorName(k); name != "" {
				label = name
			}
		}
		out[k] = actor.Evaluator{
			Key:    k,
			Label:  label,
			Module: m,
		}
	}
	return out
}

// RegisterEvaluators stores evaluator objects for a task.
func (r *ModuleRegistry) RegisterEvaluators(task string, evaluators map[evaluation.EvaluatorKey]actor.Evaluator) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.evaluators[task] = evaluators
}

// RegisterConsolidator wraps a consolidator module for a task.
func (r *ModuleRegistry) RegisterConsolidator(
	task string,
	key evaluation.ConsolidatorKey,
	label string,
	module core.Module,
) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.consolidators[task] = actor.Consolidator{
		Key:    key,
		Label:  label,
		Module: module,
	}
}

// RegisterWorkflow registers an evaluation workflow for a task.
func (r *ModuleRegistry) RegisterWorkflow(task string, wf *workflow.ParallelEvaluationWorkflow) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.workflows[task] = wf
}

// RegisterModelProvider registers a model-to-provider mapping for cost tracking.
func (r *ModuleRegistry) RegisterModelProvider(modelID, providerType string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.modelToProvider[modelID] = providerType
}

// GetModelProvider returns the provider type for a model ID.
func (r *ModuleRegistry) GetModelProvider(modelID string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.modelToProvider[modelID]
}

// RegisterModuleModel registers a module display name to model ID mapping.
// Used by the OpenInference interceptor as a fallback when ExecutionState does not contain the model ID.
func (r *ModuleRegistry) RegisterModuleModel(moduleName, modelID string) {
	if moduleName == "" || modelID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.moduleToModel[moduleName] = modelID
}

// GetModuleModel returns the model ID for a module display name, or empty string if not registered.
func (r *ModuleRegistry) GetModuleModel(moduleName string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.moduleToModel[moduleName]
}

// getFromMap is a generic helper for getting values from maps with common error handling.
// T can be any type (including maps, slices, pointers, etc.).
func getFromMap[T any](
	mu *sync.RWMutex,
	m map[string]T,
	task string,
	notFoundErr string,
	nilErr string,
	isNil func(T) bool, // Function to check if value is nil/equivalent to zero.
) (T, error) {
	mu.RLock()
	defer mu.RUnlock()

	var zero T
	value, ok := m[task]
	if !ok {
		return zero, fmt.Errorf(notFoundErr, task)
	}

	// Check for nil using provided function.
	if isNil(value) {
		return zero, fmt.Errorf(nilErr, task)
	}

	return value, nil
}

// GetGenerator returns the generator object for a task.
func (r *ModuleRegistry) GetGenerator(task string) (actor.Generator, error) {
	return getFromMap(
		&r.mu,
		r.generators,
		task,
		ErrGeneratorNotFound,
		ErrGeneratorNil,
		func(v actor.Generator) bool { return v.Module == nil },
	)
}

// GetEvaluators returns the evaluator objects for a task.
func (r *ModuleRegistry) GetEvaluators(task string) (map[evaluation.EvaluatorKey]actor.Evaluator, error) {
	return getFromMap(
		&r.mu,
		r.evaluators,
		task,
		ErrEvaluatorsNotFound,
		ErrEvaluatorsNil,
		func(v map[evaluation.EvaluatorKey]actor.Evaluator) bool { return v == nil },
	)
}

// GetConsolidator returns the consolidator object for a task.
func (r *ModuleRegistry) GetConsolidator(task string) (actor.Consolidator, error) {
	return getFromMap(
		&r.mu,
		r.consolidators,
		task,
		ErrConsolidatorNotFound,
		ErrConsolidatorNil,
		func(v actor.Consolidator) bool { return v.Module == nil },
	)
}

// GetWorkflow returns the evaluation workflow for a task.
func (r *ModuleRegistry) GetWorkflow(task string) (*workflow.ParallelEvaluationWorkflow, error) {
	return getFromMap(
		&r.mu,
		r.workflows,
		task,
		ErrWorkflowNotFound,
		ErrWorkflowNil,
		func(v *workflow.ParallelEvaluationWorkflow) bool { return v == nil },
	)
}

// RegisterFeedbackAnalyzers stores expert objects for a task.
func (r *ModuleRegistry) RegisterFeedbackAnalyzers(task string, experts map[evaluation.ExpertKey]actor.Expert) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.feedbackAnalyzers[task] = normalizeExperts(experts)
}

// RegisterFeedbackFormatter registers a feedback formatter module for a task.
func (r *ModuleRegistry) RegisterFeedbackFormatter(task string, formatter *modules.Predict) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.feedbackFormatters[task] = formatter
}

// GetFeedbackAnalyzers returns the expert objects for a task.
func (r *ModuleRegistry) GetFeedbackAnalyzers(task string) (map[evaluation.ExpertKey]actor.Expert, error) {
	return getFromMap(
		&r.mu,
		r.feedbackAnalyzers,
		task,
		ErrFeedbackAnalyzersNotFound,
		ErrFeedbackAnalyzersNil,
		func(v map[evaluation.ExpertKey]actor.Expert) bool { return v == nil },
	)
}

// GetFeedbackFormatter returns the feedback formatter module for a task.
func (r *ModuleRegistry) GetFeedbackFormatter(task string) (*modules.Predict, error) {
	return getFromMap(
		&r.mu,
		r.feedbackFormatters,
		task,
		ErrFeedbackFormatterNotFound,
		ErrFeedbackFormatterNil,
		func(v *modules.Predict) bool { return v == nil },
	)
}

func normalizeExperts(experts map[evaluation.ExpertKey]actor.Expert) map[evaluation.ExpertKey]actor.Expert {
	if experts == nil {
		return nil
	}
	out := make(map[evaluation.ExpertKey]actor.Expert, len(experts))
	for k, expert := range experts {
		expert.Key = k
		if expert.Label == "" {
			expert.Label = k.String()
		}
		out[k] = expert
	}
	return out
}
