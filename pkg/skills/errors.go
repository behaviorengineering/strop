package skills

import (
	"errors"
	"fmt"
)

// CodeInvalid is the stable code for a rejected skill, bullet, request, or trajectory.
const CodeInvalid = "skills.invalid"

// ErrInvalid is the sentinel cause for CodeInvalid errors.
var ErrInvalid = errors.New("skills: invalid")

// Error is a validation failure with a stable code and operation.
type Error struct {
	Code   string
	Op     string
	Msg    string
	Fields map[string]string
	err    error
}

// Error returns the operation and message.
func (e *Error) Error() string {
	if e == nil {
		return "skills: error"
	}
	msg := e.Msg
	if msg == "" {
		msg = "invalid"
	}
	if e.Op == "" {
		return msg
	}
	return fmt.Sprintf("%s: %s", e.Op, msg)
}

// Unwrap returns the sentinel cause.
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func invalid(op, msg string) error {
	return &Error{Code: CodeInvalid, Op: op, Msg: msg, err: ErrInvalid}
}

func invalidField(op, msg, key, value string) error {
	return &Error{
		Code:   CodeInvalid,
		Op:     op,
		Msg:    msg,
		Fields: map[string]string{key: value},
		err:    ErrInvalid,
	}
}
