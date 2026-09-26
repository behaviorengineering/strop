package scoring

import (
	"context"

	"github.com/behaviorengineering/strop/pkg/evaluation/criteria"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
)

// ScoreRequest is input for a criterion score generation backend (LLM or JEV).
type ScoreRequest struct {
	GeneratorInput  map[string]any
	GeneratorOutput map[string]any
	Feedback        string
	CriterionIDs    []criteria.CriterionID
	Rubric          map[criteria.CriterionID]string
	ScorePrompt     string
	// ModuleOptions are passed to LLM score modules only (XML interceptors, tracing).
	ModuleOptions []core.Option
}

// ScoreResult is the normalized output of score generation.
type ScoreResult struct {
	CriterionScores map[string]float64
	DirectivesAck   string
}

// Backend generates per-criterion scores from generator context and feedback.
type Backend interface {
	Generate(ctx context.Context, req ScoreRequest, opts ...core.Option) (ScoreResult, error)
}

// ScoreModuleProvider is implemented by LLM-backed score backends so factory setup can wire LLM/XML.
type ScoreModuleProvider interface {
	Backend
	ScoreModule() core.Module
}
