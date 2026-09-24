package imageread

import (
	"errors"
	"fmt"
)

// ErrInvalid is returned when the path or URL is empty or not an image source.
var ErrInvalid = errors.New("imageread: invalid")

// ErrTooLarge is returned when the image exceeds the size limit.
var ErrTooLarge = errors.New("imageread: too large")

// Error is an imageread failure with a stable operation and wrapped cause.
type Error struct {
	Op  string
	Msg string
	err error
}

func (e *Error) Error() string {
	if e == nil {
		return "imageread: error"
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

func tooLarge(op, msg string) error {
	return &Error{Op: op, Msg: msg, err: ErrTooLarge}
}

func wrap(op string, cause error) error {
	if cause == nil {
		return nil
	}
	return &Error{Op: op, Msg: cause.Error(), err: cause}
}
