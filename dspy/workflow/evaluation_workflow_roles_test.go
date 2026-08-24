package workflow

import (
	"strings"
	"testing"

	"github.com/behaviorengineering/strop/dspy/actor"
	"github.com/behaviorengineering/strop/evaluation"
)

type stubRoleInfo struct {
	names        map[evaluation.EvaluatorKey]string
	roles        map[evaluation.EvaluatorKey]bool
	consolidator evaluation.ConsolidatorKey
	consName     string
}

func (s stubRoleInfo) EvaluatorName(key evaluation.EvaluatorKey) string { return s.names[key] }
func (s stubRoleInfo) HasEvaluator(key evaluation.EvaluatorKey) bool    { return s.roles[key] }
func (s stubRoleInfo) EvaluatorWeight(key evaluation.EvaluatorKey) float64 {
	if s.roles[key] {
		return 1
	}
	return 0
}
func (s stubRoleInfo) ConsolidatorKey() evaluation.ConsolidatorKey { return s.consolidator }
func (s stubRoleInfo) ConsolidatorName() string                    { return s.consName }

func distinctRoleInfo() stubRoleInfo {
	return stubRoleInfo{
		names: map[evaluation.EvaluatorKey]string{
			"style": "Style Evaluator",
		},
		roles:        map[evaluation.EvaluatorKey]bool{"style": true},
		consolidator: "style_consolidator",
		consName:     "Style Consolidator",
	}
}

func TestNewParallelEvaluationWorkflow_rejectsCollidingConsolidatorLabel(t *testing.T) {
	t.Parallel()
	info := distinctRoleInfo()
	info.consName = "Style Evaluator"
	_, err := NewParallelEvaluationWorkflow(WorkflowConfig{
		Evaluators: map[evaluation.EvaluatorKey]actor.Evaluator{"style": {Key: "style", Label: "Style Evaluator"}},
		RoleInfo:   info,
	})
	if err == nil {
		t.Fatal("expected colliding consolidator label")
	}
	if !strings.Contains(err.Error(), "evaluation workflow roles") {
		t.Fatalf("error %q should mention workflow roles", err)
	}
}

func TestNewParallelEvaluationWorkflow_acceptsDistinctConsolidator(t *testing.T) {
	t.Parallel()
	wf, err := NewParallelEvaluationWorkflow(WorkflowConfig{
		Evaluators: map[evaluation.EvaluatorKey]actor.Evaluator{"style": {Key: "style", Label: "Style Evaluator"}},
		RoleInfo:   distinctRoleInfo(),
	})
	if err != nil {
		t.Fatalf("valid roles: %v", err)
	}
	if wf == nil {
		t.Fatal("expected workflow")
	}
	eval := wf.evaluators["style"]
	start := eval.StartEvent()
	if !start.Actor.IsEvaluator() {
		t.Fatal("stored evaluator start must be an evaluator")
	}
	if start.Actor.IsConsolidator() {
		t.Fatal("stored evaluator start must not be a consolidator")
	}
	if start.Actor.Label() != "Style Evaluator" {
		t.Fatalf("evaluator label = %q, want RoleInfo name", start.Actor.Label())
	}
}
