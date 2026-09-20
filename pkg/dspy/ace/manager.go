package ace

import (
	"context"
	"fmt"
	"strings"

	dspyace "github.com/XiaoConstantine/dspy-go/pkg/agents/ace"
)

// Outcome aliases for portable credit assignment.
type Outcome = dspyace.Outcome

const (
	OutcomeSuccess Outcome = dspyace.OutcomeSuccess
	OutcomePartial Outcome = dspyace.OutcomePartial
	OutcomeFailure Outcome = dspyace.OutcomeFailure
)

// CiteInstruction is a portable one-liner appended with the playbook so generators
// can cite short codes for ACE helpful/harmful credit. No product prompts.
const CiteInstruction = "Cite learnings by short code (e.g. [L001]) in rationale when applying them."

// Manager wraps dspy-go ACE for one long-running session of the same entity+job.
type Manager struct {
	inner    *dspyace.Manager
	entityID string
	job      string
}

// TrajectoryRecorder records steps for one refinement version (or item-round).
type TrajectoryRecorder struct {
	inner *dspyace.TrajectoryRecorder
}

// NewManager constructs a session Manager bound to entityID and job.
// The host owns Close at session end. Orchestration loops must not New/Close.
func NewManager(cfg Config, entityID, job string) (*Manager, error) {
	cfg = cfg.Defaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	entityID = strings.TrimSpace(entityID)
	job = strings.TrimSpace(job)
	if entityID == "" {
		return nil, fmt.Errorf("ace: entityID is required")
	}
	if job == "" {
		return nil, fmt.Errorf("ace: job is required")
	}

	dspCfg := dspyace.Config{
		Enabled:           true,
		LearningsPath:     cfg.LearningsPath,
		AsyncReflection:   cfg.AsyncReflection,
		CurationFrequency: cfg.CurationFrequency,
		MinConfidence:     cfg.MinConfidence,
		MaxTokens:         cfg.MaxTokens,
		// Keep dspy-go prune/similarity defaults.
		PruneMinRatio:       0.3,
		PruneMinUsage:       5,
		SimilarityThreshold: 0.85,
	}
	reflector := dspyace.NewUnifiedReflector(nil, dspyace.NewSimpleReflector())
	inner, err := dspyace.NewManager(dspCfg, reflector)
	if err != nil {
		return nil, fmt.Errorf("ace: NewManager: %w", err)
	}
	return &Manager{
		inner:    inner,
		entityID: entityID,
		job:      job,
	}, nil
}

// EntityID returns the bound entity for this session.
func (m *Manager) EntityID() string {
	if m == nil {
		return ""
	}
	return m.entityID
}

// Job returns the bound job for this session.
func (m *Manager) Job() string {
	if m == nil {
		return ""
	}
	return m.job
}

// CheckBinding fails closed when entityID or job do not match this Manager.
func (m *Manager) CheckBinding(entityID, job string) error {
	if m == nil {
		return nil
	}
	entityID = strings.TrimSpace(entityID)
	job = strings.TrimSpace(job)
	if entityID != m.entityID || job != m.job {
		return fmt.Errorf("ace: manager binding mismatch: have entity=%q job=%q, got entity=%q job=%q",
			m.entityID, m.job, entityID, job)
	}
	return nil
}

// StartTrajectory begins recording one attempt. agentID/taskType should match the binding.
func (m *Manager) StartTrajectory(agentID, taskType, query string) *TrajectoryRecorder {
	if m == nil || m.inner == nil {
		return nil
	}
	return &TrajectoryRecorder{inner: m.inner.StartTrajectory(agentID, taskType, query)}
}

// EndTrajectory finalizes a trajectory. Safe no-op when m or recorder is nil.
func (m *Manager) EndTrajectory(ctx context.Context, recorder *TrajectoryRecorder, outcome Outcome) {
	if m == nil || m.inner == nil || recorder == nil || recorder.inner == nil {
		return
	}
	m.inner.EndTrajectory(ctx, recorder.inner, outcome)
}

// LearningsContext returns the playbook text for prompt injection.
func (m *Manager) LearningsContext() string {
	if m == nil || m.inner == nil {
		return ""
	}
	return m.inner.LearningsContext()
}

// Close shuts down the underlying manager. Host calls this at session end.
func (m *Manager) Close() error {
	if m == nil || m.inner == nil {
		return nil
	}
	return m.inner.Close()
}

// RecordStep captures one action on the trajectory.
func (r *TrajectoryRecorder) RecordStep(action, tool, reasoning string, input, output map[string]any, err error) {
	if r == nil || r.inner == nil {
		return
	}
	r.inner.RecordStep(action, tool, reasoning, input, output, err)
}

// MergePlaybookIntoGuides appends ACE playbook (plus cite instruction) onto retrieved_guides text.
func MergePlaybookIntoGuides(existingGuides, playbook string) string {
	playbook = strings.TrimSpace(playbook)
	if playbook == "" {
		return existingGuides
	}
	aceBlock := CiteInstruction + "\n" + playbook
	existingGuides = strings.TrimSpace(existingGuides)
	if existingGuides == "" {
		return aceBlock
	}
	return existingGuides + "\n\n" + aceBlock
}
