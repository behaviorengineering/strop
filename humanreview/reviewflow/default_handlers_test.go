package reviewflow

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/behaviorengineering/strop/evaluation/criteria"
	"github.com/behaviorengineering/strop/humanreview"
	kitlog "github.com/behaviorengineering/strop/log"
	"github.com/behaviorengineering/strop/regenerate"
)

type silentLog struct{}

func (silentLog) WithField(string, interface{}) kitlog.Logger     { return silentLog{} }
func (silentLog) WithFields(map[string]interface{}) kitlog.Logger { return silentLog{} }
func (silentLog) WithError(error) kitlog.Logger                   { return silentLog{} }
func (silentLog) Debug(...interface{})                            {}
func (silentLog) Info(...interface{})                             {}
func (silentLog) Warn(...interface{})                             {}
func (silentLog) Error(...interface{})                            {}

type memStore struct {
	mu    sync.Mutex
	byID  map[uuid.UUID]*humanreview.HumanEvaluation
	byKey map[string]uuid.UUID
}

func newMemStore() *memStore {
	return &memStore{byID: map[uuid.UUID]*humanreview.HumanEvaluation{}, byKey: map[string]uuid.UUID{}}
}

func storeKey(root uuid.UUID, pipeline humanreview.PipelineType, job humanreview.Job) string {
	return root.String() + "|" + string(pipeline) + "|" + string(job)
}

func cloneEval(e *humanreview.HumanEvaluation) *humanreview.HumanEvaluation {
	if e == nil {
		return nil
	}
	cp := *e
	return &cp
}

func (s *memStore) Create(_ context.Context, evaluation *humanreview.HumanEvaluation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byID[evaluation.ID] = cloneEval(evaluation)
	s.byKey[storeKey(evaluation.RootEntityID, evaluation.PipelineType, evaluation.Job)] = evaluation.ID
	return nil
}

func (s *memStore) GetByID(_ context.Context, id uuid.UUID) (*humanreview.HumanEvaluation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneEval(s.byID[id]), nil
}

func (s *memStore) GetByRootEntityID(_ context.Context, root uuid.UUID, pipeline humanreview.PipelineType, job humanreview.Job) (*humanreview.HumanEvaluation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.byKey[storeKey(root, pipeline, job)]
	if !ok {
		return nil, nil
	}
	return cloneEval(s.byID[id]), nil
}

func (s *memStore) Update(_ context.Context, evaluation *humanreview.HumanEvaluation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byID[evaluation.ID] = cloneEval(evaluation)
	s.byKey[storeKey(evaluation.RootEntityID, evaluation.PipelineType, evaluation.Job)] = evaluation.ID
	return nil
}

func (s *memStore) DeleteByRootEntityID(_ context.Context, root uuid.UUID, pipeline humanreview.PipelineType, job humanreview.Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := storeKey(root, pipeline, job)
	id, ok := s.byKey[key]
	if !ok {
		return nil
	}
	delete(s.byID, id)
	delete(s.byKey, key)
	return nil
}

type fakePorts struct {
	agrees          bool
	comment         string
	alignCalls      int
	generated       int
	regenerated     int
	eval            *humanreview.HumanEvaluation
	evalAfterRecord *humanreview.HumanEvaluation
	proposedEval    *humanreview.HumanEvaluation
	approved        bool
	autoAgreed      int
	recorded        bool
	regenMode       bool
	regenModeCalls  int
	lastRegenOpts   regenerate.RegenerateOptions
}

