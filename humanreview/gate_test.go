package humanreview

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	kitlog "github.com/behaviorengineering/strop/log"
)

type memStore struct {
	mu    sync.Mutex
	byID  map[uuid.UUID]*HumanEvaluation
	byKey map[string]uuid.UUID
}

func newMemStore() *memStore {
	return &memStore{
		byID:  make(map[uuid.UUID]*HumanEvaluation),
		byKey: make(map[string]uuid.UUID),
	}
}

func storeKey(root uuid.UUID, pipeline PipelineType, job Job) string {
	return root.String() + "|" + string(pipeline) + "|" + string(job)
}

func cloneEval(e *HumanEvaluation) *HumanEvaluation {
	if e == nil {
		return nil
	}
	cp := *e
	cp.CriterionEvaluations = append([]CriterionScoreEvaluation(nil), e.CriterionEvaluations...)
	cp.PipelineHistory.History = append([]PipelineHistoryEntry(nil), e.PipelineHistory.History...)
	if e.HumanAlignmentAgrees != nil {
		v := *e.HumanAlignmentAgrees
		cp.HumanAlignmentAgrees = &v
	}
	return &cp
}

func (s *memStore) Create(_ context.Context, evaluation *HumanEvaluation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byID[evaluation.ID] = cloneEval(evaluation)
	s.byKey[storeKey(evaluation.RootEntityID, evaluation.PipelineType, evaluation.Job)] = evaluation.ID
	return nil
}

func (s *memStore) GetByID(_ context.Context, id uuid.UUID) (*HumanEvaluation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneEval(s.byID[id]), nil
}

func (s *memStore) GetByRootEntityID(_ context.Context, rootEntityID uuid.UUID, pipelineType PipelineType, job Job) (*HumanEvaluation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.byKey[storeKey(rootEntityID, pipelineType, job)]
	if !ok {
		return nil, nil
	}
	return cloneEval(s.byID[id]), nil
}

func (s *memStore) Update(_ context.Context, evaluation *HumanEvaluation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byID[evaluation.ID] = cloneEval(evaluation)
	s.byKey[storeKey(evaluation.RootEntityID, evaluation.PipelineType, evaluation.Job)] = evaluation.ID
	return nil
}

func (s *memStore) DeleteByRootEntityID(_ context.Context, rootEntityID uuid.UUID, pipelineType PipelineType, job Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := storeKey(rootEntityID, pipelineType, job)
	id, ok := s.byKey[key]
	if !ok {
		return nil
	}
	delete(s.byID, id)
	delete(s.byKey, key)
	return nil
}

type silentLog struct{}

func (silentLog) WithField(string, interface{}) kitlog.Logger { return silentLog{} }
func (silentLog) WithFields(map[string]interface{}) kitlog.Logger {
	return silentLog{}
}
func (silentLog) WithError(error) kitlog.Logger { return silentLog{} }
func (silentLog) Debug(...interface{})          {}
func (silentLog) Info(...interface{})           {}
func (silentLog) Warn(...interface{})           {}
func (silentLog) Error(...interface{})          {}

func TestGate_StartResumeAndRecordAlignmentDoesNotChangeStatus(t *testing.T) {
	DefaultJobStepRegistry().Register("my_job", "my_step")

	store := newMemStore()
	g := NewGate(store, silentLog{})
	root := uuid.New()

	first, err := g.Start(context.Background(), "demo", root, "my_job")
	require.NoError(t, err)
	require.Equal(t, StatusInProgress, first.Status)
	assert.Empty(t, first.CriterionEvaluations)

	second, err := g.Start(context.Background(), "demo", root, "my_job")
	require.NoError(t, err)
	assert.Equal(t, first.ID, second.ID)

	err = g.RecordAlignment(context.Background(), first.ID, false, "too vague")
	require.NoError(t, err)
	got, err := store.GetByID(context.Background(), first.ID)
	require.NoError(t, err)
	require.NotNil(t, got.HumanAlignmentAgrees)
	assert.False(t, *got.HumanAlignmentAgrees)
	assert.Equal(t, "too vague", got.HumanAlignmentComment)
	assert.Equal(t, StatusInProgress, got.Status, "RecordAlignment must not change status")
}

func TestGate_StartReplacesCompleted(t *testing.T) {
	DefaultJobStepRegistry().Register("job_done", "step_done")
	store := newMemStore()
	g := NewGate(store, silentLog{})
	root := uuid.New()

	ev, err := g.Start(context.Background(), "demo", root, "job_done")
	require.NoError(t, err)
	require.NoError(t, g.SetStatus(context.Background(), ev.ID, StatusCompleted))

	next, err := g.Start(context.Background(), "demo", root, "job_done")
	require.NoError(t, err)
	assert.NotEqual(t, ev.ID, next.ID)
	assert.Equal(t, StatusInProgress, next.Status)
}

func TestGate_ResetRejected(t *testing.T) {
	DefaultJobStepRegistry().Register("job_rej", "step_rej")
	store := newMemStore()
	g := NewGate(store, silentLog{})
	root := uuid.New()

	ev, err := g.Start(context.Background(), "demo", root, "job_rej")
	require.NoError(t, err)
	require.NoError(t, g.RecordAlignment(context.Background(), ev.ID, false, "no"))
	require.NoError(t, g.SetStatus(context.Background(), ev.ID, StatusRejected))

	reset, err := g.Start(context.Background(), "demo", root, "job_rej")
	require.NoError(t, err)
	assert.Equal(t, ev.ID, reset.ID)
	assert.Nil(t, reset.HumanAlignmentAgrees)
	assert.Equal(t, StatusInProgress, reset.Status)
}

func TestGate_RecordAlignmentUnknownID(t *testing.T) {
	g := NewGate(newMemStore(), silentLog{})
	err := g.RecordAlignment(context.Background(), uuid.New(), true, "")
	require.Error(t, err)
}

func TestGate_StartUnknownJob(t *testing.T) {
	g := NewGate(newMemStore(), silentLog{})
	_, err := g.Start(context.Background(), "demo", uuid.New(), "not_registered_job_xyz")
	require.Error(t, err)
}
