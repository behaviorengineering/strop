package workflow

import (
	"context"
	"errors"
	"testing"

	"github.com/behaviorengineering/strop/dspy/actor"
	"github.com/behaviorengineering/strop/evaluation"
	stroplog "github.com/behaviorengineering/strop/log"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
)

type failingModule struct {
	err error
}

func (m *failingModule) Process(context.Context, map[string]any, ...core.Option) (map[string]any, error) {
	return nil, m.err
}

func (m *failingModule) GetSignature() core.Signature {
	return core.NewSignature(nil, nil)
}

func (m *failingModule) SetSignature(core.Signature) {}

func (m *failingModule) SetLLM(core.LLM) {}

func (m *failingModule) Clone() core.Module {
	return m
}

func (m *failingModule) GetDisplayName() string {
	return "failing module"
}

func (m *failingModule) GetModuleType() string {
	return "test"
}

type noopWorkflowLogger struct{}

func (noopWorkflowLogger) WithField(string, interface{}) stroplog.Logger {
	return noopWorkflowLogger{}
}

func (noopWorkflowLogger) WithFields(map[string]interface{}) stroplog.Logger {
	return noopWorkflowLogger{}
}

func (noopWorkflowLogger) WithError(error) stroplog.Logger {
	return noopWorkflowLogger{}
}

func (noopWorkflowLogger) Debug(...interface{}) {}
func (noopWorkflowLogger) Info(...interface{})  {}
func (noopWorkflowLogger) Warn(...interface{})  {}
func (noopWorkflowLogger) Error(...interface{}) {}

func TestRunIndividualEvaluatorsPreservesAllFailures(t *testing.T) {
	t.Parallel()

	styleErr := errors.New("style evaluator failed")
	processErr := errors.New("process evaluator failed")
	workflow := &ParallelEvaluationWorkflow{
		evaluators: map[evaluation.EvaluatorKey]actor.Evaluator{
			"style":   {Key: "style", Module: &failingModule{err: styleErr}},
			"process": {Key: "process", Module: &failingModule{err: processErr}},
		},
		logger: noopWorkflowLogger{},
	}

	_, err := workflow.runIndividualEvaluators(context.Background(), nil)
	if err == nil {
		t.Fatal("expected evaluator failure")
	}
	if !errors.Is(err, styleErr) {
		t.Fatal("expected style evaluator cause to remain discoverable")
	}
	if !errors.Is(err, processErr) {
		t.Fatal("expected process evaluator cause to remain discoverable")
	}
}
