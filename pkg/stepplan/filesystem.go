package stepplan

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// FileStore roots plans under Root/plans/<planID>/.
type FileStore struct {
	Root string
}

// Compile-time check: FileStore implements Store and CheckpointInvalidator.
var (
	_ Store                 = (*FileStore)(nil)
	_ CheckpointInvalidator = (*FileStore)(nil)
)

// NewFileStore returns a FileStore for the given Root (e.g. seed work-story dir).
func NewFileStore(root string) (*FileStore, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, fmt.Errorf("stepplan: root is required")
	}
	return &FileStore{Root: filepath.Clean(root)}, nil
}

// PlansDir is Root/plans.
func (s *FileStore) PlansDir() string {
	return filepath.Join(s.Root, "plans")
}

// PlanDir returns the absolute path of a plan directory.
func (s *FileStore) PlanDir(planID string) (string, error) {
	if err := validateID(planID, "plan id"); err != nil {
		return "", err
	}
	return filepath.Join(s.PlansDir(), strings.TrimSpace(planID)), nil
}

func (s *FileStore) stepsDir(planID string) (string, error) {
	dir, err := s.PlanDir(planID)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "steps"), nil
}

// SavePlan writes plan.json under plans/<id>/.
func (s *FileStore) SavePlan(ctx context.Context, plan *Plan) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s == nil || s.Root == "" {
		return fmt.Errorf("stepplan: store root is required")
	}
	if err := Validate(plan); err != nil {
		return err
	}
	dir, err := s.PlanDir(plan.ID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("stepplan: mkdir %s: %w", dir, err)
	}
	plan.UpdatedAt = time.Now().UTC()
	if plan.CreatedAt.IsZero() {
		plan.CreatedAt = plan.UpdatedAt
	}
	if strings.TrimSpace(plan.Status) == "" {
		plan.Status = PlanStatusOpen
	}
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return fmt.Errorf("stepplan: marshal plan: %w", err)
	}
	path := filepath.Join(dir, FilePlan)
	if err := os.WriteFile(path, data, 0o640); err != nil {
		return fmt.Errorf("stepplan: write plan: %w", err)
	}
	return nil
}

// LoadPlan reads plan.json.
func (s *FileStore) LoadPlan(ctx context.Context, planID string) (*Plan, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s == nil || s.Root == "" {
		return nil, fmt.Errorf("stepplan: store root is required")
	}
	dir, err := s.PlanDir(planID)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, FilePlan)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: plan %q", ErrNotFound, planID)
		}
		return nil, fmt.Errorf("stepplan: read plan: %w", err)
	}
	var plan Plan
	if err := json.Unmarshal(data, &plan); err != nil {
		return nil, fmt.Errorf("stepplan: unmarshal plan: %w", err)
	}
	if plan.ID == "" {
		plan.ID = strings.TrimSpace(planID)
	}
	if err := Validate(&plan); err != nil {
		return nil, err
	}
	return &plan, nil
}

// SaveStep writes steps/<checkpointKey>.json.
func (s *FileStore) SaveStep(ctx context.Context, cp *Checkpoint) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s == nil || s.Root == "" {
		return fmt.Errorf("stepplan: store root is required")
	}
	if cp == nil {
		return fmt.Errorf("stepplan: checkpoint is nil")
	}
	if err := validateID(cp.PlanID, "plan id"); err != nil {
		return err
	}
	key := strings.TrimSpace(cp.CheckpointKey)
	if key == "" {
		key = strings.TrimSpace(cp.StepID)
		cp.CheckpointKey = key
	}
	if err := validateID(key, "checkpoint key"); err != nil {
		return err
	}
	switch cp.Status {
	case StepStatusComplete, StepStatusFailed:
	default:
		return fmt.Errorf("stepplan: invalid checkpoint status %q", cp.Status)
	}
	stepsDir, err := s.stepsDir(cp.PlanID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(stepsDir, 0o700); err != nil {
		return fmt.Errorf("stepplan: mkdir %s: %w", stepsDir, err)
	}
	if cp.CompletedAt.IsZero() {
		cp.CompletedAt = time.Now().UTC()
	} else {
		cp.CompletedAt = cp.CompletedAt.UTC()
	}
	data, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return fmt.Errorf("stepplan: marshal checkpoint: %w", err)
	}
	path := filepath.Join(stepsDir, key+".json")
	if err := os.WriteFile(path, data, 0o640); err != nil {
		return fmt.Errorf("stepplan: write checkpoint: %w", err)
	}
	return nil
}

// LoadStep reads a checkpoint file.
func (s *FileStore) LoadStep(ctx context.Context, planID, checkpointKey string) (*Checkpoint, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s == nil || s.Root == "" {
		return nil, fmt.Errorf("stepplan: store root is required")
	}
	if err := validateID(planID, "plan id"); err != nil {
		return nil, err
	}
	if err := validateID(checkpointKey, "checkpoint key"); err != nil {
		return nil, err
	}
	stepsDir, err := s.stepsDir(planID)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(stepsDir, strings.TrimSpace(checkpointKey)+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: plan %q step %q", ErrNotFound, planID, checkpointKey)
		}
		return nil, fmt.Errorf("stepplan: read checkpoint: %w", err)
	}
	var cp Checkpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return nil, fmt.Errorf("stepplan: unmarshal checkpoint: %w", err)
	}
	return &cp, nil
}

// ListCompleted returns sorted checkpoint keys with status complete.
func (s *FileStore) ListCompleted(ctx context.Context, planID string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s == nil || s.Root == "" {
		return nil, fmt.Errorf("stepplan: store root is required")
	}
	stepsDir, err := s.stepsDir(planID)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(stepsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("stepplan: list steps: %w", err)
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		key := strings.TrimSuffix(e.Name(), ".json")
		cp, err := s.LoadStep(ctx, planID, key)
		if err != nil {
			continue
		}
		if cp.Status == StepStatusComplete {
			out = append(out, key)
		}
	}
	sort.Strings(out)
	return out, nil
}

// DeleteStep removes steps/<checkpointKey>.json. Missing files are ignored.
func (s *FileStore) DeleteStep(ctx context.Context, planID, checkpointKey string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s == nil || s.Root == "" {
		return fmt.Errorf("stepplan: store root is required")
	}
	if err := validateID(planID, "plan id"); err != nil {
		return err
	}
	if err := validateID(checkpointKey, "checkpoint key"); err != nil {
		return err
	}
	stepsDir, err := s.stepsDir(planID)
	if err != nil {
		return err
	}
	path := filepath.Join(stepsDir, strings.TrimSpace(checkpointKey)+".json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("stepplan: delete checkpoint: %w", err)
	}
	return nil
}

// DeleteStepsAfter removes checkpoints for plan.Steps[fromIndex:] (inclusive of fromIndex).
func (s *FileStore) DeleteStepsAfter(ctx context.Context, plan *Plan, fromIndex int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if plan == nil {
		return fmt.Errorf("stepplan: plan is nil")
	}
	if fromIndex < 0 {
		fromIndex = 0
	}
	for i := fromIndex; i < len(plan.Steps); i++ {
		key := plan.Steps[i].EffectiveCheckpointKey()
		if err := s.DeleteStep(ctx, plan.ID, key); err != nil {
			return err
		}
	}
	return nil
}
