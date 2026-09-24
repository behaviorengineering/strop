package stepplan

import (
	"errors"
	"fmt"
)

// ErrCorrupt is returned when persisted plan or checkpoint bytes cannot be decoded.
var ErrCorrupt = errors.New("stepplan: corrupt")

// ErrInvalid is returned when a plan or checkpoint fails validation.
var ErrInvalid = errors.New("stepplan: invalid")

// Error is a stepplan failure with a stable operation and wrapped cause.
type Error struct {
	Op  string
	Msg string
	err error
}

func (e *Error) Error() string {
	if e == nil {
		return "stepplan: error"
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

func corrupt(op, msg string, cause error) error {
	if cause == nil {
		return &Error{Op: op, Msg: msg, err: ErrCorrupt}
	}
	return &Error{Op: op, Msg: msg, err: errors.Join(ErrCorrupt, cause)}
}

func invalid(op, msg string) error {
	return &Error{Op: op, Msg: msg, err: ErrInvalid}
}
