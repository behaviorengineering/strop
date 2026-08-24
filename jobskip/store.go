// Package jobskip is the portable per-job generate-queue skip contract.
// Apps persist skips and exclude skipped roots from pending generate lists.
package jobskip

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Record is one skipped root for one generator job.
type Record struct {
	RootID    uuid.UUID
	Job       string
	Reason    *string
	CreatedAt time.Time
}

// Item is a skipped root with an app-filled display label.
type Item struct {
	RootID    uuid.UUID
	Label     string
	Reason    *string
	CreatedAt time.Time
}

// Store persists per-job skips. Implementations must not require a strop-level transaction type.
type Store interface {
	Skip(ctx context.Context, rootID uuid.UUID, job string, reason *string) error
	Unskip(ctx context.Context, rootID uuid.UUID, job string) error
	IsSkipped(ctx context.Context, rootID uuid.UUID, job string) (bool, error)
	List(ctx context.Context, job string) ([]Record, error)
}

// Labeler maps skip records to display items. Strop never knows product titles.
type Labeler interface {
	Labels(ctx context.Context, records []Record) ([]Item, error)
}

// Selector is the UI port for choosing one skipped item. proceed is false when the user cancels.
type Selector interface {
	Select(ctx context.Context, items []Item) (rootID uuid.UUID, proceed bool, err error)
}
