package dspy

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/behaviorengineering/strop/pkg/evaluation/criteria"
	"github.com/behaviorengineering/strop/pkg/evaluation/voice"
)

func TestAppendVoiceEvaluatorRegistersRole(t *testing.T) {
	t.Parallel()
	cfg := &ChainedEvaluatorConfig{
		Signature: DefaultChainedEvaluatorSignature(),
		CriterionIDs: []criteria.CriterionID{
			criteria.CriterionIDCompleteness,
		},
	}
	profile := voice.Profile{BannedPatterns: []string{"Look,"}}
	AppendVoiceEvaluator(cfg, profile)
	require.Contains(t, cfg.RolePrompts, voice.EvaluatorKey)
	require.Contains(t, cfg.RolePrompts[voice.EvaluatorKey].FeedbackAnalysisPrompt, "voice_profile")
	require.Contains(t, cfg.RolePrompts[voice.EvaluatorKey].FeedbackAnalysisPrompt, "Look,")
	require.Contains(t, cfg.CriterionIDs, criteria.CriterionIDVoiceFidelity)
	require.Contains(t, cfg.CriterionIDs, criteria.CriterionIDCompleteness)
	require.Equal(t, voice.CriterionIDs(), VoiceRoleToCriterionIDs()[voice.EvaluatorKey])
}

func TestAppendVoiceEvaluatorNilConfig(t *testing.T) {
	t.Parallel()
	AppendVoiceEvaluator(nil, voice.Profile{})
}
