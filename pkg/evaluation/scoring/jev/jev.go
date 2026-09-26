package jev

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/behaviorengineering/strop/pkg/evaluation/criteria"
	"github.com/behaviorengineering/strop/pkg/evaluation/scoring"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
	"github.com/XiaoConstantine/dspy-go/pkg/experimental/decide"
	"github.com/XiaoConstantine/dspy-go/pkg/experimental/typesafe"
)

// Options configures a JEV score backend.
type Options struct {
	ScorePrompt string
	Rubric      map[criteria.CriterionID]string
}

// jevBackend scores criteria via decide.Decide and TypeSafe System One.
type jevBackend struct {
	module       *decide.Decide
	client       decide.SystemOneClient
	criterionIDs []criteria.CriterionID
	maxPoints    map[criteria.CriterionID]float64
	opts         Options
}

// NewBackend builds a scoring.Backend for the given criterion IDs.
// When client is nil, typesafe.NewClient() is used (TYPESAFE_API_KEY, etc.).
func NewBackend(client decide.SystemOneClient, criterionIDs []criteria.CriterionID, opts Options) (scoring.Backend, error) {
	if len(criterionIDs) == 0 {
		return nil, fmt.Errorf("jev: at least one criterion ID is required")
	}
	seen := make(map[criteria.CriterionID]struct{}, len(criterionIDs))
	for _, id := range criterionIDs {
		if id == "" {
			return nil, fmt.Errorf("jev: empty criterion ID")
		}
		if _, dup := seen[id]; dup {
			return nil, fmt.Errorf("jev: duplicate criterion ID %q", id)
		}
		seen[id] = struct{}{}
	}

	if client == nil {
		c, err := typesafe.NewClient()
		if err != nil {
			return nil, fmt.Errorf("jev: typesafe client: %w", err)
		}
		client = c
	}

	registry := criteria.DefaultRegistry()
	decideOutputs := make([]decide.Output, 0, len(criterionIDs))
	outputFields := make([]core.OutputField, 0, len(criterionIDs))
	maxPoints := make(map[criteria.CriterionID]float64, len(criterionIDs))
	for _, id := range criterionIDs {
		name := string(id)
		crit, err := registry.Get(id)
		if err != nil {
			crit = criteria.CriterionDescription{ID: id, Name: name, Description: name, MaxPoints: 2}
		}
		if crit.MaxPoints <= 0 {
			crit.MaxPoints = 2
		}
		maxPoints[id] = crit.MaxPoints
		levels := scoreLevelsForCriterion(crit, opts.Rubric[id], opts.ScorePrompt)
		decideOutputs = append(decideOutputs, decide.Score(name, levels...))
		outputFields = append(outputFields, core.OutputField{
			Field: core.NewField(name, core.WithNoPrefix(), core.WithDescription(crit.Name)),
		})
	}

	instruction := strings.TrimSpace(opts.ScorePrompt)
	if instruction == "" {
		instruction = "Score each criterion using the feedback checklist and generator output. Use each criterion's rubric anchors."
	}

	signature := core.NewSignature(
		[]core.InputField{
			{Field: core.NewField("generator_input", core.WithNoPrefix(), core.WithDescription("What the generator received"))},
			{Field: core.NewField("generator_output", core.WithNoPrefix(), core.WithDescription("What the generator produced"))},
			{Field: core.NewField("feedback", core.WithNoPrefix(), core.WithDescription("Checklist feedback from feedback analysis"))},
		},
		outputFields,
	).WithInstruction(instruction)

	module, err := decide.New(client, signature, decideOutputs...)
	if err != nil {
		return nil, fmt.Errorf("jev: decide module: %w", err)
	}
	module = module.WithName("JEV Score Generation")

	return &jevBackend{
		module:       module,
		client:       client,
		criterionIDs: append([]criteria.CriterionID(nil), criterionIDs...),
		maxPoints:    maxPoints,
		opts:         opts,
	}, nil
}

func (b *jevBackend) Generate(ctx context.Context, req scoring.ScoreRequest, _ ...core.Option) (scoring.ScoreResult, error) {
	if b == nil || b.module == nil {
		return scoring.ScoreResult{}, fmt.Errorf("jev: backend is not configured")
	}
	if strings.TrimSpace(req.Feedback) == "" {
		return scoring.ScoreResult{}, fmt.Errorf("jev: feedback is required")
	}

	inputs := map[string]any{
		"generator_input":  marshalInput(req.GeneratorInput),
		"generator_output": marshalInput(req.GeneratorOutput),
		"feedback":         req.Feedback,
	}

	result, err := b.module.ProcessDecision(ctx, inputs)
	if err != nil {
		return scoring.ScoreResult{}, fmt.Errorf("jev: %w", err)
	}

	scores := make(map[string]float64, len(b.criterionIDs))
	var ackParts []string
	for _, id := range b.criterionIDs {
		name := string(id)
		value, ok := result.Outputs[name]
		if !ok {
			return scoring.ScoreResult{}, fmt.Errorf("jev: missing score for criterion %q", name)
		}
		score, ok := asFloat64(value)
		if !ok {
			return scoring.ScoreResult{}, fmt.Errorf("jev: criterion %q score is not numeric", name)
		}
		score = clampScore(score, b.maxPoints[id])
		scores[name] = score
		ackParts = append(ackParts, fmt.Sprintf("%s=%.2f", name, score))
	}

	ack := "JEV score backend (typesafe/decide): " + strings.Join(ackParts, ", ")

	return scoring.ScoreResult{
		CriterionScores: scores,
		DirectivesAck:   ack,
	}, nil
}

func marshalInput(value map[string]any) string {
	if len(value) == 0 {
		return ""
	}
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(data)
}

func asFloat64(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	default:
		return 0, false
	}
}