func (f *fakePorts) PromptAlignment(context.Context, *humanreview.HumanEvaluation) (bool, string, error) {
	f.alignCalls++
	if f.alignCalls == 1 {
		return f.agrees, f.comment, nil
	}
	return true, "", nil
}
func (f *fakePorts) PromptRegenMode(context.Context) (bool, error) {
	f.regenModeCalls++
	return f.regenMode, nil
}
func (f *fakePorts) Generate(_ context.Context, root uuid.UUID, _ humanreview.Job) (*GenerateResult, error) {
	f.generated++
	return &GenerateResult{RootID: root, ContentID: uuid.New()}, nil
}
func (f *fakePorts) Regenerate(_ context.Context, root uuid.UUID, _ humanreview.Job, opts regenerate.RegenerateOptions) (*GenerateResult, error) {
	f.regenerated++
	f.lastRegenOpts = opts
	return &GenerateResult{RootID: root, ContentID: uuid.New()}, nil
}
func (f *fakePorts) StartEvaluation(_ context.Context, root uuid.UUID, job humanreview.Job, _ []criteria.CriterionID) (*humanreview.HumanEvaluation, error) {
	return f.eval, nil
}
func (f *fakePorts) Refresh(context.Context, uuid.UUID) (*humanreview.HumanEvaluation, error) {
	if f.recorded && f.evalAfterRecord != nil {
		return f.evalAfterRecord, nil
	}
	return f.eval, nil
}
func (f *fakePorts) RecordAlignment(context.Context, uuid.UUID, bool, string) error {
	f.recorded = true
	return nil
}
func (f *fakePorts) ProposeCriteria(_ context.Context, eval *humanreview.HumanEvaluation, _ bool, _ string) error {
	f.proposedEval = eval
	return nil
}
func (f *fakePorts) AutoAgreePending(context.Context, uuid.UUID) error {
	f.autoAgreed++
	return nil
}
func (f *fakePorts) ApprovePendingContent(context.Context, *humanreview.HumanEvaluation) error {
	f.approved = true
	return nil
}

type fakeLearner struct {
	calls int
	err   error
}

func (f *fakeLearner) AfterApproval(context.Context, *humanreview.HumanEvaluation) error {
	f.calls++
	return f.err
}

func TestDefaultHandlers_nilLearnerStillApproves(t *testing.T) {
	humanreview.DefaultJobStepRegistry().Register("kit_job_nil_learner", "kit_step_nil_learner")
	root := uuid.New()
	eval := &humanreview.HumanEvaluation{
		ID:           uuid.New(),
		RootEntityID: root,
		Job:          "kit_job_nil_learner",
		PipelineType: "demo",
		Status:       humanreview.StatusInProgress,
	}
	fp := &fakePorts{agrees: true, eval: eval}
	run := &RunState{
		RootID:       root,
		Job:          "kit_job_nil_learner",
		CriterionIDs: []criteria.CriterionID{"quality"},
	}
	e := NewEngine(Config{Start: StateInit})
	RegisterDefaultHandlers(e, Ports{
		Prompter:     fp,
		Generator:    fp,
		Session:      fp,
		Gate:         humanreview.NewGate(newMemStore(), silentLog{}),
		PipelineType: "demo",
	}, run)
	require.NoError(t, e.Run(context.Background()))
	assert.True(t, fp.approved)
}

func TestDefaultHandlers_unknownPackSkipsLearner(t *testing.T) {
	humanreview.DefaultJobStepRegistry().Register("kit_job_unknown_pack", "kit_step_unknown_pack")
	root := uuid.New()
	eval := &humanreview.HumanEvaluation{
		ID:           uuid.New(),
		RootEntityID: root,
		Job:          "kit_job_unknown_pack",
		PipelineType: "demo_unknown",
		Status:       humanreview.StatusInProgress,
	}
	fp := &fakePorts{agrees: true, eval: eval}
	learner := &fakeLearner{}
	run := &RunState{
		RootID:       root,
		Job:          "kit_job_unknown_pack",
		CriterionIDs: []criteria.CriterionID{"quality"},
	}
	e := NewEngine(Config{Start: StateInit})
	RegisterDefaultHandlers(e, Ports{
		Prompter:     fp,
		Generator:    fp,
		Session:      fp,
		Gate:         humanreview.NewGate(newMemStore(), silentLog{}),
		PipelineType: "demo_unknown",
		Learner:      learner,
		Packs:        humanreview.NewLearningPackRegistry(),
	}, run)
	require.NoError(t, e.Run(context.Background()))
	assert.True(t, fp.approved)
	assert.Equal(t, 0, learner.calls)
}

