package integrity

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestValidationErrorFromExhausted(t *testing.T) {
	valErr := errors.New("missing TITLE or BODY")
	sh, err := CreateShell(ShellConfig{
		Chat: func(context.Context, []Message) (string, error) {
			return "bad", nil
		},
		Validate:    func(string) error { return valErr },
		MaxAttempts: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, runErr := sh.Run(ctxWithDeadline(t), "sys", "usr")
	if runErr == nil {
		t.Fatal("expected error")
	}
	got := ValidationError(runErr)
	if got == nil {
		t.Fatal("expected validate cause")
	}
	if !strings.Contains(got.Error(), "missing TITLE") {
		t.Fatalf("got %v", got)
	}
	if errors.Is(got, ErrExhausted) {
		t.Fatal("cause must not be ErrExhausted")
	}
}

func TestValidationErrorNotExhausted(t *testing.T) {
	if ValidationError(errors.New("other")) != nil {
		t.Fatal("expected nil")
	}
}
