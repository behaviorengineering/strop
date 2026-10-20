package integrity

import (
	"context"
	"strings"
)

const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"

	DefaultMaxAttempts = 3
)

// Message is one chat turn for ChatFunc.
type Message struct {
	Role    string
	Content string
}

// ChatFunc performs one chat completion with the full message list.
type ChatFunc func(ctx context.Context, messages []Message) (string, error)

// ValidateFunc checks raw model output; non-nil error means incomplete or invalid.
type ValidateFunc func(raw string) error

// CompleteNoteFunc builds the user turn after a validation failure.
type CompleteNoteFunc func(validateErr error) string

// ShellConfig configures CreateShell.
type ShellConfig struct {
	Chat         ChatFunc
	Validate     ValidateFunc
	CompleteNote CompleteNoteFunc
	AfterMiss    AfterMissFunc
	Compensate   CompensateFunc
	MaxAttempts  int
}

// Shell runs chat, validate, and complete-retry loops.
type Shell struct {
	chat         ChatFunc
	validate     ValidateFunc
	completeNote CompleteNoteFunc
	afterMiss    AfterMissFunc
	compensate   CompensateFunc
	maxAttempts  int
}

// CreateShell builds a Shell from cfg.
func CreateShell(cfg ShellConfig) (*Shell, error) {
	const op = "integrity.CreateShell"
	if cfg.Chat == nil {
		return nil, invalid(op, "nil Chat")
	}
	if cfg.Validate == nil {
		return nil, invalid(op, "nil Validate")
	}
	max := cfg.MaxAttempts
	if max <= 0 {
		max = DefaultMaxAttempts
	}
	note := cfg.CompleteNote
	if note == nil {
		note = DefaultCompleteNote
	}
	return &Shell{
		chat:         cfg.Chat,
		validate:     cfg.Validate,
		completeNote: note,
		afterMiss:    cfg.AfterMiss,
		compensate:   cfg.Compensate,
		maxAttempts:  max,
	}, nil
}

// DefaultCompleteNote is the portable completion instruction when CompleteNote is nil.
func DefaultCompleteNote(validateErr error) string {
	_ = validateErr
	return "Continue your previous reply. Emit any missing required fields. Do not start over."
}

// Run starts a new conversation with system and user prompts.
func (s *Shell) Run(ctx context.Context, system, user string) (string, error) {
	const op = "integrity.Shell.Run"
	if err := s.requireReady(ctx, op); err != nil {
		return "", err
	}
	system = strings.TrimSpace(system)
	user = strings.TrimSpace(user)
	if system == "" && user == "" {
		return "", invalid(op, "empty system and user")
	}
	seed := []Message{
		{Role: RoleSystem, Content: system},
		{Role: RoleUser, Content: user},
	}
	messages := append([]Message{}, seed...)
	return s.loopWithBudget(ctx, op, s.chat, messages, s.maxAttempts, true, seed)
}

// Resume continues after a prior first attempt (e.g. batch prefetch) failed validation.
func (s *Shell) Resume(ctx context.Context, prior []Message, lastRaw string, lastErr error) (string, error) {
	const op = "integrity.Shell.Resume"
	if err := s.requireReady(ctx, op); err != nil {
		return "", err
	}
	if lastErr == nil {
		return lastRaw, nil
	}
	if len(prior) == 0 {
		return lastRaw, invalid(op, "empty prior with validation error")
	}
	seed := append([]Message{}, prior...)
	messages := append([]Message{}, prior...)
	switch s.missAction(lastErr) {
	case MissRestart:
		// Retry from seed without appending the failed prefetch turn.
	default:
		messages = append(messages,
			Message{Role: RoleAssistant, Content: lastRaw},
			Message{Role: RoleUser, Content: s.noteFor(lastErr)},
		)
	}
	remaining := s.maxAttempts - 1
	if remaining <= 0 {
		return s.finishAfterExhaust(ctx, op, messages, lastRaw, lastErr, seed, s.chat, true)
	}
	return s.loopWithBudget(ctx, op, s.chat, messages, remaining, true, seed)
}

func (s *Shell) requireReady(ctx context.Context, op string) error {
	if s == nil || s.chat == nil || s.validate == nil {
		return invalid(op, "nil shell")
	}
	if ctx == nil {
		return invalid(op, "nil context")
	}
	if _, ok := ctx.Deadline(); !ok {
		return invalid(op, "missing context deadline")
	}
	return nil
}

func (s *Shell) missAction(valErr error) MissAction {
	if s == nil || s.afterMiss == nil {
		return MissContinue
	}
	switch s.afterMiss(valErr) {
	case MissRestart:
		return MissRestart
	default:
		return MissContinue
	}
}

func (s *Shell) loopWithBudget(
	ctx context.Context,
	op string,
	chat ChatFunc,
	messages []Message,
	attempts int,
	allowCompensate bool,
	seed []Message,
) (string, error) {
	var lastRaw string
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		raw, chatErr := chat(ctx, messages)
		if chatErr != nil {
			return raw, wrapChat(op, chatErr)
		}
		lastRaw = raw
		valErr := s.validate(raw)
		if valErr == nil {
			return raw, nil
		}
		lastErr = valErr
		if attempt+1 >= attempts {
			break
		}
		switch s.missAction(valErr) {
		case MissRestart:
			messages = append([]Message{}, seed...)
		default:
			messages = append(messages,
				Message{Role: RoleAssistant, Content: raw},
				Message{Role: RoleUser, Content: s.noteFor(valErr)},
			)
		}
	}
	if lastErr == nil {
		lastErr = ErrExhausted
	}
	return s.finishAfterExhaust(ctx, op, messages, lastRaw, lastErr, seed, chat, allowCompensate)
}

func (s *Shell) finishAfterExhaust(
	ctx context.Context,
	op string,
	messages []Message,
	lastRaw string,
	lastErr error,
	seed []Message,
	chat ChatFunc,
	allowCompensate bool,
) (string, error) {
	if !allowCompensate || s.compensate == nil {
		return lastRaw, exhausted(op, "validation failed", lastErr)
	}
	steer, compErr := s.compensate(ctx, messages, lastRaw, lastErr)
	if compErr != nil {
		return lastRaw, wrapChat(op, compErr)
	}
	if steer.Action != SteerReenter {
		return lastRaw, exhausted(op, "validation failed", lastErr)
	}
	nextChat := chat
	if steer.Chat != nil {
		nextChat = steer.Chat
	}
	nextAttempts := s.maxAttempts
	if steer.Attempts > 0 {
		nextAttempts = steer.Attempts
	}
	nextMessages := messages
	if steer.Restart {
		nextMessages = append([]Message{}, seed...)
	}
	return s.loopWithBudget(ctx, op, nextChat, nextMessages, nextAttempts, false, seed)
}

func (s *Shell) noteFor(err error) string {
	if s == nil || s.completeNote == nil {
		return DefaultCompleteNote(err)
	}
	note := strings.TrimSpace(s.completeNote(err))
	if note == "" {
		return DefaultCompleteNote(err)
	}
	return note
}