func TestDefaultHandlers_compositionJobCallsLearner(t *testing.T) {
	humanreview.DefaultJobStepRegistry().Register("compose_job", "compose_step")
	root := uuid.New()
	eval := &humanreview.HumanEvaluation{
		ID:           uuid.New(),
		RootEntityID: root,
		Job:          "compose_job",
		PipelineType: "demo",
		Status:       humanreview.StatusInProgress,
	}
	fp := &fakePorts{agrees: true, eval: eval}
	learner := &fakeLearner{err: assert.AnError}
	packs := humanreview.NewLearningPackRegistry()
	packs.Register(humanreview.NewStaticLearningPack("demo", "compose_job"))
	run := &RunState{
		RootID:       root,
		Job:          "compose_job",
		CriterionIDs: []criteria.CriterionID{"quality"},
	}
	e := NewEngine(Config{Start: StateInit})
	RegisterDefaultHandlers(e, Ports{
		Prompter:     fp,
		Generator:    fp,
		Session:      fp,
		Gate:         humanreview.NewGate(newMemStore(), silentLog{}),
		PipelineType: "demo",
		Learner:      learner,
		Packs:        packs,
	}, run)
	require.NoError(t, e.Run(context.Background()))
	assert.True(t, fp.approved)
	assert.Equal(t, 1, learner.calls)
}

func TestDefaultHandlers_nonCompositionJobSkipsLearner(t *testing.T) {
	humanreview.DefaultJobStepRegistry().Register("other_job", "other_step")
	root := uuid.New()
	eval := &humanreview.HumanEvaluation{
		ID:           uuid.New(),
		RootEntityID: root,
		Job:          "other_job",
		PipelineType: "demo",
		Status:       humanreview.StatusInProgress,
	}
	fp := &fakePorts{agrees: true, eval: eval}
	learner := &fakeLearner{}
	packs := humanreview.NewLearningPackRegistry()
	packs.Register(humanreview.NewStaticLearningPack("demo", "compose_job"))
	run := &RunState{
		RootID:       root,
		Job:          "other_job",
		CriterionIDs: []criteria.CriterionID{"quality"},
	}
	e := NewEngine(Config{Start: StateInit})
	RegisterDefaultHandlers(e, Ports{
		Prompter:     fp,
		Generator:    fp,
		Session:      fp,
		Gate:         humanreview.NewGate(newMemStore(), silentLog{}),
		PipelineType: "demo",
		Learner:      learner,
		Packs:        packs,
	}, run)
	require.NoError(t, e.Run(context.Background()))
	assert.True(t, fp.approved)
	assert.Equal(t, 0, learner.calls)
}

func TestDefaultHandlers_agreePath(t *testing.T) {
	humanreview.DefaultJobStepRegistry().Register("kit_job", "kit_step")
	root := uuid.New()
	eval := &humanreview.HumanEvaluation{
		ID:           uuid.New(),
		RootEntityID: root,
		Job:          "kit_job",
		PipelineType: "demo",
		Status:       humanreview.StatusInProgress,
	}
	fp := &fakePorts{agrees: true, eval: eval}
	run := &RunState{
		RootID:       root,
		Job:          "kit_job",
		CriterionIDs: []criteria.CriterionID{"quality"},
	}
	e := NewEngine(Config{Start: StateInit})
	RegisterDefaultHandlers(e, Ports{
		Prompter:     fp,
		Generator:    fp,
		Session:      fp,
		Gate:         humanreview.NewGate(newMemStore(), silentLog{}),
		PipelineType: "demo",
	}, run)
	require.NoError(t, e.Run(context.Background()))
	assert.Equal(t, 1, fp.generated)
	assert.Equal(t, 0, fp.regenerated)
	assert.Equal(t, 1, fp.autoAgreed)
	assert.True(t, fp.approved)
	assert.Equal(t, eval.ID, run.EvaluationID)
}

