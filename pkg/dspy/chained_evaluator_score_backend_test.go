package dspy

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/behaviorengineering/strop/pkg/evaluation/criteria"
	"github.com/behaviorengineering/strop/pkg/evaluation/scoring"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
)

type recordingScoreBackend struct {
	called  bool
	lastReq scoring.ScoreRequest
	result  scoring.ScoreResult
}

func (b *recordingScoreBackend) Generate(ctx context.Context, req scoring.ScoreRequest, _ ...core.Option) (scoring.ScoreResult, error) {
	b.called = true
	b.lastReq = req
	return b.result, nil
}

func TestChainedEvaluatorModule_injectedScoreBackend_skipsScoreModule(t *testing.T) {
	t.Parallel()
	stub := &recordingScoreBackend{
		result: scoring.ScoreResult{
			CriterionScores: map[string]float64{"instruction_compliance": 2},
			DirectivesAck:   "stub scoring ack",
		},
	}
	scorePrompt := "CRITERION ID MAPPING\n- Instruction Compliance → \"instruction_compliance\"\n"
	mod, err := CreateChainedEvaluatorModuleWithScoreBackend(
		DefaultChainedEvaluatorSignature(),
		"Test Evaluator",
		"Analyze feedback.",
		scorePrompt,
		"",
		nil,
		stub,
	)
	require.NoError(t, err)
	assert.Nil(t, mod.GetScoreGenerationModule())
	assert.Equal(t, stub, mod.GetScoreBackend())
}

func TestChainedEvaluatorModule_Process_usesInjectedScoreBackend(t *testing.T) {
	stub := &recordingScoreBackend{
		result: scoring.ScoreResult{
			CriterionScores: map[string]float64{"instruction_compliance": 2},
			DirectivesAck:   "stub scoring ack",
		},
	}
	scorePrompt := "CRITERION ID MAPPING\n- Instruction Compliance → \"instruction_compliance\"\n"
	mod, err := CreateChainedEvaluatorModuleWithScoreBackend(
		DefaultChainedEvaluatorSignature(),
		"Test Evaluator",
		"Analyze feedback.",
		scorePrompt,
		"",
		nil,
		stub,
	)
	require.NoError(t, err)

	mod.SetInterceptors([]core.ModuleInterceptor{
		func(ctx context.Context, inputs map[string]any, info *core.ModuleInfo, handler core.ModuleHandler, _ ...core.Option) (map[string]any, error) {
			if info != nil && strings.Contains(info.ModuleName, "Feedback Analysis") {
				return map[string]any{
					FieldFeedback:      "[✓] All criteria met.",
					FieldDirectivesAck: "1. Reviewed checklist.",
				}, nil
			}
			return handler(ctx, inputs)
		},
	})

	outs, err := mod.Process(context.Background(), map[string]interface{}{
		FieldGeneratorInput:  map[string]interface{}{"version": 1},
		FieldGeneratorOutput: map[string]interface{}{"text": "hello"},
	})
	require.NoError(t, err)
	require.True(t, stub.called)
	assert.Equal(t, "[✓] All criteria met.", outs[FieldFeedback])
	assert.Equal(t, 2.0, outs[FieldCriterionScores].(map[string]interface{})["instruction_compliance"])
	assert.Contains(t, outs[FieldDirectivesAck], "stub scoring ack")
	assert.Equal(t, "[✓] All criteria met.", stub.lastReq.Feedback)
	assert.Equal(t, []criteria.CriterionID{criteria.CriterionIDInstructionCompliance}, stub.lastReq.CriterionIDs)
}
