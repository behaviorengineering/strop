package orchestration

import (
	"context"
	"strings"

	"github.com/behaviorengineering/strop/evaluation"
	"github.com/behaviorengineering/strop/streaming"
)

// PhaseWalkRequest is the input for one multi-field phase attempt.
// LockedOutput contains all prior-phase owned fields that are non-empty.
// OwnedFields lists the field keys that this phase is allowed to write.
// PreviousFailedOutput is the output map from the last failing attempt of this phase (nil on first attempt).
type PhaseWalkRequest struct {
	PhaseID              string
	OwnedFields          []string
	LockedOutput         map[string]string
	PreviousFeedback     string
	PreviousFailedOutput map[string]string
	Version              int
}

// PhaseWalkResponse is the result of one multi-field phase attempt.
// OutputFields must be keyed by owned-field names; only those keys are merged into the draft.
// Eval is optional but recommended for aggregation.
type PhaseWalkResponse struct {
	OutputFields map[string]string
	Passed       bool
	Score        float64
	Feedback     string
	Eval         *evaluation.AggregatedEvaluation
	Rationale    string
}

// PhaseWalkRunner runs one multi-field phase attempt (app-supplied).
type PhaseWalkRunner interface {
	Run(ctx context.Context, req PhaseWalkRequest, eventChan streaming.EventChannel) (*PhaseWalkResponse, error)
}

// PhaseWalkFinalize builds the final CompositionResult after all phases pass.
// draft is the accumulated field map. passedEvals contains one entry per passed phase (in order).
// Return nil, nil to fall back to the default aggregation finalize.
type PhaseWalkFinalize func(draft map[string]string, passedEvals []evaluation.LabeledEval, scores []float64) (*CompositionResult, error)

// PhaseWalkOwnedFields maps PhaseID → list of field keys that phase owns.
// Phases not in the map have no field ownership; they can still pass/fail by runner logic.
type PhaseWalkOwnedFields map[PhaseID][]string

// PhaseWalkConfig is the app-supplied recipe for NewPhaseWalkStrategy.
//
// The runner decides pass/fail, score, and feedback per phase; the strategy handles
// draft lifecycle (merge owned fields on pass, locked-prior-fields snapshot), retry
// wiring (previous failed output + feedback to the next attempt), and final aggregation.
//
// When MergeOnFail is true, owned fields from a failing attempt are merged into the
// draft immediately (same keys as on pass). Failed output is still stored for retry
// wiring. Use this when the runner evaluates against a draft updated before pass/fail
// (e.g. sayings post composition merges generator output before evaluation).
type PhaseWalkConfig struct {
	Phases      []PhaseDef
	OwnedFields PhaseWalkOwnedFields // which fields each phase may write
	Runner      PhaseWalkRunner
	Seed        map[string]string // initial draft (copied; may be nil)
	Version     int               // passed through to the runner in every request
	Finalize    PhaseWalkFinalize // nil → aggregate all passed-phase evals by phase order
	EmptyErr    error             // returned when Result() is called without any passing phase
	MergeOnFail bool              // merge owned fields into draft even when Passed is false
}

// PhaseWalkState is the OutputState returned when no custom Finalize is set.
type PhaseWalkState struct {
	Draft     map[string]string
	Rationale string
	Eval      *evaluation.AggregatedEvaluation
	Scores    []float64
}

type phaseWalkStrategy struct {
	phases      []PhaseDef
	owned       PhaseWalkOwnedFields
	draft       map[string]string
	runner      PhaseWalkRunner
	version     int
	finalize    PhaseWalkFinalize
	emptyErr    error
	mergeOnFail bool

	scores       []float64
	passedEvals  []evaluation.LabeledEval
	rationales   []string

	lastFailed map[string]map[string]string // phaseID → fields from last failing attempt
}

// NewPhaseWalkStrategy builds a CompositionStrategy where each phase can own and write a set of fields.
//
// Prior phases' owned non-empty fields are locked for subsequent phases.
// Failed outputs are passed to the next attempt of the same phase.
// Final result is built by Finalize (or default aggregate if nil).
func NewPhaseWalkStrategy(cfg PhaseWalkConfig) CompositionStrategy {
	if cfg.Runner == nil {
		panic("PhaseWalkRunner cannot be nil")
	}
	if cfg.OwnedFields == nil {
		cfg.OwnedFields = PhaseWalkOwnedFields{}
	}
	draft := make(map[string]string, len(cfg.Seed))
	for k, v := range cfg.Seed {
		draft[k] = v
	}
	return &phaseWalkStrategy{
		phases:      append([]PhaseDef(nil), cfg.Phases...),
		owned:       cfg.OwnedFields,
		draft:       draft,
		runner:      cfg.Runner,
		version:     cfg.Version,
		finalize:    cfg.Finalize,
		emptyErr:    cfg.EmptyErr,
		mergeOnFail: cfg.MergeOnFail,
		lastFailed:  make(map[string]map[string]string),
	}
}