func TestDefaultHandlers_disagreeThenRegen(t *testing.T) {
	humanreview.DefaultJobStepRegistry().Register("kit_job2", "kit_step2")
	root := uuid.New()
	store := newMemStore()
	gate := humanreview.NewGate(store, silentLog{})
	eval, err := gate.Start(context.Background(), "demo", root, "kit_job2")
	require.NoError(t, err)

	fp := &fakePorts{agrees: false, comment: "rewrite", eval: eval}
	run := &RunState{
		RootID:       root,
		Job:          "kit_job2",
		CriterionIDs: []criteria.CriterionID{"quality"},
		EvaluationID: eval.ID,
	}
	e := NewEngine(Config{Start: StateAlignment})
	RegisterDefaultHandlers(e, Ports{
		Prompter:     fp,
		Generator:    fp,
		Session:      fp,
		Gate:         gate,
		Normalizer:   humanreview.PassthroughNormalizer{},
		PipelineType: "demo",
	}, run)
	require.NoError(t, e.Run(context.Background()))
	assert.Equal(t, 1, fp.regenerated)
	assert.Equal(t, 1, fp.autoAgreed)
	assert.True(t, fp.approved)
	assert.Equal(t, "rewrite", run.LastStructuredFeedback)
	assert.Equal(t, 1, fp.regenModeCalls)
	assert.False(t, run.UseResearch)
	assert.False(t, fp.lastRegenOpts.Research)
}

type capturingNormalizer struct {
	research bool
}

func (c *capturingNormalizer) Normalize(ctx context.Context, comment string) (string, error) {
	c.research = regenerate.ResearchModeFromContext(ctx)
	return comment, nil
}

func TestDefaultHandlers_emptyCommentSkipsRegenModePrompt(t *testing.T) {
	humanreview.DefaultJobStepRegistry().Register("kit_job_empty_comment", "kit_step_empty_comment")
	root := uuid.New()
	store := newMemStore()
	gate := humanreview.NewGate(store, silentLog{})
	eval, err := gate.Start(context.Background(), "demo", root, "kit_job_empty_comment")
	require.NoError(t, err)

	fp := &fakePorts{agrees: false, comment: "", eval: eval}
	run := &RunState{
		RootID:       root,
		Job:          "kit_job_empty_comment",
		CriterionIDs: []criteria.CriterionID{"quality"},
		EvaluationID: eval.ID,
	}
	e := NewEngine(Config{Start: StateAlignment})
	RegisterDefaultHandlers(e, Ports{
		Prompter:     fp,
		Generator:    fp,
		Session:      fp,
		Gate:         gate,
		PipelineType: "demo",
	}, run)
	require.NoError(t, e.Run(context.Background()))
	assert.Equal(t, 0, fp.regenModeCalls)
	assert.Empty(t, run.LastStructuredFeedback)
	assert.False(t, run.UseResearch)
	assert.Equal(t, 1, fp.regenerated)
}

func TestDefaultHandlers_researchFlagPassedToRegen(t *testing.T) {
	humanreview.DefaultJobStepRegistry().Register("kit_job_research", "kit_step_research")
	root := uuid.New()
	store := newMemStore()
	gate := humanreview.NewGate(store, silentLog{})
	eval, err := gate.Start(context.Background(), "demo", root, "kit_job_research")
	require.NoError(t, err)

	normalizer := &capturingNormalizer{}
	fp := &fakePorts{agrees: false, comment: "rewrite", regenMode: true, eval: eval}
	run := &RunState{
		RootID:       root,
		Job:          "kit_job_research",
		CriterionIDs: []criteria.CriterionID{"quality"},
		EvaluationID: eval.ID,
	}
	e := NewEngine(Config{Start: StateAlignment})
	RegisterDefaultHandlers(e, Ports{
		Prompter:     fp,
		Generator:    fp,
		Session:      fp,
		Gate:         gate,
		Normalizer:   normalizer,
		PipelineType: "demo",
	}, run)
	require.NoError(t, e.Run(context.Background()))
	assert.Equal(t, 1, fp.regenModeCalls)
	assert.True(t, run.UseResearch)
	assert.True(t, fp.lastRegenOpts.Research)
	assert.True(t, normalizer.research)
	assert.Equal(t, "rewrite", fp.lastRegenOpts.Message)
}

