package integrity

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func ctxWithDeadline(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func validateTitleBody(raw string) error {
	hasTitle := strings.Contains(raw, "TITLE:")
	hasBody := strings.Contains(raw, "BODY:")
	if hasTitle && hasBody {
		return nil
	}
	return errors.New("missing TITLE or BODY")
}

type recordingChat struct {
	calls     [][]Message
	responses []string
	errOn     int // 1-based attempt index; 0 = never
	err       error
}

func (r *recordingChat) chat(ctx context.Context, messages []Message) (string, error) {
	r.calls = append(r.calls, append([]Message{}, messages...))
	attempt := len(r.calls)
	if r.errOn == attempt {
		return "", r.err
	}
	if len(r.responses) == 0 {
		return "", errors.New("no response configured")
	}
	idx := attempt - 1
	if idx >= len(r.responses) {
		idx = len(r.responses) - 1
	}
	return r.responses[idx], nil
}

func TestCreateShellNilChat(t *testing.T) {
	_, err := CreateShell(ShellConfig{
		Validate: validateTitleBody,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected ErrInvalid, got %v", err)
	}
}

func TestCreateShellNilValidate(t *testing.T) {
	_, err := CreateShell(ShellConfig{
		Chat: func(context.Context, []Message) (string, error) { return "", nil },
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected ErrInvalid, got %v", err)
	}
}

func TestRunSuccessFirstTry(t *testing.T) {
	rec := &recordingChat{responses: []string{"TITLE: x\nBODY: y"}}
	sh, err := CreateShell(ShellConfig{Chat: rec.chat, Validate: validateTitleBody})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := sh.Run(ctxWithDeadline(t), "sys", "usr")
	if err != nil {
		t.Fatal(err)
	}
	if raw != "TITLE: x\nBODY: y" {
		t.Fatalf("raw %q", raw)
	}
	if len(rec.calls) != 1 {
		t.Fatalf("expected 1 chat call, got %d", len(rec.calls))
	}
}

func TestRunCompleteOnAttempt2(t *testing.T) {
	note := "please complete"
	rec := &recordingChat{responses: []string{
		"TITLE: only",
		"TITLE: ok\nBODY: ok",
	}}
	sh, err := CreateShell(ShellConfig{
		Chat:         rec.chat,
		Validate:     validateTitleBody,
		CompleteNote: func(error) string { return note },
		MaxAttempts:  3,
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := sh.Run(ctxWithDeadline(t), "sys", "usr")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw, "BODY:") {
		t.Fatalf("expected body in %q", raw)
	}
	if len(rec.calls) != 2 {
		t.Fatalf("expected 2 chat calls, got %d", len(rec.calls))
	}
	second := rec.calls[1]
	if len(second) < 4 {
		t.Fatalf("expected 4+ messages on second call, got %d", len(second))
	}
	if second[2].Role != RoleAssistant || second[2].Content != "TITLE: only" {
		t.Fatalf("assistant turn: %+v", second[2])
	}
	if second[3].Role != RoleUser || !strings.Contains(second[3].Content, note) {
		t.Fatalf("user complete note: %+v", second[3])
	}
}

func TestRunExhausted(t *testing.T) {
	rec := &recordingChat{responses: []string{"a", "b", "c"}}
	sh, err := CreateShell(ShellConfig{
		Chat:        rec.chat,
		Validate:    validateTitleBody,
		MaxAttempts: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := sh.Run(ctxWithDeadline(t), "sys", "usr")
	if err == nil {
		t.Fatal("expected error")
	}
	if raw != "c" {
		t.Fatalf("last raw %q", raw)
	}
	if !errors.Is(err, ErrExhausted) {
		t.Fatalf("expected ErrExhausted, got %v", err)
	}
	unwrapped := errors.Unwrap(err)
	for unwrapped != nil {
		if strings.Contains(unwrapped.Error(), "missing TITLE") {
			break
		}
		unwrapped = errors.Unwrap(unwrapped)
	}
	if unwrapped == nil {
		t.Fatalf("expected validate cause in chain: %v", err)
	}
	if len(rec.calls) != 3 {
		t.Fatalf("expected 3 calls, got %d", len(rec.calls))
	}
}

func TestRunChatError(t *testing.T) {
	chatErr := errors.New("transport down")
	rec := &recordingChat{
		responses: []string{"TITLE: x\nBODY: y"},
		errOn:     1,
		err:       chatErr,
	}
	sh, err := CreateShell(ShellConfig{Chat: rec.chat, Validate: validateTitleBody})
	if err != nil {
		t.Fatal(err)
	}
	_, err = sh.Run(ctxWithDeadline(t), "sys", "usr")
	if err == nil {
		t.Fatal("expected error")
	}
	if errors.Is(err, ErrExhausted) {
		t.Fatal("chat error must not be exhausted")
	}
	if len(rec.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(rec.calls))
	}
}

func TestRunNoDeadline(t *testing.T) {
	rec := &recordingChat{responses: []string{"TITLE: x\nBODY: y"}}
	sh, err := CreateShell(ShellConfig{Chat: rec.chat, Validate: validateTitleBody})
	if err != nil {
		t.Fatal(err)
	}
	_, err = sh.Run(context.Background(), "sys", "usr")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected ErrInvalid, got %v", err)
	}
	if len(rec.calls) != 0 {
		t.Fatalf("expected no chat, got %d", len(rec.calls))
	}
}

func TestResumeSkip(t *testing.T) {
	rec := &recordingChat{responses: []string{"TITLE: x\nBODY: y"}}
	sh, err := CreateShell(ShellConfig{Chat: rec.chat, Validate: validateTitleBody})
	if err != nil {
		t.Fatal(err)
	}
	prior := []Message{
		{Role: RoleSystem, Content: "sys"},
		{Role: RoleUser, Content: "usr"},
	}
	raw, err := sh.Resume(ctxWithDeadline(t), prior, "already ok", nil)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "already ok" {
		t.Fatalf("raw %q", raw)
	}
	if len(rec.calls) != 0 {
		t.Fatalf("expected 0 calls, got %d", len(rec.calls))
	}
}

func TestResumeContinue(t *testing.T) {
	note := "finish it"
	rec := &recordingChat{responses: []string{"TITLE: ok\nBODY: ok"}}
	sh, err := CreateShell(ShellConfig{
		Chat:         rec.chat,
		Validate:     validateTitleBody,
		CompleteNote: func(error) string { return note },
		MaxAttempts:  3,
	})
	if err != nil {
		t.Fatal(err)
	}
	prior := []Message{
		{Role: RoleSystem, Content: "sys"},
		{Role: RoleUser, Content: "usr"},
	}
	parseErr := errors.New("missing TITLE or BODY")
	raw, err := sh.Resume(ctxWithDeadline(t), prior, "TITLE: partial", parseErr)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw, "BODY:") {
		t.Fatalf("raw %q", raw)
	}
	if len(rec.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(rec.calls))
	}
	msgs := rec.calls[0]
	if len(msgs) < 4 {
		t.Fatalf("messages %d", len(msgs))
	}
	if msgs[2].Role != RoleAssistant || msgs[2].Content != "TITLE: partial" {
		t.Fatalf("assistant %+v", msgs[2])
	}
	if msgs[3].Role != RoleUser || !strings.Contains(msgs[3].Content, note) {
		t.Fatalf("user %+v", msgs[3])
	}
}
