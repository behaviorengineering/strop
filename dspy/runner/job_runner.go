package runner

import (
	"context"
	"fmt"
	"strings"

	stropdspy "github.com/behaviorengineering/strop/dspy"
	dspymodules "github.com/behaviorengineering/strop/dspy/modules"
	"github.com/behaviorengineering/strop/dspy/registry"
	"github.com/behaviorengineering/strop/dspy/tracing"
	"github.com/behaviorengineering/strop/evaluation"
	stroplog "github.com/behaviorengineering/strop/log"
	"github.com/behaviorengineering/strop/streaming"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
)

// DemoSelection is the near/contrast pair SetDemos on one Generate call.
type DemoSelection struct {
	NearID     string
	ContrastID string
}

// LearningExampleArtifact represents a learning artifact for use as an example.
type LearningExampleArtifact struct {
	ID              string
	ArtifactType    string
	ArtifactContent map[string]interface{}
	QualityStatus   string
}

// GenerationResult is the output of Generate including which demos were applied.
type GenerationResult struct {
	Outputs map[string]interface{}
	Demos   DemoSelection
}

// LearningServiceForGeneration provides examples for a job/step.
type LearningServiceForGeneration interface {
	GetExamplesForGeneration(
		ctx context.Context,
		job string,
		step string,
		context map[string]interface{},
		limit int,
	) ([]*LearningExampleArtifact, error)
}

// ExampleFormatter converts learning artifacts into DSPy examples for a job/step.
// usedIDs lists artifact IDs that successfully became examples (same order as examples).
type ExampleFormatter interface {
	FormatExamples(artifacts []*LearningExampleArtifact, job, step string) (examples []core.Example, usedIDs []string, err error)
}

// GeneratorInput is the input contract for Generate.
type GeneratorInput interface {
	ToMap() map[string]interface{}
	GetVersion() int
}

// EvaluationInput is the evaluator-view contract.
// EvaluationMap is the job-specific generator_input bag. GetVersion is iterationVersion.
type EvaluationInput interface {
	EvaluationMap() map[string]interface{}
	GetVersion() int
}

// buildEvaluationInputs builds the outer workflow envelope.
// Inner generator_input / generator_output keys stay job-specific.
func buildEvaluationInputs(generatorInput EvaluationInput, generatorOutput map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		stropdspy.FieldGeneratorInput:   generatorInput.EvaluationMap(),
		stropdspy.FieldGeneratorOutput:  generatorOutput,
		stropdspy.FieldIterationVersion: generatorInput.GetVersion(),
	}
}

// GenerationConfig configures a single Generate call.
type GenerationConfig struct {
	ModuleName      string
	JobName         string
	StepName        string
	ErrorCode       string
	ErrorMessage    string
	OutputExtractor func(map[string]interface{}) (map[string]interface{}, error)
}

// JobRunner runs generator modules and evaluation workflows for any pipeline.
type JobRunner struct {
	registry         *registry.ModuleRegistry
	learningService  LearningServiceForGeneration
	exampleFormatter ExampleFormatter
	logger           stroplog.Logger
}

// NewJobRunner creates a runner for generation and evaluation.
func NewJobRunner(
	reg *registry.ModuleRegistry,
	learningService LearningServiceForGeneration,
	exampleFormatter ExampleFormatter,
	logger stroplog.Logger,
) *JobRunner {
	if reg == nil {
		panic("registry cannot be nil")
	}
	return &JobRunner{
		registry:         reg,
		learningService:  learningService,
		exampleFormatter: exampleFormatter,
		logger:           logger,
	}
}

// Generate runs the generator module and returns raw outputs.
func (r *JobRunner) Generate(
	ctx context.Context,
	config GenerationConfig,
	input GeneratorInput,
	eventChan streaming.EventChannel,
) (map[string]interface{}, error) {
	result, err := r.GenerateResult(ctx, config, input, eventChan)
	if err != nil {
		return nil, err
	}
	return result.Outputs, nil
}