func TestDefaultHandlers_proposeUsesRefreshedEval(t *testing.T) {
	humanreview.DefaultJobStepRegistry().Register("kit_job_refresh", "kit_step_refresh")
	root := uuid.New()
	eval := &humanreview.HumanEvaluation{
		ID:           uuid.New(),
		RootEntityID: root,
		Job:          "kit_job_refresh",
		PipelineType: "demo",
		Status:       humanreview.StatusInProgress,
	}
	after := cloneEval(eval)
	after.HumanAlignmentComment = "after-record"
	fp := &fakePorts{agrees: true, eval: eval, evalAfterRecord: after}
	run := &RunState{
		RootID:       root,
		Job:          "kit_job_refresh",
		CriterionIDs: []criteria.CriterionID{"quality"},
		EvaluationID: eval.ID,
	}
	e := NewEngine(Config{Start: StateAlignment})
	RegisterDefaultHandlers(e, Ports{
		Prompter:  fp,
		Generator: fp,
		Session:   fp,
	}, run)
	require.NoError(t, e.Run(context.Background()))
	require.NotNil(t, fp.proposedEval)
	assert.Equal(t, "after-record", fp.proposedEval.HumanAlignmentComment)
}

func TestDefaultHandlers_initRequiresSessionWhenEvaluationSet(t *testing.T) {
	run := &RunState{EvaluationID: uuid.New(), RootID: uuid.New()}
	e := NewEngine(Config{Start: StateInit})
	RegisterDefaultHandlers(e, Ports{}, run)
	err := e.Run(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "session port is required")
}

type missingAfterResetStore struct {
	mu        sync.Mutex
	eval      *humanreview.HumanEvaluation
	resetDone bool
}

func (s *missingAfterResetStore) Create(_ context.Context, evaluation *humanreview.HumanEvaluation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.eval = cloneEval(evaluation)
	return nil
}

func (s *missingAfterResetStore) GetByID(context.Context, uuid.UUID) (*humanreview.HumanEvaluation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.resetDone {
		return nil, nil
	}
	return cloneEval(s.eval), nil
}

func (s *missingAfterResetStore) GetByRootEntityID(
	context.Context,
	uuid.UUID,
	humanreview.PipelineType,
	humanreview.Job,
) (*humanreview.HumanEvaluation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneEval(s.eval), nil
}

func (s *missingAfterResetStore) Update(_ context.Context, evaluation *humanreview.HumanEvaluation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := s.eval
	s.eval = cloneEval(evaluation)
	if previous != nil && previous.Status == humanreview.StatusRejected && evaluation.Status == humanreview.StatusInProgress {
		s.resetDone = true
	}
	return nil
}

func (s *missingAfterResetStore) DeleteByRootEntityID(context.Context, uuid.UUID, humanreview.PipelineType, humanreview.Job) error {
	return nil
}

func TestDefaultHandlers_gateStartNilFails(t *testing.T) {
	humanreview.DefaultJobStepRegistry().Register("kit_job_nil_start", "kit_step_nil_start")
	root := uuid.New()
	eval := &humanreview.HumanEvaluation{
		ID:           uuid.New(),
		RootEntityID: root,
		Job:          "kit_job_nil_start",
		PipelineType: "demo",
		Status:       humanreview.StatusRejected,
	}
	store := &missingAfterResetStore{eval: eval}
	gate := humanreview.NewGate(store, silentLog{})
	fp := &fakePorts{eval: eval}
	run := &RunState{
		RootID:       root,
		Job:          "kit_job_nil_start",
		EvaluationID: eval.ID,
	}
	e := NewEngine(Config{Start: StateRegeneration})
	RegisterDefaultHandlers(e, Ports{
		Prompter:     fp,
		Generator:    fp,
		Session:      fp,
		Gate:         gate,
		PipelineType: "demo",
	}, run)
	err := e.Run(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reopening the approval gate")
}
