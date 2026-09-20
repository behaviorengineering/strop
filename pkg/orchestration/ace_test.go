package orchestration

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/behaviorengineering/strop/pkg/dspy/ace"
	"github.com/behaviorengineering/strop/pkg/refinement"
	"github.com/behaviorengineering/strop/pkg/runreport"
	"github.com/behaviorengineering/strop/pkg/streaming"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type aceAwareStrategy struct {
	fakeRefinementStrategy
	seenPlaybooks []string
}

func (s *aceAwareStrategy) RunReportMeta() runreport.Meta {
	return runreport.PipelineJobMeta("test", "refinement")
}

func (s *aceAwareStrategy) GenerateAndEvaluate(ctx context.Context, version int, previousFeedback string, state interface{}, eventChan streaming.EventChannel) (*IterationOutput, error) {
	if m := ace.FromContext(ctx); m != nil {
		s.seenPlaybooks = append(s.seenPlaybooks, m.LearningsContext())
	} else {
		s.seenPlaybooks = append(s.seenPlaybooks, "")
	}
	return s.fakeRefinementStrategy.GenerateAndEvaluate(ctx, version, previousFeedback, state, eventChan)
}

func (s *aceAwareStrategy) DiagnoseForHealing(previousState, currentState interface{}, feedback string) *refinement.ProblemDiagnosis {
	return refinement.DiagnoseFromFeedbackOnly(feedback)
}

func TestRunRefinementLoop_ACEAbsentUnchanged(t *testing.T) {
	t.Parallel()
	policy := refinement.NewService(testStropLogger(), "rejected", "pending", 0)
	strategy := &aceAwareStrategy{
		fakeRefinementStrategy: fakeRefinementStrategy{
			scores:    []float64{10.0},
			feedbacks: []string{"all good"},
		},
	}
	id, err := RunRefinementLoop(context.Background(), uuid.New(), strategy, policy, 3, nil)
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, id)
	require.Equal(t, 1, strategy.generateCount)
}

func TestRunRefinementLoop_ACEWrongBindingFailsClosed(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "learnings.md")
	m, err := ace.NewManager(ace.Config{Enabled: true, LearningsPath: path}, "other-entity", "refinement")
	require.NoError(t, err)
	defer func() { require.NoError(t, m.Close()) }()

	ctx := ace.WithManager(context.Background(), m)
	policy := refinement.NewService(testStropLogger(), "rejected", "pending", 0)
	strategy := &aceAwareStrategy{
		fakeRefinementStrategy: fakeRefinementStrategy{
			scores:    []float64{10.0},
			feedbacks: []string{"ok"},
		},
	}
	_, err = RunRefinementLoop(ctx, uuid.New(), strategy, policy, 2, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "binding mismatch")
}

func TestRunRefinementLoop_ACETwoVersionsSameSession(t *testing.T) {
	t.Parallel()
	entityID := uuid.New()
	path := filepath.Join(t.TempDir(), "learnings.md")
	m, err := ace.NewManager(ace.Config{Enabled: true, LearningsPath: path, AsyncReflection: false}, entityID.String(), "refinement")
	require.NoError(t, err)
	defer func() { require.NoError(t, m.Close()) }()

	ctx := ace.WithManager(context.Background(), m)
	policy := refinement.NewService(testStropLogger(), "rejected", "pending", 0)
	strategy := &aceAwareStrategy{
		fakeRefinementStrategy: fakeRefinementStrategy{
			scores:    []float64{8.0, 10.0},
			feedbacks: []string{"needs work [L001]", "all good [L001]"},
		},
	}
	id, err := RunRefinementLoop(ctx, entityID, strategy, policy, 5, nil)
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, id)
	require.Equal(t, 2, strategy.generateCount)
	require.Len(t, strategy.seenPlaybooks, 2)
	// Second attempt still has Manager on ctx (inject path available).
	require.NotNil(t, ace.FromContext(ctx))
}

func TestRunRefinementLoop_ACEHealingRecordsTrajectory(t *testing.T) {
	t.Parallel()
	entityID := uuid.New()
	path := filepath.Join(t.TempDir(), "learnings.md")
	m, err := ace.NewManager(ace.Config{Enabled: true, LearningsPath: path}, entityID.String(), "refinement")
	require.NoError(t, err)
	defer func() { require.NoError(t, m.Close()) }()

	ctx := ace.WithManager(context.Background(), m)
	policy := refinement.NewService(testStropLogger(), "rejected", "pending", 1)
	selected := uuid.New()
	strategy := &aceAwareStrategy{
		fakeRefinementStrategy: fakeRefinementStrategy{
			selectedID:       selected,
			withInitialScore: true,
			scores:           []float64{6.0, 10.0},
			feedbacks:        []string{"worse", "recovered"},
		},
	}
	id, err := RunRefinementLoop(ctx, entityID, strategy, policy, 5, nil)
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, id)
	require.GreaterOrEqual(t, strategy.generateCount, 2)
}
