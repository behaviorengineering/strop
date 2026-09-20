package agentsession

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Store roots agent sessions under Root/sessions/<id>/.
type Store struct {
	Root string
}

// New returns a Store for the given Root (e.g. ~/.config/gitboard/agents).
func New(root string) (*Store, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, fmt.Errorf("agentsession: root is required")
	}
	return &Store{Root: filepath.Clean(root)}, nil
}

// SessionsDir is Root/sessions.
func (s *Store) SessionsDir() string {
	return filepath.Join(s.Root, "sessions")
}

// Dir returns the absolute path of a session directory.
func (s *Store) Dir(id string) (string, error) {
	id, err := sanitizeID(id)
	if err != nil {
		return "", err
	}
	return filepath.Join(s.SessionsDir(), id), nil
}

// Create makes sessions/<uuid>/ (0700), writes meta.json, and returns the meta.
func (s *Store) Create(ctx context.Context, kind string, extra map[string]any) (*Meta, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s == nil || s.Root == "" {
		return nil, fmt.Errorf("agentsession: store root is required")
	}
	id := uuid.NewString()
	dir, err := s.Dir(id)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("agentsession: mkdir %s: %w", dir, err)
	}
	now := time.Now().UTC()
	meta := &Meta{
		ID:        id,
		Kind:      strings.TrimSpace(kind),
		Status:    StatusOpen,
		CreatedAt: now,
		UpdatedAt: now,
		Extra:     cloneMap(extra),
	}
	if err := s.writeMeta(dir, meta); err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	return meta, nil
}

// Load reads meta.json for the session.
func (s *Store) Load(ctx context.Context, id string) (*Meta, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	dir, err := s.Dir(id)
	if err != nil {
		return nil, err
	}
	return s.readMeta(dir)
}

// List returns session metas under sessions/, newest UpdatedAt first.
func (s *Store) List(ctx context.Context, opts ListOpts) ([]Meta, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root := s.SessionsDir()
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("agentsession: list %s: %w", root, err)
	}
	kind := strings.TrimSpace(opts.Kind)
	status := strings.TrimSpace(opts.Status)
	out := make([]Meta, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		meta, err := s.readMeta(filepath.Join(root, e.Name()))
		if err != nil {
			continue
		}
		if kind != "" && meta.Kind != kind {
			continue
		}
		if status != "" && meta.Status != status {
			continue
		}
		out = append(out, *meta)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out, nil
}

// AppendTurn appends one JSON line to transcript.jsonl and bumps UpdatedAt.
func (s *Store) AppendTurn(ctx context.Context, id string, turn Turn) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	dir, err := s.Dir(id)
	if err != nil {
		return err
	}
	meta, err := s.readMeta(dir)
	if err != nil {
		return err
	}
	if meta.Status == StatusClosed {
		return fmt.Errorf("agentsession: session %s is closed", id)
	}
	if turn.At.IsZero() {
		turn.At = time.Now().UTC()
	} else {
		turn.At = turn.At.UTC()
	}
	turn.Role = strings.TrimSpace(turn.Role)
	if turn.Role == "" {
		return fmt.Errorf("agentsession: turn role is required")
	}
	line, err := json.Marshal(turn)
	if err != nil {
		return fmt.Errorf("agentsession: marshal turn: %w", err)
	}
	path := filepath.Join(dir, FileTranscript)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
	if err != nil {
		return fmt.Errorf("agentsession: open transcript: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("agentsession: write transcript: %w", err)
	}
	return s.touch(dir, meta)
}

// ReadTurns loads all transcript lines (empty if no file).
func (s *Store) ReadTurns(ctx context.Context, id string) ([]Turn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	dir, err := s.Dir(id)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, FileTranscript)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("agentsession: read transcript: %w", err)
	}
	lines := strings.Split(string(data), "\n")
	out := make([]Turn, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var t Turn
		if err := json.Unmarshal([]byte(line), &t); err != nil {
			return nil, fmt.Errorf("agentsession: parse transcript line: %w", err)
		}
		out = append(out, t)
	}
	return out, nil
}

