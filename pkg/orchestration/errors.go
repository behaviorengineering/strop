package orchestration

import (
	"fmt"
)

// Error is an orchestration failure with a stable operation and wrapped cause.
type Error struct {
	Op  string
	Msg string
	err error
}

func (e *Error) Error() string {
	if e == nil {
		return "orchestration: error"
	}
	if e.Op == "" {
		return e.Msg
	}
	if e.Msg == "" {
		return e.Op
	}
	return fmt.Sprintf("%s: %s", e.Op, e.Msg)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func wrap(op string, cause error) error {
	if cause == nil {
		return nil
	}
	return &Error{Op: op, Msg: cause.Error(), err: cause}
}
