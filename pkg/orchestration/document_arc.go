package orchestration

import "strings"

// DocumentArcPhaseSpec binds one phased composition pass to orchestration metadata.
// Prompts and validators stay in the app; this struct holds portable arc shape only.
type DocumentArcPhaseSpec struct {
	ID                 string
	DisplayName        string
	MaxAttempts        int
	ActiveOutputFields []string // field keys this phase may write
	MinPassScore       float64  // default 8.0 when unset; runner may override
}

// DocumentArcDefinition describes reader order and phased assembly for one document type.
type DocumentArcDefinition struct {
	Name   string
	Phases []DocumentArcPhaseSpec
}

// PhaseSpec returns the phase spec for id, or nil when unknown.
func (a DocumentArcDefinition) PhaseSpec(phaseID string) *DocumentArcPhaseSpec {
	phaseID = strings.TrimSpace(strings.ToLower(phaseID))
	for i := range a.Phases {
		if strings.EqualFold(a.Phases[i].ID, phaseID) {
			return &a.Phases[i]
		}
	}
	return nil
}

// PhaseWalkOwnedFields maps each arc phase to its writable field keys for PhaseWalkStrategy.
func (a DocumentArcDefinition) PhaseWalkOwnedFields() PhaseWalkOwnedFields {
	out := make(PhaseWalkOwnedFields, len(a.Phases))
	for _, phase := range a.Phases {
		fields := append([]string(nil), phase.ActiveOutputFields...)
		out[PhaseID(phase.ID)] = fields
	}
	return out
}

// OrchestrationPhases maps this arc to the shared composition loop.
func (a DocumentArcDefinition) OrchestrationPhases() []PhaseDef {
	out := make([]PhaseDef, 0, len(a.Phases))
	for _, phase := range a.Phases {
		maxAttempts := phase.MaxAttempts
		if maxAttempts <= 0 {
			maxAttempts = 1
		}
		out = append(out, PhaseDef{
			ID:          PhaseID(phase.ID),
			DisplayName: phase.DisplayName,
			MaxAttempts: maxAttempts,
		})
	}
	return out
}

// PhaseMinPassScore returns the minimum weighted score to pass a composition phase.
func (a DocumentArcDefinition) PhaseMinPassScore(phaseID string) float64 {
	spec := a.PhaseSpec(phaseID)
	if spec == nil || spec.MinPassScore <= 0 {
		return 8.0
	}
	return spec.MinPassScore
}

// LockedOutputFieldsBefore returns field keys owned by phases before phaseID.
func (a DocumentArcDefinition) LockedOutputFieldsBefore(phaseID string) []string {
	var locked []string
	for _, phase := range a.Phases {
		if strings.EqualFold(phase.ID, phaseID) {
			break
		}
		locked = append(locked, phase.ActiveOutputFields...)
	}
	return locked
}

// PreviousVersionDisplayFields returns active prose fields for a phase, excluding excludeFields.
func (a DocumentArcDefinition) PreviousVersionDisplayFields(phaseID string, excludeFields ...string) []string {
	spec := a.PhaseSpec(phaseID)
	if spec == nil {
		return nil
	}
	excluded := make(map[string]struct{}, len(excludeFields))
	for _, f := range excludeFields {
		excluded[f] = struct{}{}
	}
	out := make([]string, 0, len(spec.ActiveOutputFields))
	for _, field := range spec.ActiveOutputFields {
		if _, skip := excluded[field]; skip {
			continue
		}
		out = append(out, field)
	}
	return out
}