func (s *phaseWalkStrategy) Phases() []PhaseDef {
	return s.phases
}

func (s *phaseWalkStrategy) RunPhase(
	ctx context.Context,
	phase PhaseDef,
	feedback string,
	eventChan streaming.EventChannel,
) (*PhaseResult, error) {
	phaseID := string(phase.ID)
	ownedFields := s.owned[phase.ID]

	req := PhaseWalkRequest{
		PhaseID:              phaseID,
		OwnedFields:          append([]string(nil), ownedFields...),
		LockedOutput:         s.lockedPriorFields(phase.ID),
		PreviousFeedback:     feedback,
		PreviousFailedOutput: s.lastFailed[phaseID],
		Version:              s.version,
	}

	resp, err := s.runner.Run(ctx, req, eventChan)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return &PhaseResult{
			Fields:   nil,
			Score:    0,
			Feedback: "phase runner returned nil response",
			Passed:   false,
		}, nil
	}

	if resp.Passed {
		s.mergeOwnedFields(ownedFields, resp.OutputFields)
		if resp.Eval != nil {
			s.scores = append(s.scores, resp.Score)
			s.passedEvals = append(s.passedEvals, evaluation.LabeledEval{Label: phaseID, Eval: resp.Eval})
		}
		if strings.TrimSpace(resp.Rationale) != "" {
			s.rationales = append(s.rationales, resp.Rationale)
		}
		delete(s.lastFailed, phaseID)
	} else {
		if s.mergeOnFail && len(resp.OutputFields) > 0 {
			s.mergeOwnedFields(ownedFields, resp.OutputFields)
		}
		if len(resp.OutputFields) > 0 {
			s.lastFailed[phaseID] = resp.OutputFields
		}
	}

	return &PhaseResult{
		Fields:   resp.OutputFields,
		Score:    resp.Score,
		Feedback: resp.Feedback,
		Passed:   resp.Passed,
	}, nil
}

// Result is valid after RunCompositionLoop succeeds.
func (s *phaseWalkStrategy) Result() (*CompositionResult, error) {
	draftCopy := make(map[string]string, len(s.draft))
	for k, v := range s.draft {
		draftCopy[k] = v
	}

	if s.finalize != nil {
		res, err := s.finalize(draftCopy, s.passedEvals, s.scores)
		if err != nil {
			return nil, err
		}
		if res != nil {
			return res, nil
		}
	}

	// Default: aggregate all passed-phase evals.
	if len(s.passedEvals) == 0 && len(s.scores) == 0 {
		err := s.emptyErr
		if err == nil {
			err = errPhaseWalkEmpty
		}
		return nil, err
	}
	agg := evaluation.AggregateLabeledEvals(s.scores, s.passedEvals, 0)
	st := &PhaseWalkState{
		Draft:     draftCopy,
		Rationale: strings.Join(s.rationales, "\n"),
		Eval:      agg,
		Scores:    append([]float64(nil), s.scores...),
	}
	return &CompositionResult{
		Score:       agg.WeightedScore,
		Feedback:    agg.ConsolidatedFeedback,
		OutputState: st,
		EvalPayload: agg,
	}, nil
}

// lockedPriorFields returns non-empty owned fields from all phases that come before activeID.
func (s *phaseWalkStrategy) mergeOwnedFields(ownedFields []string, outputFields map[string]string) {
	if len(outputFields) == 0 {
		return
	}
	ownedSet := make(map[string]struct{}, len(ownedFields))
	for _, f := range ownedFields {
		ownedSet[f] = struct{}{}
	}
	for k, v := range outputFields {
		if len(ownedSet) == 0 {
			s.draft[k] = v
		} else if _, owned := ownedSet[k]; owned {
			s.draft[k] = v
		}
	}
}

// lockedPriorFields returns non-empty owned fields from all phases that come before activeID.
func (s *phaseWalkStrategy) lockedPriorFields(activeID PhaseID) map[string]string {
	locked := make(map[string]string)
	for _, phase := range s.phases {
		if phase.ID == activeID {
			break
		}
		for _, field := range s.owned[phase.ID] {
			if v := strings.TrimSpace(s.draft[field]); v != "" {
				locked[field] = v
			}
		}
	}
	return locked
}

var errPhaseWalkEmpty = errorf("phase walk composition finished without evaluation")

func errorf(msg string) error {
	return &phaseWalkError{msg: msg}
}

type phaseWalkError struct{ msg string }

func (e *phaseWalkError) Error() string { return e.msg }
