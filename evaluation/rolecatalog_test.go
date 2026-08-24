package evaluation

import (
	"strings"
	"testing"
)

type stubRoleInfo struct {
	names        map[EvaluatorKey]string
	roles        map[EvaluatorKey]bool
	weights      map[EvaluatorKey]float64
	consolidator ConsolidatorKey
	consName     string
}

func (s stubRoleInfo) EvaluatorName(key EvaluatorKey) string { return s.names[key] }
func (s stubRoleInfo) HasEvaluator(key EvaluatorKey) bool    { return s.roles[key] }
func (s stubRoleInfo) EvaluatorWeight(key EvaluatorKey) float64 {
	if w, ok := s.weights[key]; ok {
		return w
	}
	return 1
}
func (s stubRoleInfo) ConsolidatorKey() ConsolidatorKey { return s.consolidator }
func (s stubRoleInfo) ConsolidatorName() string         { return s.consName }

func validRoleInfo() stubRoleInfo {
	return stubRoleInfo{
		names:        map[EvaluatorKey]string{"prose_style": "Prose Style Evaluator"},
		roles:        map[EvaluatorKey]bool{"prose_style": true},
		consolidator: "prose_style_consolidator",
		consName:     "Prose Style Consolidator",
	}
}

func TestValidateRoleInfo_acceptsDistinctIdentity(t *testing.T) {
	t.Parallel()
	if err := ValidateRoleInfo(validRoleInfo(), []EvaluatorKey{"prose_style"}); err != nil {
		t.Fatalf("valid roles: %v", err)
	}
}

func TestValidateRoleInfo_rejectsNil(t *testing.T) {
	t.Parallel()
	if err := ValidateRoleInfo(nil, nil); err == nil {
		t.Fatal("expected error for nil role info")
	}
}

func TestValidateRoleInfo_rejectsEmptyConsolidatorKey(t *testing.T) {
	t.Parallel()
	info := validRoleInfo()
	info.consolidator = "  "
	if err := ValidateRoleInfo(info, []EvaluatorKey{"prose_style"}); err == nil {
		t.Fatal("expected error for empty consolidator key")
	}
}

func TestValidateRoleInfo_rejectsLabelCollision(t *testing.T) {
	t.Parallel()
	info := validRoleInfo()
	info.consName = "Prose Style Evaluator"
	err := ValidateRoleInfo(info, []EvaluatorKey{"prose_style"})
	if err == nil {
		t.Fatal("expected label collision")
	}
	if !strings.Contains(err.Error(), "collides") {
		t.Fatalf("error %q should mention collision", err)
	}
}

func TestValidateRoleInfo_rejectsUnknownEvaluator(t *testing.T) {
	t.Parallel()
	if err := ValidateRoleInfo(validRoleInfo(), []EvaluatorKey{"missing"}); err == nil {
		t.Fatal("expected unknown evaluator")
	}
}

func TestValidateRoleInfo_rejectsDuplicateEvaluatorKeys(t *testing.T) {
	t.Parallel()
	if err := ValidateRoleInfo(validRoleInfo(), []EvaluatorKey{"prose_style", "Prose_Style"}); err == nil {
		t.Fatal("expected duplicate evaluator key")
	}
}
