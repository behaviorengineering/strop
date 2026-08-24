package orchestration

import (
	"context"
	"fmt"

	"github.com/behaviorengineering/strop/evaluation"
	"github.com/behaviorengineering/strop/streaming"
)

// SectionFieldRequest is the input for one non-empty section generate+evaluate attempt.
type SectionFieldRequest struct {
	SectionID            string
	SourceText           string
	LockedOutput         map[string]string
	PreviousFeedback     string
	PreviousFailedOutput string
	Version              int
}

// SectionFieldResponse is the generate+evaluate result for one section attempt.
type SectionFieldResponse struct {
	OutputText string
	Rationale  string
	Eval       *evaluation.AggregatedEvaluation
}

// SectionFieldRunner runs generate+evaluate for one section field (app-supplied).
type SectionFieldRunner interface {
	Run(ctx context.Context, req SectionFieldRequest, eventChan streaming.EventChannel) (*SectionFieldResponse, error)
}

// SectionCodec maps a typed draft to and from the string map used by field-walk.
type SectionCodec[T any] struct {
	ToMap      func(T) map[string]string
	FromMap    func(map[string]string) T
	SourceText func(draft T, sectionID string) string // Optional; default reads from ToMap draft.
}

// SectionWalkState is OutputState after a successful section walk.
type SectionWalkState[T any] struct {
	Draft     T
	Rationale string
	Eval      *evaluation.AggregatedEvaluation
	Scores    []float64
}

// SectionWalkConfig is the app-supplied recipe for NewSectionWalkStrategy.
type SectionWalkConfig[T any] struct {
	Sections        DocumentSectionDefinition
	Seed            T
	Version         int
	VersionFeedback string
	Runner          SectionFieldRunner
	Codec           SectionCodec[T]
	EmptyResultErr  error
}

type sectionWalkStrategy[T any] struct {
	inner CompositionStrategy
	from  func(map[string]string) T
	empty error
}

// NewSectionWalkStrategy builds a CompositionStrategy that walks ordered sections on a typed draft.
// Delegates to NewFieldWalkStrategy; pass/fail and draft lifecycle stay in field-walk.
func NewSectionWalkStrategy[T any](cfg SectionWalkConfig[T]) CompositionStrategy {
	if cfg.Runner == nil {
		panic("SectionFieldRunner cannot be nil")
	}
	if cfg.Codec.ToMap == nil || cfg.Codec.FromMap == nil {
		panic("SectionCodec ToMap and FromMap cannot be nil")
	}
	emptyErr := cfg.EmptyResultErr
	if emptyErr == nil {
		emptyErr = errSectionWalkEmpty
	}
	codec := cfg.Codec
	sourceText := codec.SourceText
	if sourceText == nil {
		sourceText = func(draft T, sectionID string) string {
			m := codec.ToMap(draft)
			if m == nil {
				return ""
			}
			return m[sectionID]
		}
	}
	inner := NewFieldWalkStrategy(FieldWalkConfig{
		Phases:          cfg.Sections.PhaseDefs(),
		MinPassScore:    cfg.Sections.MinPass(),
		Version:         cfg.Version,
		VersionFeedback: cfg.VersionFeedback,
		Seed:            codec.ToMap(cfg.Seed),
		EmptyResultErr:  emptyErr,
		SourceText: func(draft map[string]string, fieldID string) string {
			return sourceText(codec.FromMap(draft), fieldID)
		},
		Runner: sectionFieldRunnerAdapter{runner: cfg.Runner},
	})
	return &sectionWalkStrategy[T]{
		inner: inner,
		from:  codec.FromMap,
		empty: emptyErr,
	}
}

type sectionFieldRunnerAdapter struct {
	runner SectionFieldRunner
}

func (a sectionFieldRunnerAdapter) Run(
	ctx context.Context,
	req FieldPhaseRequest,
	eventChan streaming.EventChannel,
) (*FieldPhaseResponse, error) {
	resp, err := a.runner.Run(ctx, SectionFieldRequest{
		SectionID:            req.FieldID,
		SourceText:           req.SourceText,
		LockedOutput:         req.LockedOutput,
		PreviousFeedback:     req.PreviousFeedback,
		PreviousFailedOutput: req.PreviousFailedOutput,
		Version:              req.Version,
	}, eventChan)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, nil
	}
	return &FieldPhaseResponse{
		OutputText: resp.OutputText,
		Rationale:  resp.Rationale,
		Eval:       resp.Eval,
	}, nil
}

func (s *sectionWalkStrategy[T]) Phases() []PhaseDef {
	return s.inner.Phases()
}

func (s *sectionWalkStrategy[T]) RunPhase(
	ctx context.Context,
	phase PhaseDef,
	feedback string,
	eventChan streaming.EventChannel,
) (*PhaseResult, error) {
	return s.inner.RunPhase(ctx, phase, feedback, eventChan)
}

func (s *sectionWalkStrategy[T]) Result() (*CompositionResult, error) {
	res, err := s.inner.Result()
	if err != nil {
		return nil, err
	}
	if res == nil {
		return nil, s.empty
	}
	st, ok := res.OutputState.(*FieldWalkState)
	if !ok || st == nil {
		return nil, s.empty
	}
	res.OutputState = &SectionWalkState[T]{
		Draft:     s.from(st.Draft),
		Rationale: st.Rationale,
		Eval:      st.Eval,
		Scores:    st.Scores,
	}
	return res, nil
}

var errSectionWalkEmpty = fmt.Errorf("section walk composition finished without evaluation")
