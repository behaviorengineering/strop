package orchestration

import (
	"context"
	"errors"
	"testing"

	"github.com/behaviorengineering/strop/pkg/streaming"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockCompositionStrategy struct {
	phases      []PhaseDef
	attempts    map[PhaseID]int
	passOnTry   map[PhaseID]int
	runPhaseErr error
	resultErr   error
	resultScore float64
}

func (m *mockCompositionStrategy) Phases() []PhaseDef {
	return m.phases
}

func (m *mockCompositionStrategy) RunPhase(
	_ context.Context,
	phase PhaseDef,
	_ string,
	_ streaming.EventChannel,
) (*PhaseResult, error) {
	if m.runPhaseErr != nil {
		return nil, m.runPhaseErr
	}
	m.attempts[phase.ID]++
	want := m.passOnTry[phase.ID]
	if want <= 0 {
		want = 1
	}
	passed := m.attempts[phase.ID] >= want
	return &PhaseResult{
		Fields:   map[string]string{"phase": string(phase.ID)},
		Score:    8.0,
		Feedback: "retry feedback",
		Passed:   passed,
	}, nil
}

func (m *mockCompositionStrategy) Result() (*CompositionResult, error) {
	if m.resultErr != nil {
		return nil, m.resultErr
	}
	score := m.resultScore
	if score == 0 {
		score = 9.0
	}
	return &CompositionResult{
		Score:       score,
		Feedback:    "assembled ok",
		OutputState: map[string]string{"doc": "draft"},
	}, nil
}

func TestRunCompositionLoop_allPhasesPass(t *testing.T) {
	strategy := &mockCompositionStrategy{
		phases: []PhaseDef{
			{ID: "skim", MaxAttempts: 3},
			{ID: "warmth", MaxAttempts: 3},
		},
		attempts:  make(map[PhaseID]int),
		passOnTry: map[PhaseID]int{"skim": 1, "warmth": 1},
	}

	out, err := RunCompositionLoop(context.Background(), strategy, nil)
	require.NoError(t, err)
	require.NotNil(t, out)
	assert.Equal(t, 9.0, out.Score)
	assert.Equal(t, "assembled ok", out.Feedback)
	assert.Equal(t, 1, strategy.attempts["skim"])
	assert.Equal(t, 1, strategy.attempts["warmth"])
}

func TestRunCompositionLoop_retriesUntilPass(t *testing.T) {
	strategy := &mockCompositionStrategy{
		phases: []PhaseDef{
			{ID: "depth", MaxAttempts: 3},
		},
		attempts:  make(map[PhaseID]int),
		passOnTry: map[PhaseID]int{"depth": 2},
	}

	out, err := RunCompositionLoop(context.Background(), strategy, nil)
	require.NoError(t, err)
	require.NotNil(t, out)
	assert.Equal(t, 2, strategy.attempts["depth"])
}

func TestRunCompositionLoop_exhaustsAttempts(t *testing.T) {
	strategy := &mockCompositionStrategy{
		phases: []PhaseDef{
			{ID: "skim", MaxAttempts: 2},
		},
		attempts:  make(map[PhaseID]int),
		passOnTry: map[PhaseID]int{"skim": 99},
	}

	out, err := RunCompositionLoop(context.Background(), strategy, nil)
	require.Error(t, err)
	assert.Nil(t, out)
	assert.Contains(t, err.Error(), "skim")
	assert.Equal(t, 2, strategy.attempts["skim"])
}

func TestRunCompositionLoop_nilStrategy(t *testing.T) {
	out, err := RunCompositionLoop(context.Background(), nil, nil)
	require.Error(t, err)
	assert.Nil(t, out)
	assert.Contains(t, err.Error(), "composition strategy is nil")
}

func TestRunCompositionLoop_runPhaseError(t *testing.T) {
	strategy := &mockCompositionStrategy{
		phases:      []PhaseDef{{ID: "skim", MaxAttempts: 1}},
		attempts:    make(map[PhaseID]int),
		passOnTry:   map[PhaseID]int{},
		runPhaseErr: errors.New("generate failed"),
	}

	out, err := RunCompositionLoop(context.Background(), strategy, nil)
	require.Error(t, err)
	assert.Nil(t, out)
	assert.Contains(t, err.Error(), "generate failed")
}

type mockCompensatingStrategy struct {
	mockCompositionStrategy
	budget       int
	planCalls    int
	applyCalls   int
	passOnApply  int
	collectErr   error
	planErr      error
	applyErr     error
	lastEvidence *CompensationEvidence
	lastPlan     *RepairPlan
}

func (m *mockCompensatingStrategy) CompensateAttempts(PhaseDef) int {
	return m.budget
}

func (m *mockCompensatingStrategy) CollectEvidence(
	_ context.Context,
	phase PhaseDef,
	feedback string,
	failed map[string]string,
) (*CompensationEvidence, error) {
	if m.collectErr != nil {
		return nil, m.collectErr
	}
	m.lastEvidence = &CompensationEvidence{
		PhaseID:      string(phase.ID),
		Feedback:     feedback,
		FailedOutput: failed,
	}
	return m.lastEvidence, nil
}

func (m *mockCompensatingStrategy) PlanRepair(
	_ context.Context,
	evidence *CompensationEvidence,
	_ streaming.EventChannel,
) (*RepairPlan, error) {
	m.planCalls++
	if m.planErr != nil {
		return nil, m.planErr
	}
	m.lastPlan = &RepairPlan{Summary: "repair " + evidence.PhaseID, Steps: []string{"rewrite owned fields"}, Body: evidence.Feedback}
	return m.lastPlan, nil
}

func (m *mockCompensatingStrategy) ApplyAndGate(
	_ context.Context,
	phase PhaseDef,
	_ *CompensationEvidence,
	_ *RepairPlan,
	_ streaming.EventChannel,
) (*PhaseResult, error) {
	m.applyCalls++
	if m.applyErr != nil {
		return nil, m.applyErr
	}
	passOn := m.passOnApply
	if passOn <= 0 {
		passOn = 1
	}
	return &PhaseResult{
		Fields:   map[string]string{"phase": string(phase.ID)},
		Score:    8.5,
		Feedback: "compensate feedback",
		Passed:   m.applyCalls >= passOn,
	}, nil
}

func TestRunCompositionLoop_compensatorNotCalledOnPass(t *testing.T) {
	strategy := &mockCompensatingStrategy{
		mockCompositionStrategy: mockCompositionStrategy{
			phases:    []PhaseDef{{ID: "fork", MaxAttempts: 3}},
			attempts:  make(map[PhaseID]int),
			passOnTry: map[PhaseID]int{"fork": 1},
		},
		budget: 2,
	}
	out, err := RunCompositionLoop(context.Background(), strategy, nil)
	require.NoError(t, err)
	require.NotNil(t, out)
	assert.Equal(t, 0, strategy.planCalls)
	assert.Equal(t, 0, strategy.applyCalls)
}

func TestRunCompositionLoop_compensatePassAfterExhaust(t *testing.T) {
	strategy := &mockCompensatingStrategy{
		mockCompositionStrategy: mockCompositionStrategy{
			phases:    []PhaseDef{{ID: "fork", MaxAttempts: 2}},
			attempts:  make(map[PhaseID]int),
			passOnTry: map[PhaseID]int{"fork": 99},
		},
		budget:      2,
		passOnApply: 1,
	}
	out, err := RunCompositionLoop(context.Background(), strategy, nil)
	require.NoError(t, err)
	require.NotNil(t, out)
	assert.Equal(t, 2, strategy.attempts["fork"])
	assert.Equal(t, 1, strategy.planCalls)
	assert.Equal(t, 1, strategy.applyCalls)
	require.NotNil(t, strategy.lastEvidence)
	assert.Len(t, strategy.lastEvidence.AttemptHistory, 2)
}

func TestRunCompositionLoop_compensateExhausts(t *testing.T) {
	strategy := &mockCompensatingStrategy{
		mockCompositionStrategy: mockCompositionStrategy{
			phases:    []PhaseDef{{ID: "fork", MaxAttempts: 1}},
			attempts:  make(map[PhaseID]int),
			passOnTry: map[PhaseID]int{"fork": 99},
		},
		budget:      2,
		passOnApply: 99,
	}
	out, err := RunCompositionLoop(context.Background(), strategy, nil)
	require.Error(t, err)
	assert.Nil(t, out)
	assert.Contains(t, err.Error(), "compensate attempts")
	assert.Equal(t, 2, strategy.applyCalls)
}

func TestRunCompositionLoop_zeroCompensateBudgetHardFails(t *testing.T) {
	strategy := &mockCompensatingStrategy{
		mockCompositionStrategy: mockCompositionStrategy{
			phases:    []PhaseDef{{ID: "fork", MaxAttempts: 1}},
			attempts:  make(map[PhaseID]int),
			passOnTry: map[PhaseID]int{"fork": 99},
		},
		budget: 0,
	}
	out, err := RunCompositionLoop(context.Background(), strategy, nil)
	require.Error(t, err)
	assert.Nil(t, out)
	assert.NotContains(t, err.Error(), "compensate")
	assert.Equal(t, 0, strategy.planCalls)
}

func TestRunCompositionLoop_resultError(t *testing.T) {
	strategy := &mockCompositionStrategy{
		phases: []PhaseDef{
			{ID: "skim", MaxAttempts: 1},
		},
		attempts:  make(map[PhaseID]int),
		passOnTry: map[PhaseID]int{"skim": 1},
		resultErr: errors.New("missing draft"),
	}

	out, err := RunCompositionLoop(context.Background(), strategy, nil)
	require.Error(t, err)
	assert.Nil(t, out)
	assert.Contains(t, err.Error(), "missing draft")
}
