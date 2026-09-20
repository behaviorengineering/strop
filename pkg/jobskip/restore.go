package jobskip

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// RestoreResult is the outcome of Restore. Empty means the skip archive had no rows.
type RestoreResult struct {
	Empty      bool
	RestoredID uuid.UUID
}

// Restore lists skipped roots for job, asks the selector to pick one, then unskips it.
func Restore(ctx context.Context, store Store, job string, labeler Labeler, selector Selector) (RestoreResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if store == nil {
		return RestoreResult{}, fmt.Errorf("job skip store is required")
	}
	if job == "" {
		return RestoreResult{}, fmt.Errorf("job cannot be empty")
	}
	if labeler == nil {
		return RestoreResult{}, fmt.Errorf("job skip labeler is required")
	}
	if selector == nil {
		return RestoreResult{}, fmt.Errorf("job skip selector is required")
	}

	records, err := store.List(ctx, job)
	if err != nil {
		return RestoreResult{}, fmt.Errorf("list skipped roots: %w", err)
	}
	if len(records) == 0 {
		return RestoreResult{Empty: true}, nil
	}

	items, err := labeler.Labels(ctx, records)
	if err != nil {
		return RestoreResult{}, fmt.Errorf("label skipped roots: %w", err)
	}
	if len(items) == 0 {
		return RestoreResult{}, fmt.Errorf("labeler returned no items for %d skipped root(s)", len(records))
	}

	rootID, proceed, err := selector.Select(ctx, items)
	if err != nil {
		return RestoreResult{}, fmt.Errorf("select skipped root: %w", err)
	}
	if !proceed {
		return RestoreResult{}, nil
	}
	if rootID == uuid.Nil {
		return RestoreResult{}, fmt.Errorf("selected skipped root id is empty")
	}

	if err := store.Unskip(ctx, rootID, job); err != nil {
		return RestoreResult{}, fmt.Errorf("unskip root: %w", err)
	}
	return RestoreResult{RestoredID: rootID}, nil
}
