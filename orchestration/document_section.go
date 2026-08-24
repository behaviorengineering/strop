package orchestration

import "strings"

// DocumentSectionDefinition describes ordered string sections for section-walk refinement
// (polish, translation). One section ID per field-walk phase.
type DocumentSectionDefinition struct {
	Name         string
	SectionIDs   []string // Reader / refinement order.
	MaxAttempts  int      // Per-section default (1 when unset).
	MinPassScore float64  // Field-walk gate (8.0 when unset).
}

// PhaseDefs maps section IDs to CompositionStrategy phases for NewFieldWalkStrategy.
func (d DocumentSectionDefinition) PhaseDefs() []PhaseDef {
	maxAttempts := d.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1
	}
	out := make([]PhaseDef, 0, len(d.SectionIDs))
	for _, id := range d.SectionIDs {
		displayName := id
		out = append(out, PhaseDef{
			ID:          PhaseID(id),
			DisplayName: displayName,
			MaxAttempts: maxAttempts,
		})
	}
	return out
}

// MinPass returns the minimum weighted score to pass a section phase.
func (d DocumentSectionDefinition) MinPass() float64 {
	if d.MinPassScore <= 0 {
		return 8.0
	}
	return d.MinPassScore
}

// LockedSectionsBefore returns non-empty prior sections from draft (earlier in SectionIDs than activeID).
func (d DocumentSectionDefinition) LockedSectionsBefore(activeID string, draft map[string]string) map[string]string {
	activeID = strings.TrimSpace(activeID)
	out := make(map[string]string)
	for _, id := range d.SectionIDs {
		if id == activeID {
			break
		}
		if draft == nil {
			continue
		}
		if v := strings.TrimSpace(draft[id]); v != "" {
			out[id] = v
		}
	}
	return out
}
