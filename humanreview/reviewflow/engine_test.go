package reviewflow

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEngine_terminalsStopWithoutRunningHandler(t *testing.T) {
	t.Parallel()
	exitCalled := false
	e := NewEngine(Config{Start: StateCompletion})
	e.Register(StateCompletion, func(context.Context) (State, error) {
		return StateExit, nil
	})
	e.Register(StateExit, func(context.Context) (State, error) {
		exitCalled = true
		return StateExit, nil
	})
	require.NoError(t, e.Run(context.Background()))
	assert.False(t, exitCalled, "StateExit is terminal and must not run its handler")
}

func TestEngine_rejectionIsTerminal(t *testing.T) {
	t.Parallel()
	rejectionCalled := false
	e := NewEngine(Config{Start: StateGeneration})
	e.Register(StateGeneration, func(context.Context) (State, error) {
		return StateRejection, nil
	})
	e.Register(StateRejection, func(context.Context) (State, error) {
		rejectionCalled = true
		return StateExit, nil
	})
	require.NoError(t, e.Run(context.Background()))
	assert.False(t, rejectionCalled)
}

func TestEngine_doneIsTerminal(t *testing.T) {
	t.Parallel()
	doneCalled := false
	e := NewEngine(Config{Start: StateGeneration})
	e.Register(StateGeneration, func(context.Context) (State, error) {
		return StateDone, nil
	})
	e.Register(StateDone, func(context.Context) (State, error) {
		doneCalled = true
		return StateExit, nil
	})
	require.NoError(t, e.Run(context.Background()))
	assert.False(t, doneCalled)
}

func TestEngine_maxIterations(t *testing.T) {
	t.Parallel()
	e := NewEngine(Config{Start: StateInit, MaxIterations: 3})
	e.Register(StateInit, func(context.Context) (State, error) {
		return StateInit, nil
	})
	err := e.Run(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeded maximum iterations")
}

func TestEngine_missingHandler(t *testing.T) {
	t.Parallel()
	e := NewEngine(Config{Start: StateInit})
	err := e.Run(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no handler for state")
	assert.Contains(t, err.Error(), "init")
}

func TestEngine_handlerError(t *testing.T) {
	t.Parallel()
	e := NewEngine(Config{Start: StateInit})
	e.Register(StateInit, func(context.Context) (State, error) {
		return StateExit, errors.New("handler failed")
	})
	err := e.Run(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "init")
	assert.Contains(t, err.Error(), "handler failed")
}

func TestEngine_completionRunsThenExitStops(t *testing.T) {
	t.Parallel()
	visited := []State{}
	e := NewEngine(Config{Start: StateFinalizeCriteria})
	e.Register(StateFinalizeCriteria, func(context.Context) (State, error) {
		visited = append(visited, StateFinalizeCriteria)
		return StateCompletion, nil
	})
	e.Register(StateCompletion, func(context.Context) (State, error) {
		visited = append(visited, StateCompletion)
		return StateExit, nil
	})
	require.NoError(t, e.Run(context.Background()))
	assert.Equal(t, []State{StateFinalizeCriteria, StateCompletion}, visited)
}

func TestEngine_cancelsBetweenStates(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	generationRan := false
	e := NewEngine(Config{Start: StateInit})
	e.Register(StateInit, func(context.Context) (State, error) {
		cancel()
		return StateGeneration, nil
	})
	e.Register(StateGeneration, func(context.Context) (State, error) {
		generationRan = true
		return StateExit, nil
	})
	err := e.Run(ctx)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.False(t, generationRan)
}
