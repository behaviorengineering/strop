package orchestration

import (
	"context"
	"testing"

	stroplog "github.com/behaviorengineering/strop/log"
	"github.com/behaviorengineering/strop/refinement"
	"github.com/behaviorengineering/strop/streaming"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testLogger struct {
	entry *logrus.Entry
}

func (l *testLogger) WithField(key string, value interface{}) stroplog.Logger {
	return &testLogger{entry: l.entry.WithField(key, value)}
}

func (l *testLogger) WithFields(fields map[string]interface{}) stroplog.Logger {
	return &testLogger{entry: l.entry.WithFields(fields)}
}

func (l *testLogger) WithError(err error) stroplog.Logger {
	return &testLogger{entry: l.entry.WithError(err)}
}

func (l *testLogger) Debug(args ...interface{}) { l.entry.Debug(args...) }
func (l *testLogger) Info(args ...interface{})  { l.entry.Info(args...) }
func (l *testLogger) Warn(args ...interface{})  { l.entry.Warn(args...) }
func (l *testLogger) Error(args ...interface{}) { l.entry.Error(args...) }

func testStropLogger() stroplog.Logger {
	return &testLogger{entry: logrus.NewEntry(logrus.New())}
}

type fakeRefinementStrategy struct {
	entityID         uuid.UUID
	selectedID       uuid.UUID
	scores           []float64
	feedbacks        []string
	saveCount        int
	generateCount    int
	withInitialScore bool
}

func (f *fakeRefinementStrategy) LoadContext(_ context.Context, entityID uuid.UUID) (*LoopContext, error) {
	f.entityID = entityID
	loopCtx := &LoopContext{
		NextVersion: 1,
		State:       nil,
	}
	if f.withInitialScore {
		prevScore := 8.0
		loopCtx.InitialPreviousScore = &prevScore
		loopCtx.InitialSelectedID = &f.selectedID
	}
	return loopCtx, nil
}

func (f *fakeRefinementStrategy) GenerateAndEvaluate(_ context.Context, _ int, _ string, _ interface{}, _ streaming.EventChannel) (*IterationOutput, error) {
	idx := f.generateCount
	if idx >= len(f.scores) {
		idx = len(f.scores) - 1
	}
	f.generateCount++
	return &IterationOutput{
		Score:       f.scores[idx],
		Feedback:    f.feedbacks[idx],
		OutputState: "output",
	}, nil
}

func (f *fakeRefinementStrategy) SaveVersion(_ context.Context, _ int, _ interface{}, _ *IterationOutput) (uuid.UUID, error) {
	f.saveCount++
	if f.selectedID == uuid.Nil {
		f.selectedID = uuid.New()
	}
	return f.selectedID, nil
}

func (f *fakeRefinementStrategy) ContextID() uuid.UUID {
	if f.entityID == uuid.Nil {
		return uuid.New()
	}
	return f.entityID
}

func TestRunRefinementLoop_perfectScoreStopsAfterSave(t *testing.T) {
	t.Parallel()
	policy := refinement.NewService(testStropLogger(), "rejected", "pending", 0)
	strategy := &fakeRefinementStrategy{
		scores:    []float64{10.0},
		feedbacks: []string{"all good"},
	}

	id, err := RunRefinementLoop(
		context.Background(),
		uuid.New(),
		strategy,
		policy,
		3,
		nil,
	)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, id)
	assert.Equal(t, 1, strategy.saveCount)
	assert.Equal(t, 1, strategy.generateCount)
}

func TestRunRefinementLoop_scoreDecreaseReturnsPreviousSelection(t *testing.T) {
	t.Parallel()
	policy := refinement.NewService(testStropLogger(), "rejected", "pending", 0)
	previousID := uuid.New()
	strategy := &fakeRefinementStrategy{
		selectedID: previousID,
		scores:     []float64{8.0, 6.0},
		feedbacks:  []string{"ok", "worse"},
	}
	strategy.withInitialScore = true

	id, err := RunRefinementLoop(
		context.Background(),
		uuid.New(),
		strategy,
		policy,
		5,
		nil,
	)
	require.NoError(t, err)
	assert.Equal(t, previousID, id)
	assert.Equal(t, 1, strategy.saveCount)
	assert.Equal(t, 2, strategy.generateCount)
}

func TestRunRefinementLoop_maxVersionsStopsWithoutExtraGenerations(t *testing.T) {
	t.Parallel()
	policy := refinement.NewService(testStropLogger(), "rejected", "pending", 0)
	strategy := &fakeRefinementStrategy{
		scores:    []float64{5.0, 6.0, 7.0, 8.0},
		feedbacks: []string{"a", "b", "c", "d"},
	}

	id, err := RunRefinementLoop(
		context.Background(),
		uuid.New(),
		strategy,
		policy,
		2,
		nil,
	)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, id)
	assert.Equal(t, 2, strategy.generateCount)
	assert.Equal(t, 2, strategy.saveCount)
}
