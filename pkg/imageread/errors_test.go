package imageread

import (
	"errors"
	"testing"
)

func TestTooLargeUnwraps(t *testing.T) {
	err := tooLarge("LoadBytes", "too big")
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("expected ErrTooLarge, got %v", err)
	}
}

func TestInvalidUnwraps(t *testing.T) {
	err := invalid("LoadBytes", "empty")
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected ErrInvalid, got %v", err)
	}
}
