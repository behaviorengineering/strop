package orchestration

import (
	"context"
	"fmt"
	"strings"

	"github.com/behaviorengineering/strop/evaluation"
	"github.com/behaviorengineering/strop/streaming"
)

// FieldPhaseRequest is the input for one field generate+evaluate attempt.
type FieldPhaseRequest struct {
	FieldID              string
	SourceText           string
	LockedOutput         map[string]string
	PreviousFeedback     string
	PreviousFailedOutput string
	Version              int
}

// FieldPhaseResponse is the generate+evaluate result for one field attempt.
type FieldPhaseResponse struct {
	OutputText string
	Rationale  string
	Eval       *evaluation.AggregatedEvaluation
}

// FieldPhaseRunner runs generate+evaluate for one field (app-supplied).
type FieldPhaseRunner interface {
	Run(ctx context.Context, req FieldPhaseRequest, eventChan streaming.EventChannel) (*FieldPhaseResponse, error)
}

// FieldSourceTextFunc returns the text to transform for a field from the working draft.
type FieldSourceTextFunc func(draft map[string]string, fieldID string) string

// FieldWalkConfig is the app-supplied recipe for NewFieldWalkStrategy.
type FieldWalkConfig struct {
	Phases          []PhaseDef
	MinPassScore    float64
	SourceText      FieldSourceTextFunc
	Runner          FieldPhaseRunner
	EmptyResultErr  error
	Version         int
	VersionFeedback string
	Seed            map[string]string
}

// FieldWalkState is OutputState after a successful field walk.
type FieldWalkState struct {
	Draft     map[string]string
	Rationale string
	Eval      *evaluation.AggregatedEvaluation
	Scores    []float64
}

type fieldWalkStrategy struct {
	phases                 []PhaseDef
	draft                  map[string]string
	version                int
	versionFeedback        string
	versionFeedbackApplied bool
	sourceText             FieldSourceTextFunc
	runner                 FieldPhaseRunner
	minPassScore           float64
	emptyResultErr         error

	fieldScores      []float64
	passedFieldEvals []evaluation.LabeledEval
	rationales       []string
	lastFailedOutput string
}

// NewFieldWalkStrategy builds a CompositionStrategy that walks ordered fields on a string draft.
// Empty source fields auto-pass. Prior fields only are locked. Version feedback is applied once.
func NewFieldWalkStrategy(cfg FieldWalkConfig) CompositionStrategy {
	if cfg.Runner == nil {
		panic("FieldPhaseRunner cannot be nil")
	}
	if cfg.EmptyResultErr == nil {
		cfg.EmptyResultErr = errFieldWalkEmpty
	}
	if cfg.SourceText == nil {
		cfg.SourceText = func(draft map[string]string, fieldID string) string {
			if draft == nil {
				return ""
			}
			return draft[fieldID]
		}
	}
	draft := make(map[string]string, len(cfg.Seed))
	for k, v := range cfg.Seed {
		draft[k] = v
	}
	return &fieldWalkStrategy{
		phases:          append([]PhaseDef(nil), cfg.Phases...),
		draft:           draft,
		version:         cfg.Version,
		versionFeedback: cfg.VersionFeedback,
		sourceText:      cfg.SourceText,
		runner:          cfg.Runner,
		minPassScore:    cfg.MinPassScore,
		emptyResultErr:  cfg.EmptyResultErr,
	}
}

func (s *fieldWalkStrategy) Phases() []PhaseDef {
	return s.phases
}

func (s *fieldWalkStrategy) RunPhase(
	ctx context.Context,
	phase PhaseDef,
	feedback string,
	eventChan streaming.EventChannel,
) (*PhaseResult, error) {
	fieldID := string(phase.ID)
	sourceText := s.sourceText(s.draft, fieldID)
	if strings.TrimSpace(sourceText) == "" {
		s.draft[fieldID] = ""
		return &PhaseResult{
			Fields:   map[string]string{fieldID: ""},
			Score:    s.minPassScore,
			Feedback: "",
			Passed:   true,
		}, nil
	}

	phaseFeedback := feedback
	if phaseFeedback == "" && !s.versionFeedbackApplied && strings.TrimSpace(s.versionFeedback) != "" {
		phaseFeedback = s.versionFeedback
		s.versionFeedbackApplied = true
	}

	resp, err := s.runner.Run(ctx, FieldPhaseRequest{
		FieldID:              fieldID,
		SourceText:           sourceText,
		LockedOutput:         lockedPriorFields(s.phases, s.draft, fieldID),
		PreviousFeedback:     phaseFeedback,
		PreviousFailedOutput: s.lastFailedOutput,
		Version:              s.version,
	}, eventChan)
	if err != nil {
		return nil, err
	}
	if resp == nil || resp.Eval == nil {
		return nil, s.emptyResultErr
	}

	score := resp.Eval.WeightedScore
	passed := score >= s.minPassScore && strings.TrimSpace(resp.OutputText) != ""
	if passed {
		s.draft[fieldID] = resp.OutputText
		s.fieldScores = append(s.fieldScores, score)
		s.passedFieldEvals = append(s.passedFieldEvals, evaluation.LabeledEval{
			Label: fieldID,
			Eval:  resp.Eval,
		})
		if strings.TrimSpace(resp.Rationale) != "" {
			s.rationales = append(s.rationales, resp.Rationale)
		}
		s.lastFailedOutput = ""
	} else {
		s.lastFailedOutput = resp.OutputText
	}

	return &PhaseResult{
		Fields:   map[string]string{fieldID: resp.OutputText},
		Score:    score,
		Feedback: resp.Eval.ConsolidatedFeedback,
		Passed:   passed,
	}, nil
}

func (s *fieldWalkStrategy) Result() (*CompositionResult, error) {
	agg := evaluation.AggregateLabeledEvals(s.fieldScores, s.passedFieldEvals, s.minPassScore)
	if agg == nil {
		if s.emptyResultErr != nil {
			return nil, s.emptyResultErr
		}
		return nil, errFieldWalkEmpty
	}
	draftCopy := make(map[string]string, len(s.draft))
	for k, v := range s.draft {
		draftCopy[k] = v
	}
	st := &FieldWalkState{
		Draft:     draftCopy,
		Rationale: strings.Join(s.rationales, "\n"),
		Eval:      agg,
		Scores:    append([]float64(nil), s.fieldScores...),
	}
	return &CompositionResult{
		Score:       agg.WeightedScore,
		Feedback:    agg.ConsolidatedFeedback,
		OutputState: st,
		EvalPayload: agg,
	}, nil
}

func lockedPriorFields(phases []PhaseDef, draft map[string]string, activeID string) map[string]string {
	out := make(map[string]string)
	for _, phase := range phases {
		id := string(phase.ID)
		if id == activeID {
			break
		}
		if v := strings.TrimSpace(draft[id]); v != "" {
			out[id] = v
		}
	}
	return out
}

var errFieldWalkEmpty = fmt.Errorf("field walk composition finished without evaluation")
