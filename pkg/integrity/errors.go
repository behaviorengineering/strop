package integrity

import (
	"errors"
	"fmt"
)

// ErrInvalid is returned for nil dependencies, missing deadline, or empty inputs.
var ErrInvalid = errors.New("integrity: invalid")

// ErrExhausted is returned when validation still fails after MaxAttempts.
var ErrExhausted = errors.New("integrity: exhausted")

// Error is an integrity failure with a stable operation and wrapped cause.
type Error struct {
	Op  string
	Msg string
	err error
}

func (e *Error) Error() string {
	if e == nil {
		return "integrity: error"
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

func invalid(op, msg string) error {
	return &Error{Op: op, Msg: msg, err: ErrInvalid}
}

func exhausted(op, msg string, cause error) error {
	return &Error{Op: op, Msg: msg, err: errors.Join(ErrExhausted, cause)}
}

func wrapChat(op string, cause error) error {
	if cause == nil {
		return nil
	}
	return &Error{Op: op, Msg: "chat", err: cause}
}
