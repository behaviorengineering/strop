package orchestration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDocumentArcDefinition_PhaseWalkOwnedFields(t *testing.T) {
	arc := DocumentArcDefinition{
		Name: "test_doc",
		Phases: []DocumentArcPhaseSpec{
			{ID: "a", ActiveOutputFields: []string{"f1", "f2"}},
			{ID: "b", ActiveOutputFields: []string{"f3"}},
		},
	}
	owned := arc.PhaseWalkOwnedFields()
	require.Len(t, owned, 2)
	assert.Equal(t, []string{"f1", "f2"}, owned[PhaseID("a")])
	assert.Equal(t, []string{"f3"}, owned[PhaseID("b")])
}

func TestDocumentArcDefinition_OrchestrationPhases(t *testing.T) {
	arc := DocumentArcDefinition{
		Phases: []DocumentArcPhaseSpec{
			{ID: "skim", DisplayName: "skim", MaxAttempts: 3},
			{ID: "warmth", DisplayName: "warmth", MaxAttempts: 0},
		},
	}
	phases := arc.OrchestrationPhases()
	require.Len(t, phases, 2)
	assert.Equal(t, PhaseID("skim"), phases[0].ID)
	assert.Equal(t, 3, phases[0].MaxAttempts)
	assert.Equal(t, 1, phases[1].MaxAttempts)
}

func TestDocumentArcDefinition_LockedOutputFieldsBefore(t *testing.T) {
	arc := DocumentArcDefinition{
		Phases: []DocumentArcPhaseSpec{
			{ID: "skim", ActiveOutputFields: []string{"plan", "desc"}},
			{ID: "warmth", ActiveOutputFields: []string{"plan", "fluff"}},
			{ID: "depth", ActiveOutputFields: []string{"plan", "depth"}},
		},
	}
	assert.Empty(t, arc.LockedOutputFieldsBefore("skim"))
	assert.Equal(t, []string{"plan", "desc"}, arc.LockedOutputFieldsBefore("warmth"))
	assert.Equal(t, []string{"plan", "desc", "plan", "fluff"}, arc.LockedOutputFieldsBefore("depth"))
}

func TestDocumentArcDefinition_PhaseMinPassScore(t *testing.T) {
	arc := DocumentArcDefinition{
		Phases: []DocumentArcPhaseSpec{
			{ID: "skim", MinPassScore: 7.5},
		},
	}
	assert.Equal(t, 7.5, arc.PhaseMinPassScore("skim"))
	assert.Equal(t, 8.0, arc.PhaseMinPassScore("unknown"))
}

func TestDocumentArcDefinition_PreviousVersionDisplayFields(t *testing.T) {
	arc := DocumentArcDefinition{
		Phases: []DocumentArcPhaseSpec{
			{ID: "skim", ActiveOutputFields: []string{"phase_plan", "description", "tldr"}},
		},
	}
	fields := arc.PreviousVersionDisplayFields("skim", "phase_plan")
	assert.Equal(t, []string{"description", "tldr"}, fields)
	assert.Nil(t, arc.PreviousVersionDisplayFields("missing"))
}

func TestDocumentArcDefinition_PhaseSpec(t *testing.T) {
	arc := DocumentArcDefinition{
		Phases: []DocumentArcPhaseSpec{{ID: "Skim", DisplayName: "skim"}},
	}
	spec := arc.PhaseSpec("SKIM")
	require.NotNil(t, spec)
	assert.Equal(t, "Skim", spec.ID)
	assert.Nil(t, arc.PhaseSpec("nope"))
}
