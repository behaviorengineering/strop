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
	onCall  func()
}

func (b *recordingScoreBackend) Generate(ctx context.Context, req scoring.ScoreRequest, _ ...core.Option) (scoring.ScoreResult, error) {
	b.called = true
	b.lastReq = req
	if b.onCall != nil {
		b.onCall()
	}
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
		false,
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
		false,
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

func TestChainedEvaluatorModule_Process_scoreThenFeedback_order(t *testing.T) {
	var steps []string
	stub := &recordingScoreBackend{
		result: scoring.ScoreResult{
			CriterionScores: map[string]float64{
				"instruction_compliance": 1.41,
				"completeness":           1.80,
			},
			DirectivesAck: "jev ack",
		},
		onCall: func() { steps = append(steps, "score") },
	}
	scorePrompt := "CRITERION ID MAPPING\n- Instruction Compliance → \"instruction_compliance\"\n- Completeness → \"completeness\"\n"
	mod, err := CreateChainedEvaluatorModuleWithScoreBackend(
		DefaultChainedEvaluatorSignature(),
		"Test Evaluator",
		"Interpret scores.",
		scorePrompt,
		"",
		nil,
		stub,
		true,
	)
	require.NoError(t, err)

	var faSawCriterionScores bool
	var faScoreEvidence string

	mod.SetInterceptors([]core.ModuleInterceptor{
		func(ctx context.Context, inputs map[string]any, info *core.ModuleInfo, handler core.ModuleHandler, _ ...core.Option) (map[string]any, error) {
			if info != nil && strings.Contains(info.ModuleName, "Feedback Analysis") {
				steps = append(steps, "feedback")
				if cs, ok := inputs[FieldCriterionScores]; ok && cs != nil {
					faSawCriterionScores = true
				}
				if ev, ok := inputs[FieldScoreEvidence].(string); ok {
					faScoreEvidence = ev
				}
				return map[string]any{
					FieldFeedback:      "[ ] [Style Editor] instruction_compliance below max",
					FieldDirectivesAck: "1. Interpreted zeros.",
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
	require.Equal(t, []string{"score", "feedback"}, steps)
	require.True(t, faSawCriterionScores)
	assert.Contains(t, stub.lastReq.Feedback, "=== instruction_compliance ===")
	assert.Equal(t, 1.41, outs[FieldCriterionScores].(map[string]interface{})["instruction_compliance"])
	require.Contains(t, faScoreEvidence, "instruction_compliance")
	idxLow := strings.Index(faScoreEvidence, "instruction_compliance")
	idxHigh := strings.Index(faScoreEvidence, "completeness")
	require.Greater(t, idxHigh, idxLow)
	assert.Contains(t, faScoreEvidence, "1.41 / 2.00 -> 1-point band")
	assert.Contains(t, strings.ToLower(faScoreEvidence), "most instructions followed")
}