// GenerateResult runs the generator and returns outputs plus demo selection IDs.
func (r *JobRunner) GenerateResult(
	ctx context.Context,
	config GenerationConfig,
	input GeneratorInput,
	eventChan streaming.EventChannel,
) (*GenerationResult, error) {
	gen, err := r.registry.GetGenerator(config.ModuleName)
	if err != nil {
		msg := fmt.Sprintf("%s not initialized", config.ErrorMessage)
		if strings.Contains(err.Error(), "not found") {
			msg += " (check config: job_configs.<task>.modules.generator and ai_providers; see startup logs for skip reason)"
		}
		return nil, fmt.Errorf("%s: %w", msg, err)
	}
	if _, err := dspymodules.PredictOf(gen.Module); err != nil {
		return nil, fmt.Errorf("%s: %w", fmt.Sprintf("%s is not a Predict-backed generator module", config.ErrorMessage),
			err,
		)
	}
	inputs := input.ToMap()
	inputs[stropdspy.FieldIterationVersion] = input.GetVersion()

	if eventChan != nil {
		ctx = streaming.ContextWithEventChannel(ctx, eventChan)
		eventChan <- gen.StartEvent()
	}

	var demos DemoSelection
	var beforeProcess func(core.Module) error
	if r.learningService != nil && r.exampleFormatter != nil {
		jobName, stepName := config.JobName, config.StepName
		beforeProcess = func(m core.Module) error {
			selection, err := r.retrieveAndSetExamples(ctx, m, jobName, stepName, inputs)
			if err != nil && r.logger != nil {
				r.logger.WithError(err).Warn("Failed to retrieve learning examples, continuing without examples")
			}
			demos = selection
			return nil
		}
	}
	var opts []core.Option
	if eventChan != nil {
		opts = append(opts, core.WithStreamHandler(gen.StreamHandler(eventChan)))
	}
	outputs, err := stropdspy.RunModule(ctx, gen.Module, inputs, beforeProcess, opts...)
	// Normalize array fields that may be returned as JSON strings (quotes, topics) so display shows a list.
	if outputs != nil {
		stropdspy.NormalizeOutputArrayFields(outputs, []string{"quotes", "topics"})
	}
	if eventChan != nil {
		eventChan <- gen.EndEvent(outputs, err)
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fmt.Sprintf("Failed to generate %s", config.ErrorMessage),
			stropdspy.SanitizeDSPyError(err),
		)
	}
	return &GenerationResult{Outputs: outputs, Demos: demos}, nil
}

// EvaluateWorkflow runs the job's evaluation workflow.
func (r *JobRunner) EvaluateWorkflow(
	ctx context.Context,
	job string,
	generatorInput EvaluationInput,
	generatorOutput map[string]interface{},
	eventChan streaming.EventChannel,
) (*evaluation.AggregatedEvaluation, error) {
	if generatorInput == nil {
		return nil, fmt.Errorf("%s", fmt.Sprintf("evaluation input is nil for %s", job))
	}
	inputs := buildEvaluationInputs(generatorInput, generatorOutput)
	workflow, err := r.registry.GetWorkflow(job)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fmt.Sprintf("Failed to get %s evaluation workflow", job), err)
	}
	result, err := workflow.EvaluateStream(ctx, inputs, eventChan)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fmt.Sprintf("Failed to evaluate %s", job), stropdspy.SanitizeDSPyError(err))
	}
	return result, nil
}

func (r *JobRunner) retrieveAndSetExamples(
	ctx context.Context,
	module core.Module,
	job, step string,
	generatorInputs map[string]interface{},
) (DemoSelection, error) {
	var empty DemoSelection
	if r.learningService == nil || r.exampleFormatter == nil {
		return empty, nil
	}
	predict, err := dspymodules.PredictOf(module)
	if err != nil {
		return empty, err
	}
	contextMap := generatorInputs
	if contextMap == nil {
		contextMap = make(map[string]interface{})
	}
	const maxExamples = 2
	artifacts, err := r.learningService.GetExamplesForGeneration(ctx, job, step, contextMap, maxExamples)
	if err != nil {
		return empty, fmt.Errorf("failed to retrieve learning examples: %w", err)
	}
	if len(artifacts) == 0 {
		return empty, nil
	}
	examples, usedIDs, err := r.exampleFormatter.FormatExamples(artifacts, job, step)
	if err != nil {
		return empty, fmt.Errorf("failed to format learning examples: %w", err)
	}
	if len(examples) == 0 {
		return empty, nil
	}
	predict.SetDemos(examples)
	if r.logger != nil {
		r.logger.WithFields(map[string]interface{}{
			"job": job, "step": step, "examples": len(examples),
		}).Debug("Set learning examples on generator module")
	}
	selection := DemoSelection{}
	if len(usedIDs) > 0 {
		selection.NearID = usedIDs[0]
	}
	if len(usedIDs) > 1 {
		selection.ContrastID = usedIDs[1]
	}
	tracing.MarkDemoSelectionOnChainSpan(ctx, job, step, selection.NearID, selection.ContrastID)
	return selection, nil
}

// FormatExample builds a single DSPy example from raw input/output and builder funcs.
func FormatExample(
	inputRaw, outputRaw interface{},
	inputBuilder func(map[string]interface{}, string) (map[string]interface{}, error),
	outputBuilder func(interface{}) (map[string]interface{}, error),
) (core.Example, error) {
	inputMap, err := ParseArtifactInput(inputRaw)
	if err != nil {
		return core.Example{}, err
	}
	originalText, err := ExtractOriginalText(inputMap)
	if err != nil {
		return core.Example{}, err
	}
	exampleInputs, err := inputBuilder(inputMap, originalText)
	if err != nil {
		return core.Example{}, err
	}
	exampleOutputs, err := outputBuilder(outputRaw)
	if err != nil {
		return core.Example{}, err
	}
	return core.Example{Inputs: exampleInputs, Outputs: exampleOutputs}, nil
}

