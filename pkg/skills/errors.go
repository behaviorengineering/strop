package skills

import (
	"errors"
	"fmt"
)

// ErrInvalid is returned when a skill, bullet, request, or trajectory fails validation.
var ErrInvalid = errors.New("skills: invalid")

func invalid(msg string) error {
	return fmt.Errorf("%w: %s", ErrInvalid, msg)
}
