package humanreview

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"
)

// PipelineHistoryBuilder builds pipeline history for a root entity.
type PipelineHistoryBuilder interface {
	BuildHistory(ctx context.Context, originationID uuid.UUID) (PipelineHistory, error)
}

// PipelineHistoryBuilderFactory returns the builder for a review job.
type PipelineHistoryBuilderFactory interface {
	GetBuilder(job Job) (PipelineHistoryBuilder, error)
}

// MapHistoryBuilderFactory routes jobs to registered builders.
type MapHistoryBuilderFactory struct {
	mu       sync.RWMutex
	builders map[Job]PipelineHistoryBuilder
}

// NewMapHistoryBuilderFactory creates an empty map-based factory.
func NewMapHistoryBuilderFactory() *MapHistoryBuilderFactory {
	return &MapHistoryBuilderFactory{builders: make(map[Job]PipelineHistoryBuilder)}
}

// Register associates a job with a history builder.
func (f *MapHistoryBuilderFactory) Register(job Job, builder PipelineHistoryBuilder) {
	if f == nil || builder == nil {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.builders == nil {
		f.builders = make(map[Job]PipelineHistoryBuilder)
	}
	f.builders[job] = builder
}

// GetBuilder returns the builder for the job.
func (f *MapHistoryBuilderFactory) GetBuilder(job Job) (PipelineHistoryBuilder, error) {
	if f == nil {
		return nil, fmt.Errorf("builder not found for job: %s", job)
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	b, ok := f.builders[job]
	if !ok || b == nil {
		return nil, fmt.Errorf("builder not found for job: %s", job)
	}
	return b, nil
}
