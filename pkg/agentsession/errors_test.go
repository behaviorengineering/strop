package agentsession

import (
	"errors"
	"testing"
)

func TestCorruptUnwraps(t *testing.T) {
	err := corrupt("readMeta", "bad json", errors.New("syntax"))
	if !errors.Is(err, ErrCorrupt) {
		t.Fatalf("expected ErrCorrupt, got %v", err)
	}
}
