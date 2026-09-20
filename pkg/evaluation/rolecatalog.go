package evaluation

import (
	"fmt"
	"strings"
)

// EvaluatorKey identifies a scored evaluator persona. Distinct from ConsolidatorKey.
type EvaluatorKey string

// String returns the key for factory/registry edges that still take strings.
func (k EvaluatorKey) String() string {
	return string(k)
}

// ConsolidatorKey identifies a feedback consolidator. Distinct from EvaluatorKey.
type ConsolidatorKey string

// String returns the key for factory/registry edges that still take strings.
func (k ConsolidatorKey) String() string {
	return string(k)
}

// ExpertKey identifies a hidden feedback-analysis expert. Distinct from evaluator and consolidator keys.
type ExpertKey string

// String returns the key for factory/registry edges that still take strings.
func (k ExpertKey) String() string {
	return string(k)
}

// ValidateRoleInfo fails when an evaluator key is unknown, duplicated, or a consolidator
// display/agent label collides with an evaluator label. Key-type collisions are a type error.
// Called from NewParallelEvaluationWorkflow at job register.
func ValidateRoleInfo(info RoleInfo, evaluatorKeys []EvaluatorKey) error {
	if info == nil {
		return fmt.Errorf("role info is nil")
	}
	consolidatorKey := info.ConsolidatorKey()
	if strings.TrimSpace(consolidatorKey.String()) == "" {
		return fmt.Errorf("consolidator role key is empty")
	}
	consolidatorLabel := strings.TrimSpace(info.ConsolidatorName())
	if consolidatorLabel == "" {
		return fmt.Errorf("consolidator %q has empty display name", consolidatorKey)
	}

	seenKeys := make(map[string]struct{}, len(evaluatorKeys))
	for _, key := range evaluatorKeys {
		raw := strings.TrimSpace(key.String())
		if raw == "" {
			return fmt.Errorf("evaluator role key is empty")
		}
		if _, dup := seenKeys[strings.ToLower(raw)]; dup {
			return fmt.Errorf("duplicate evaluator role key %q", key)
		}
		seenKeys[strings.ToLower(raw)] = struct{}{}
		if !info.HasEvaluator(key) {
			return fmt.Errorf("unknown evaluator role %q", key)
		}
		evalLabel := strings.TrimSpace(info.EvaluatorName(key))
		if evalLabel == "" {
			return fmt.Errorf("evaluator %q has empty display name", key)
		}
		if strings.EqualFold(consolidatorLabel, evalLabel) {
			return fmt.Errorf("consolidator label %q collides with evaluator %q", consolidatorLabel, key)
		}
	}
	return nil
}
