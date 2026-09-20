// Package runreport collects structured execution traces for pipeline runs and writes
// short-lived JSON debug files. Pipelines attach config to context, optionally provide
// Meta via MetaProvider, and orchestration/DSPy middleware record steps automatically.
package runreport

import (
	"time"

	"github.com/google/uuid"
)

// StepKind categorizes one recorded event in a run timeline.
type StepKind string

const (
	StepRunStart            StepKind = "run_start"
	StepRunFinish           StepKind = "run_finish"
	StepRefinementIteration StepKind = "refinement_iteration"
	StepCompositionPhase    StepKind = "composition_phase"
	StepModuleCall          StepKind = "module_call"
	StepEvaluator           StepKind = "evaluator"
	StepAlignment           StepKind = "alignment"
	StepWarning             StepKind = "warning"
	StepHealingRetry        StepKind = "healing_retry"
)

// Outcome summarizes how the run ended.
type Outcome string

const (
	OutcomeSuccess Outcome = "success"
	OutcomeFailed  Outcome = "failed"
)

// Meta identifies a pipeline job run (who/what was processed).
type Meta struct {
	Pipeline  string    `json:"pipeline"`
	Job       string    `json:"job"`
	EntityID  string    `json:"entity_id"`
	Version   int       `json:"version,omitempty"`
	RunID     string    `json:"run_id"`
	StartedAt time.Time `json:"started_at"`
}

// Step is one chronological event during a run.
type Step struct {
	At         time.Time              `json:"at"`
	Kind       StepKind               `json:"kind"`
	Message    string                 `json:"message,omitempty"`
	Phase      string                 `json:"phase,omitempty"`
	Module     string                 `json:"module,omitempty"`
	Attempt    int                    `json:"attempt,omitempty"`
	OK         *bool                  `json:"ok,omitempty"`
	Error      string                 `json:"error,omitempty"`
	Score      *float64               `json:"score,omitempty"`
	DurationMS int64                  `json:"duration_ms,omitempty"`
	Details    map[string]interface{} `json:"details,omitempty"`
}

// Report is the persisted execution trace document.
type Report struct {
	Meta     Meta    `json:"meta"`
	Outcome  Outcome `json:"outcome"`
	Error    string  `json:"error,omitempty"`
	Steps    []Step  `json:"steps"`
	FilePath string  `json:"file_path,omitempty"`
}

// MetaProvider is implemented by orchestration.RefinementStrategy implementations
// that want rich run-report metadata. Optional — loops fall back to entity ID only.
type MetaProvider interface {
	RunReportMeta() Meta
}

// NewMeta builds a Meta with a fresh run ID and timestamp.
func NewMeta(pipeline, job, entityID string, version int) Meta {
	return Meta{
		Pipeline:  pipeline,
		Job:       job,
		EntityID:  entityID,
		Version:   version,
		RunID:     uuid.New().String(),
		StartedAt: time.Now().UTC(),
	}
}

// PipelineJobMeta returns partial metadata; ResolveMeta fills entity ID, version, and run ID.
func PipelineJobMeta(pipeline, job string) Meta {
	return Meta{Pipeline: pipeline, Job: job}
}
