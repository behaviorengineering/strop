package dspy

import (
	"context"
	"fmt"

	dspymodules "github.com/behaviorengineering/strop/pkg/dspy/modules"
	"github.com/behaviorengineering/strop/pkg/evaluation/scoring"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
)

var _ scoring.ScoreModuleProvider = (*llmScoreBackend)(nil)

// llmScoreBackend runs score generation through a DirectivesCoT module.
type llmScoreBackend struct {
	scoreModule *dspymodules.DirectivesCoT
}

// NewLLMScoreBackend wraps the score-generation DirectivesCoT module.
func NewLLMScoreBackend(scoreModule *dspymodules.DirectivesCoT) scoring.Backend {
	return &llmScoreBackend{scoreModule: scoreModule}
}

// ScoreModule returns the underlying module for factory LLM/XML setup.
func (b *llmScoreBackend) ScoreModule() core.Module {
	if b == nil || b.scoreModule == nil {
		return nil
	}
	return b.scoreModule
}

// Generate calls the score module and normalizes criterion_scores and directives_ack.
func (b *llmScoreBackend) Generate(ctx context.Context, req scoring.ScoreRequest, opts ...core.Option) (scoring.ScoreResult, error) {
	if b == nil || b.scoreModule == nil {
		return scoring.ScoreResult{}, fmt.Errorf("LLM score backend is not configured")
	}
	if req.Feedback == "" {
		return scoring.ScoreResult{}, fmt.Errorf("feedback is required for score generation")
	}

	scoreInputs := make(map[string]interface{})
	if req.GeneratorInput != nil {
		scoreInputs[FieldGeneratorInput] = req.GeneratorInput
	}
	if req.GeneratorOutput != nil {
		scoreInputs[FieldGeneratorOutput] = req.GeneratorOutput
	}
	scoreInputs[FieldFeedback] = req.Feedback

	scoreResult, err := b.scoreModule.Process(ctx, scoreInputs, req.ModuleOptions...)
	if err != nil {
		return scoring.ScoreResult{}, err
	}
	if len(scoreResult) == 0 {
		return scoring.ScoreResult{}, fmt.Errorf("score generation returned empty result")
	}

	criterionScoresValue, exists := scoreResult[FieldCriterionScores]
	if !exists {
		return scoring.ScoreResult{}, fmt.Errorf("criterion_scores field is missing in score generation result")
	}
	coerced, err := CoerceCriterionScoresMap(criterionScoresValue)
	if err != nil {
		return scoring.ScoreResult{}, fmt.Errorf("criterion_scores invalid: %w", err)
	}
	floatScores, err := scoring.FloatMapFromCriterionScores(coerced)
	if err != nil {
		return scoring.ScoreResult{}, err
	}

	ack, err := ExtractRequiredReasoningField(scoreResult)
	if err != nil {
		return scoring.ScoreResult{}, err
	}

	return scoring.ScoreResult{
		CriterionScores: floatScores,
		DirectivesAck:   ack,
	}, nil
}
