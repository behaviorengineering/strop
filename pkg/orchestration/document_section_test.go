package orchestration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDocumentSectionDefinition_PhaseDefs(t *testing.T) {
	def := DocumentSectionDefinition{
		Name:         "test_doc",
		SectionIDs:   []string{"teaser", "body", "cta"},
		MaxAttempts:  3,
		MinPassScore: 7.0,
	}
	phases := def.PhaseDefs()
	require.Len(t, phases, 3)
	assert.Equal(t, PhaseID("teaser"), phases[0].ID)
	assert.Equal(t, 3, phases[0].MaxAttempts)
	assert.Equal(t, PhaseID("cta"), phases[2].ID)
}

func TestDocumentSectionDefinition_PhaseDefs_defaultMaxAttempts(t *testing.T) {
	def := DocumentSectionDefinition{SectionIDs: []string{"a"}}
	phases := def.PhaseDefs()
	require.Len(t, phases, 1)
	assert.Equal(t, 1, phases[0].MaxAttempts)
}

func TestDocumentSectionDefinition_MinPass(t *testing.T) {
	assert.Equal(t, 8.0, (DocumentSectionDefinition{}).MinPass())
	assert.Equal(t, 7.0, (DocumentSectionDefinition{MinPassScore: 7.0}).MinPass())
}

func TestDocumentSectionDefinition_LockedSectionsBefore(t *testing.T) {
	def := DocumentSectionDefinition{
		SectionIDs: []string{"teaser", "description", "tldr"},
	}
	draft := map[string]string{
		"teaser":      "t",
		"description": "d",
		"tldr":        "tl",
	}
	locked := def.LockedSectionsBefore("description", draft)
	assert.Equal(t, map[string]string{"teaser": "t"}, locked)

	locked = def.LockedSectionsBefore("tldr", draft)
	assert.Equal(t, map[string]string{"teaser": "t", "description": "d"}, locked)

	assert.Empty(t, def.LockedSectionsBefore("teaser", draft))
}

func TestDocumentSectionDefinition_LockedSectionsBefore_skipsEmpty(t *testing.T) {
	def := DocumentSectionDefinition{SectionIDs: []string{"a", "b", "c"}}
	draft := map[string]string{"a": "  ", "b": "ok"}
	locked := def.LockedSectionsBefore("c", draft)
	assert.Equal(t, map[string]string{"b": "ok"}, locked)
}
