package humanreview

import (
	"sync"
)

// LearningPack is the pipeline-owned composition-job list. Chain loading and
// attribution stay in the app until later slices; strop must not import product types.
type LearningPack interface {
	PipelineType() PipelineType
	CompositionJobs() []Job
	IsCompositionJob(job Job) bool
}

// StaticLearningPack is a LearningPack with a fixed composition-job list.
type StaticLearningPack struct {
	pipeline PipelineType
	jobs     []Job
}

// NewStaticLearningPack returns a pack for tests and simple pipeline wiring.
func NewStaticLearningPack(pipeline PipelineType, jobs ...Job) *StaticLearningPack {
	copied := append([]Job(nil), jobs...)
	return &StaticLearningPack{pipeline: pipeline, jobs: copied}
}

// PipelineType implements LearningPack.
func (p *StaticLearningPack) PipelineType() PipelineType {
	if p == nil {
		return ""
	}
	return p.pipeline
}

// CompositionJobs implements LearningPack.
func (p *StaticLearningPack) CompositionJobs() []Job {
	if p == nil {
		return nil
	}
	return append([]Job(nil), p.jobs...)
}

// IsCompositionJob implements LearningPack.
func (p *StaticLearningPack) IsCompositionJob(job Job) bool {
	if p == nil {
		return false
	}
	for _, candidate := range p.jobs {
		if candidate == job {
			return true
		}
	}
	return false
}

// LearningPackRegistry maps pipeline type to a learning pack.
type LearningPackRegistry struct {
	mu    sync.RWMutex
	packs map[PipelineType]LearningPack
}

// NewLearningPackRegistry creates an empty registry. Empty is valid.
func NewLearningPackRegistry() *LearningPackRegistry {
	return &LearningPackRegistry{packs: make(map[PipelineType]LearningPack)}
}

// Register stores pack by PipelineType(). Nil pack or empty pipeline type is ignored.
func (r *LearningPackRegistry) Register(pack LearningPack) {
	if r == nil || pack == nil {
		return
	}
	pipeline := pack.PipelineType()
	if pipeline == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.packs == nil {
		r.packs = make(map[PipelineType]LearningPack)
	}
	r.packs[pipeline] = pack
}

// Get returns the registered pack or ErrUnknownLearningPack.
func (r *LearningPackRegistry) Get(pipelineType PipelineType) (LearningPack, error) {
	if r == nil {
		return nil, ErrUnknownLearningPack(pipelineType)
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	pack, ok := r.packs[pipelineType]
	if !ok || pack == nil {
		return nil, ErrUnknownLearningPack(pipelineType)
	}
	return pack, nil
}

var (
	defaultLearningPackRegistryMu sync.RWMutex
	defaultLearningPackRegistry   = NewLearningPackRegistry()
)

// DefaultLearningPackRegistry returns the process-wide registry (packs register at startup).
func DefaultLearningPackRegistry() *LearningPackRegistry {
	defaultLearningPackRegistryMu.RLock()
	defer defaultLearningPackRegistryMu.RUnlock()
	return defaultLearningPackRegistry
}

// GetLearningPack looks up a pack on the default registry.
func GetLearningPack(pipelineType PipelineType) (LearningPack, error) {
	return DefaultLearningPackRegistry().Get(pipelineType)
}
