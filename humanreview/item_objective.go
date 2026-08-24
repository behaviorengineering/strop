package humanreview

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ItemObjective is the persisted pattern key for one root entity + job.
// Payload JSON shape is owned by the pipeline pack.
type ItemObjective struct {
	PipelineType PipelineType           `json:"pipeline_type"`
	RootEntityID uuid.UUID              `json:"root_entity_id"`
	Job          Job                    `json:"job"`
	Payload      map[string]interface{} `json:"payload"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
}

// ItemObjectiveStore persists item objectives. Get returns (nil, nil) when missing.
type ItemObjectiveStore interface {
	Upsert(ctx context.Context, objective *ItemObjective) error
	Get(
		ctx context.Context,
		pipelineType PipelineType,
		rootEntityID uuid.UUID,
		job Job,
	) (*ItemObjective, error)
}

type objectiveKey struct {
	pipeline PipelineType
	root     uuid.UUID
	job      Job
}

// MemoryItemObjectiveStore is an in-memory store for tests. Unique on pipeline+root+job.
type MemoryItemObjectiveStore struct {
	mu    sync.Mutex
	byKey map[objectiveKey]*ItemObjective
}

// NewMemoryItemObjectiveStore creates an empty in-memory objective store.
func NewMemoryItemObjectiveStore() *MemoryItemObjectiveStore {
	return &MemoryItemObjectiveStore{byKey: make(map[objectiveKey]*ItemObjective)}
}

// Upsert inserts or replaces the row for (pipeline, root, job).
func (s *MemoryItemObjectiveStore) Upsert(_ context.Context, objective *ItemObjective) error {
	if s == nil || objective == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.byKey == nil {
		s.byKey = make(map[objectiveKey]*ItemObjective)
	}
	key := objectiveKey{pipeline: objective.PipelineType, root: objective.RootEntityID, job: objective.Job}
	now := time.Now()
	copied := cloneItemObjective(objective)
	if existing, ok := s.byKey[key]; ok {
		copied.CreatedAt = existing.CreatedAt
		copied.UpdatedAt = now
	} else {
		if copied.CreatedAt.IsZero() {
			copied.CreatedAt = now
		}
		copied.UpdatedAt = now
	}
	s.byKey[key] = copied
	return nil
}

// Get returns the unique row for (pipeline, root, job), or (nil, nil).
func (s *MemoryItemObjectiveStore) Get(
	_ context.Context,
	pipelineType PipelineType,
	rootEntityID uuid.UUID,
	job Job,
) (*ItemObjective, error) {
	if s == nil {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	found, ok := s.byKey[objectiveKey{pipeline: pipelineType, root: rootEntityID, job: job}]
	if !ok {
		return nil, nil
	}
	return cloneItemObjective(found), nil
}

func cloneItemObjective(src *ItemObjective) *ItemObjective {
	if src == nil {
		return nil
	}
	out := *src
	if src.Payload != nil {
		out.Payload = make(map[string]interface{}, len(src.Payload))
		for key, value := range src.Payload {
			out.Payload[key] = value
		}
	}
	return &out
}
