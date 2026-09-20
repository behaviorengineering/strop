package humanreview

import (
	"fmt"
	"sync"
)

// JobStepRegistry maps review jobs to their generation steps.
type JobStepRegistry struct {
	mu    sync.RWMutex
	steps map[Job]Step
}

// NewJobStepRegistry creates an empty registry.
func NewJobStepRegistry() *JobStepRegistry {
	return &JobStepRegistry{steps: make(map[Job]Step)}
}

// Register associates a job with its step. Later calls overwrite.
func (r *JobStepRegistry) Register(job Job, step Step) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.steps == nil {
		r.steps = make(map[Job]Step)
	}
	r.steps[job] = step
}

// StepForJob returns the registered step for a job.
func (r *JobStepRegistry) StepForJob(job Job) (Step, error) {
	if r == nil {
		return "", fmt.Errorf("unknown job: %s", job)
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	step, ok := r.steps[job]
	if !ok {
		return "", fmt.Errorf("unknown job: %s", job)
	}
	return step, nil
}

var (
	defaultJobStepRegistryMu sync.RWMutex
	defaultJobStepRegistry   = NewJobStepRegistry()
)

// DefaultJobStepRegistry returns the process-wide registry (packs register at startup).
func DefaultJobStepRegistry() *JobStepRegistry {
	defaultJobStepRegistryMu.RLock()
	defer defaultJobStepRegistryMu.RUnlock()
	return defaultJobStepRegistry
}

// StepForJob looks up a job on the default registry.
func StepForJob(job Job) (Step, error) {
	return DefaultJobStepRegistry().StepForJob(job)
}

// GetStepForJob is an alias for StepForJob (legacy name used by app code).
func GetStepForJob(job Job) (Step, error) {
	return StepForJob(job)
}
