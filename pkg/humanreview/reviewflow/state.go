package reviewflow

// State is one step in the portable reviewflow engine.
type State string

const (
	StateInit             State = "init"
	StateGeneration       State = "generation"
	StateAlignment        State = "alignment"
	StateRegeneration     State = "regeneration"
	StateFinalizeCriteria State = "finalize_criteria"
	StateCompletion       State = "completion"
	StateExit             State = "exit"
	StateRejection        State = "rejection"
	StateDone             State = "done"
)

// DefaultTerminals are states that stop the loop without running a handler.
// Returning Exit, Rejection, or Done from a live handler ends the run.
func DefaultTerminals() map[State]struct{} {
	return map[State]struct{}{
		StateExit:      {},
		StateRejection: {},
		StateDone:      {},
	}
}

func isTerminal(terminals map[State]struct{}, state State) bool {
	if terminals == nil {
		_, ok := DefaultTerminals()[state]
		return ok
	}
	_, ok := terminals[state]
	return ok
}