// SaveJSON writes name (basename only) as indented JSON under the session dir.
func (s *Store) SaveJSON(ctx context.Context, id, name string, v any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	dir, err := s.Dir(id)
	if err != nil {
		return err
	}
	base, err := safeBlobName(name)
	if err != nil {
		return err
	}
	meta, err := s.readMeta(dir)
	if err != nil {
		return err
	}
	if meta.Status == StatusClosed {
		return fmt.Errorf("agentsession: session %s is closed", id)
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("agentsession: marshal %s: %w", base, err)
	}
	path := filepath.Join(dir, base)
	if err := os.WriteFile(path, data, 0o640); err != nil {
		return fmt.Errorf("agentsession: write %s: %w", base, err)
	}
	return s.touch(dir, meta)
}

// LoadJSON reads a JSON blob into dest.
func (s *Store) LoadJSON(ctx context.Context, id, name string, dest any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	dir, err := s.Dir(id)
	if err != nil {
		return err
	}
	base, err := safeBlobName(name)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, base)
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("agentsession: read %s: %w", base, err)
	}
	if err := json.Unmarshal(data, dest); err != nil {
		return fmt.Errorf("agentsession: unmarshal %s: %w", base, err)
	}
	return nil
}

// Close sets status=closed and bumps UpdatedAt.
func (s *Store) Close(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	dir, err := s.Dir(id)
	if err != nil {
		return err
	}
	meta, err := s.readMeta(dir)
	if err != nil {
		return err
	}
	meta.Status = StatusClosed
	meta.UpdatedAt = time.Now().UTC()
	return s.writeMeta(dir, meta)
}

// Prune removes session directories per opts.
func (s *Store) Prune(ctx context.Context, opts PruneOpts) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	metas, err := s.List(ctx, ListOpts{})
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, m := range metas {
		if opts.ClosedOnly && m.Status != StatusClosed {
			continue
		}
		if opts.MaxAge > 0 && !m.UpdatedAt.IsZero() && now.Sub(m.UpdatedAt) > opts.MaxAge {
			dir, err := s.Dir(m.ID)
			if err != nil {
				return err
			}
			if err := os.RemoveAll(dir); err != nil {
				return fmt.Errorf("agentsession: prune %s: %w", m.ID, err)
			}
		}
	}
	if opts.KeepPerKind <= 0 {
		return nil
	}
	metas, err = s.List(ctx, ListOpts{})
	if err != nil {
		return err
	}
	byKind := map[string][]Meta{}
	for _, m := range metas {
		if opts.ClosedOnly && m.Status != StatusClosed {
			continue
		}
		key := m.Kind
		byKind[key] = append(byKind[key], m)
	}
	for _, group := range byKind {
		if len(group) <= opts.KeepPerKind {
			continue
		}
		// List already sorted newest first.
		for _, m := range group[opts.KeepPerKind:] {
			dir, err := s.Dir(m.ID)
			if err != nil {
				return err
			}
			if err := os.RemoveAll(dir); err != nil {
				return fmt.Errorf("agentsession: prune keep %s: %w", m.ID, err)
			}
		}
	}
	return nil
}

func (s *Store) writeMeta(dir string, meta *Meta) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("agentsession: marshal meta: %w", err)
	}
	path := filepath.Join(dir, FileMeta)
	if err := os.WriteFile(path, data, 0o640); err != nil {
		return fmt.Errorf("agentsession: write meta: %w", err)
	}
	return nil
}

func (s *Store) readMeta(dir string) (*Meta, error) {
	path := filepath.Join(dir, FileMeta)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("agentsession: read meta: %w", err)
	}
	var meta Meta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("agentsession: unmarshal meta: %w", err)
	}
	if meta.ID == "" {
		meta.ID = filepath.Base(dir)
	}
	return &meta, nil
}

func (s *Store) touch(dir string, meta *Meta) error {
	meta.UpdatedAt = time.Now().UTC()
	return s.writeMeta(dir, meta)
}

func sanitizeID(id string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", fmt.Errorf("agentsession: id is required")
	}
	if strings.Contains(id, "/") || strings.Contains(id, `\`) || id == "." || id == ".." {
		return "", fmt.Errorf("agentsession: invalid id %q", id)
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '-', r == '_':
		default:
			return "", fmt.Errorf("agentsession: invalid id %q", id)
		}
	}
	return id, nil
}

func safeBlobName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("agentsession: blob name is required")
	}
	base := filepath.Base(name)
	if base != name || base == "." || base == ".." {
		return "", fmt.Errorf("agentsession: invalid blob name %q", name)
	}
	if base == FileMeta || base == FileTranscript {
		return "", fmt.Errorf("agentsession: reserved blob name %q", base)
	}
	return base, nil
}

func cloneMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
