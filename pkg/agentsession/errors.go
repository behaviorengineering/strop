package agentsession

import (
	"errors"
	"fmt"
)

// ErrNotFound is returned when a session id does not exist.
var ErrNotFound = errors.New("agentsession: not found")

// ErrClosed is returned when mutating a closed session.
var ErrClosed = errors.New("agentsession: closed")

// ErrCorrupt is returned when session meta or transcript lines cannot be decoded.
var ErrCorrupt = errors.New("agentsession: corrupt")

// ErrInvalid is returned when a request fails validation.
var ErrInvalid = errors.New("agentsession: invalid")

// Error is an agentsession failure with a stable operation and wrapped cause.
type Error struct {
	Op  string
	Msg string
	err error
}

func (e *Error) Error() string {
	if e == nil {
		return "agentsession: error"
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
