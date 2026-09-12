package dspy

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/behaviorengineering/strop/evaluation"
	"github.com/behaviorengineering/strop/evaluation/aicadence"
	"github.com/behaviorengineering/strop/evaluation/criteria"
)

func TestAppendAICadenceEvaluator(t *testing.T) {
	t.Parallel()
	cfg := &ChainedEvaluatorConfig{
		Signature: DefaultChainedEvaluatorSignature(),
		RolePrompts: map[evaluation.EvaluatorKey]ChainedEvaluatorRolePrompts{
			"process_evaluator": {
				FeedbackAnalysisPrompt: "fa",
				ScoreGenerationPrompt:  "sg",
			},
		},
		CriterionIDs: []criteria.CriterionID{criteria.CriterionIDCompleteness},
	}
	AppendAICadenceEvaluator(cfg)
	require.Contains(t, cfg.RolePrompts, aicadence.EvaluatorKey)
	require.Contains(t, cfg.RolePrompts[aicadence.EvaluatorKey].FeedbackAnalysisPrompt, "no_aphorism_stack")
	require.Contains(t, cfg.CriterionIDs, criteria.CriterionIDNoAphorismStack)
	require.Contains(t, cfg.CriterionIDs, criteria.CriterionIDCompleteness)
}
