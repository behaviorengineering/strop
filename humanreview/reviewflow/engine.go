package reviewflow

import (
	"context"
	"fmt"

	"github.com/behaviorengineering/strop/log"
)

const defaultMaxIterations = 1000

// Handler processes the current state and returns the next state.
type Handler func(ctx context.Context) (State, error)

// Config constructs an Engine.
type Config struct {
	Start         State
	MaxIterations int
	Terminals     map[State]struct{}
	Logger        log.Logger
}

// Engine runs a handler table until a terminal state is returned.
type Engine struct {
	start         State
	handlers      map[State]Handler
	terminals     map[State]struct{}
	maxIterations int
	logger        log.Logger
}

// NewEngine creates an engine. Register handlers before Run.
func NewEngine(cfg Config) *Engine {
	maxIterations := cfg.MaxIterations
	if maxIterations <= 0 {
		maxIterations = defaultMaxIterations
	}
	terminals := cfg.Terminals
	if terminals == nil {
		terminals = DefaultTerminals()
	}
	return &Engine{
		start:         cfg.Start,
		handlers:      make(map[State]Handler),
		terminals:     terminals,
		maxIterations: maxIterations,
		logger:        cfg.Logger,
	}
}

// Register sets the handler for a state. Later calls replace the previous handler.
func (e *Engine) Register(state State, handler Handler) {
	if e == nil {
		return
	}
	e.handlers[state] = handler
}

// Run executes handlers from Config.Start until a terminal next-state is returned.
// Terminal states (default: Exit, Rejection, Done) stop the loop without running those handlers.
func (e *Engine) Run(ctx context.Context) error {
	if e == nil {
		return fmt.Errorf("reviewflow engine is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	state := e.start
	iteration := 0
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		iteration++
		if iteration > e.maxIterations {
			return fmt.Errorf("state machine exceeded maximum iterations (%d): possible infinite loop in state %s", e.maxIterations, state)
		}
		if e.logger != nil {
			e.logger.WithFields(map[string]interface{}{
				"state":     string(state),
				"iteration": iteration,
			}).Debug("State transition")
		}
		handler, ok := e.handlers[state]
		if !ok {
			return fmt.Errorf("no handler for state: %s", state)
		}
		nextState, err := handler(ctx)
		if err != nil {
			return fmt.Errorf("state %s failed: %w", state, err)
		}
		if isTerminal(e.terminals, nextState) {
			return nil
		}
		state = nextState
	}
}