// ParseArtifactInput parses artifact input (map or string).
func ParseArtifactInput(inputRaw interface{}) (map[string]interface{}, error) {
	if m, ok := inputRaw.(map[string]interface{}); ok {
		return m, nil
	}
	if s, ok := inputRaw.(string); ok {
		return map[string]interface{}{stropdspy.FieldOriginalText: s}, nil
	}
	return nil, fmt.Errorf("input must be map or string, got %T", inputRaw)
}

// ExtractOriginalText returns original_text from an input map.
func ExtractOriginalText(inputMap map[string]interface{}) (string, error) {
	v, ok := inputMap[stropdspy.FieldOriginalText]
	if !ok {
		return "", fmt.Errorf("original_text is required in input")
	}
	s, ok := v.(string)
	if !ok || s == "" {
		return "", fmt.Errorf("original_text must be a non-empty string")
	}
	return s, nil
}

// GetStringFromMap safely returns a string value from a map.
func GetStringFromMap(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// ExtractRequiredField extracts a required string field and wraps failure with a domain error.
func ExtractRequiredField(outputs map[string]interface{}, fieldName, errorCode string) (string, error) {
	v, err := stropdspy.ExtractRequiredStringField(outputs, fieldName)
	if err != nil {
		return "", fmt.Errorf("%s: %w", fmt.Sprintf("module returned empty or invalid field %q", fieldName), err)
	}
	return v, nil
}

// ExtractRequiredReasoning extracts directives_ack and wraps failure with a domain error.
func ExtractRequiredReasoning(outputs map[string]interface{}, errorCode string) (string, error) {
	v, err := stropdspy.ExtractRequiredReasoningField(outputs)
	if err != nil {
		return "", fmt.Errorf("%s: %w", "module returned empty or invalid directives_ack", err)
	}
	return v, nil
}

// JobExampleFormatter formats one (input, output) pair into a DSPy example for a single job/step.
// Pipelines implement this per job; the composition root injects a map into CompositeExampleFormatter.
type JobExampleFormatter interface {
	FormatOne(inputRaw, outputRaw interface{}) (core.Example, error)
}

// JobStepKey returns a stable key for (job, step) used in the formatter map.
func JobStepKey(job, step string) string {
	return job + "|" + step
}

// CompositeExampleFormatter delegates to per-job formatters keyed by job|step.
// It implements ExampleFormatter; formatters are injected by the caller (IoC).
type CompositeExampleFormatter struct {
	formatters map[string]JobExampleFormatter
	logger     stroplog.Logger
}

// NewCompositeExampleFormatter creates a composite that uses the injected formatters map.
// The caller builds the map from JobStepKey(job, step) to formatter. logger may be nil.
func NewCompositeExampleFormatter(logger stroplog.Logger, formatters map[string]JobExampleFormatter) *CompositeExampleFormatter {
	m := make(map[string]JobExampleFormatter, len(formatters))
	for k, v := range formatters {
		m[k] = v
	}
	return &CompositeExampleFormatter{formatters: m, logger: logger}
}

// FormatExamples implements ExampleFormatter.
func (f *CompositeExampleFormatter) FormatExamples(
	artifacts []*LearningExampleArtifact,
	job, step string,
) ([]core.Example, []string, error) {
	formatter := f.formatters[JobStepKey(job, step)]
	if formatter == nil {
		if f.logger != nil {
			f.logger.WithFields(map[string]interface{}{"job": job, "step": step}).
				Debug("No formatter for job/step, skipping artifacts")
		}
		return nil, nil, nil
	}
	const artifactTypeGeneratorExample = "generator_example"
	examples := make([]core.Example, 0, len(artifacts))
	usedIDs := make([]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		if artifact == nil || artifact.ArtifactType != artifactTypeGeneratorExample {
			continue
		}
		inputRaw, hasInput := artifact.ArtifactContent["input"]
		outputRaw, hasOutput := artifact.ArtifactContent["output"]
		if !hasInput || !hasOutput {
			if f.logger != nil {
				f.logger.WithFields(map[string]interface{}{
					"artifact_id": artifact.ID, "has_input": hasInput, "has_output": hasOutput,
				}).Debug("Skipping artifact - missing input or output")
			}
			continue
		}
		example, err := formatter.FormatOne(inputRaw, outputRaw)
		if err != nil {
			if f.logger != nil {
				f.logger.WithError(err).WithFields(map[string]interface{}{
					"artifact_id": artifact.ID, "job": job, "step": step,
				}).Warn("Failed to format artifact as DSPy example, skipping")
			}
			continue
		}
		examples = append(examples, example)
		if id := strings.TrimSpace(artifact.ID); id != "" {
			usedIDs = append(usedIDs, id)
		}
	}
	return examples, usedIDs, nil
}
