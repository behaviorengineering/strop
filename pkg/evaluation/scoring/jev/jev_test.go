package jev_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/behaviorengineering/strop/pkg/evaluation/criteria"
	"github.com/behaviorengineering/strop/pkg/evaluation/scoring"
	"github.com/behaviorengineering/strop/pkg/evaluation/scoring/jev"

	"github.com/XiaoConstantine/dspy-go/pkg/experimental/decide"
	"github.com/XiaoConstantine/dspy-go/pkg/experimental/typesafe"
)

type stubSystemOneClient struct {
	scoreIndex float64
}

func (c *stubSystemOneClient) SystemOne(_ context.Context, request typesafe.SystemOneRequest) (*typesafe.SystemOneResponse, error) {
	answers := make(map[string]typesafe.Answer, len(request.Questions))
	for name, question := range request.Questions {
		levelCount := scoreQuestionLevelCount(question)
		probs := uniformScoreProbabilities(c.scoreIndex, levelCount)
		answers[name] = typesafe.ScoreAnswer{
			Score:         c.scoreIndex,
			Confidence:    0.9,
			Probabilities: probs,
		}
	}
	return &typesafe.SystemOneResponse{
		Model:   "jev-test",
		Answers: answers,
		Usage:   typesafe.Usage{InputTokens: 1, OutputTokens: 1},
	}, nil
}

func scoreQuestionLevelCount(question typesafe.Question) int {
	if sq, ok := question.(typesafe.ScoreQuestion); ok {
		n := len(sq.Criteria)
		if n >= 2 {
			return n
		}
	}
	return 3
}

func uniformScoreProbabilities(selected float64, levelCount int) map[int]float64 {
	if levelCount < 2 {
		levelCount = 2
	}
	index := int(selected)
	if index < 0 {
		index = 0
	}
	if index >= levelCount {
		index = levelCount - 1
	}
	out := make(map[int]float64, levelCount)
	for i := 0; i < levelCount; i++ {
		if i == index {
			out[i] = 1.0
		} else {
			out[i] = 0.0
		}
	}
	return out
}

func TestNewBackend_requiresCriterionIDs(t *testing.T) {
	t.Parallel()
	_, err := jev.NewBackend(&stubSystemOneClient{scoreIndex: 1}, nil, jev.Options{})
	require.Error(t, err)
}

func TestBackend_scoresAllCriteria_onRegistryScale(t *testing.T) {
	t.Parallel()
	ids := []criteria.CriterionID{
		criteria.CriterionIDInstructionCompliance,
		criteria.CriterionIDCompleteness,
	}
	backend, err := jev.NewBackend(&stubSystemOneClient{scoreIndex: 2}, ids, jev.Options{ScorePrompt: "Score using checklist."})
	require.NoError(t, err)

	result, err := backend.Generate(context.Background(), scoring.ScoreRequest{
		GeneratorInput:  map[string]any{"version": 1},
		GeneratorOutput: map[string]any{"text": "hello"},
		Feedback:        "[✓] All criteria met.",
		CriterionIDs:    ids,
	})
	require.NoError(t, err)
	assert.InDelta(t, 2.0, result.CriterionScores["instruction_compliance"], 1e-9)
	assert.InDelta(t, 2.0, result.CriterionScores["completeness"], 1e-9)
	assert.NotEmpty(t, result.DirectivesAck)
}

func TestCloneBackend_rebuildsJEV(t *testing.T) {
	t.Parallel()
	ids := []criteria.CriterionID{criteria.CriterionIDOutputQuality}
	original, err := jev.NewBackend(&stubSystemOneClient{scoreIndex: 1}, ids, jev.Options{})
	require.NoError(t, err)
	cloned, err := jev.CloneBackend(original)
	require.NoError(t, err)
	assert.NotSame(t, original, cloned)
}

func TestStub_implementsSystemOneClient(t *testing.T) {
	t.Parallel()
	var _ decide.SystemOneClient = &stubSystemOneClient{}
}
