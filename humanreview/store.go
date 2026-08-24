package humanreview

import (
	"context"

	"github.com/google/uuid"
)

// Store persists HumanEvaluation rows. Implementations must not require a strop-level transaction type.
// GetByID and GetByRootEntityID return (nil, nil) when the row is missing.
type Store interface {
	Create(ctx context.Context, evaluation *HumanEvaluation) error
	GetByID(ctx context.Context, id uuid.UUID) (*HumanEvaluation, error)
	GetByRootEntityID(ctx context.Context, rootEntityID uuid.UUID, pipelineType PipelineType, job Job) (*HumanEvaluation, error)
	Update(ctx context.Context, evaluation *HumanEvaluation) error
	DeleteByRootEntityID(ctx context.Context, rootEntityID uuid.UUID, pipelineType PipelineType, job Job) error
}
