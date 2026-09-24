package stepplan

import (
	"errors"
	"testing"
)

func TestCorruptUnwraps(t *testing.T) {
	err := corrupt("LoadStep", "bad json", errors.New("syntax"))
	if !errors.Is(err, ErrCorrupt) {
		t.Fatalf("expected ErrCorrupt, got %v", err)
	}
}

func TestInvalidUnwraps(t *testing.T) {
	err := invalid("NewPlan", "empty id")
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected ErrInvalid, got %v", err)
	}
}
