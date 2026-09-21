package skills

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func sampleSkill(id, stage string, effect SideEffect) Skill {
	return Skill{
		ID:          id,
		Version:     "1",
		Description: "A short capability.",
		SideEffect:  effect,
		Stages:      []string{stage},
	}
}

func TestSelectHardFiltersAndCap(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	require.NoError(t, reg.Register(sampleSkill("beta.read", "locked", SideEffectRead)))
	require.NoError(t, reg.Register(sampleSkill("alpha.read", "locked", SideEffectRead)))
	require.NoError(t, reg.Register(sampleSkill("gamma.write", "locked", SideEffectWrite)))
	require.NoError(t, reg.Register(sampleSkill("delta.read", "open", SideEffectRead)))
	anyStage := sampleSkill("epsilon.read", "locked", SideEffectRead)
	anyStage.Stages = nil
	require.NoError(t, reg.Register(anyStage))

	got, err := Select(reg, Request{
		Stage:       "locked",
		SideEffects: []SideEffect{SideEffectRead},
		Allow:       []string{"gamma.write", "delta.read", "beta.read", "alpha.read", "epsilon.read", "missing.read"},
		Cap:         2,
	})
	require.NoError(t, err)
	require.Equal(t, []string{"alpha.read", "beta.read"}, skillIDs(got.Skills))

	none, err := Select(reg, Request{
		Stage:       "locked",
		SideEffects: []SideEffect{SideEffectRead},
	})
	require.NoError(t, err)
	require.Empty(t, none.Skills)

	_, err = Select(reg, Request{
		Stage:       "locked",
		SideEffects: []SideEffect{SideEffectRead},
		Allow:       []string{"alpha.read"},
		Cap:         MaxSkillCap + 1,
	})
	require.ErrorIs(t, err, ErrInvalid)
}

func TestRegisterRejectsDuplicateAndFreeText(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	err := reg.Register(Skill{ID: "not a skill", Version: "1", Description: "x", SideEffect: SideEffectNone})
	require.ErrorIs(t, err, ErrInvalid)
	require.NoError(t, reg.Register(sampleSkill("alpha.read", "locked", SideEffectRead)))
	err = reg.Register(sampleSkill("alpha.read", "locked", SideEffectRead))
	require.ErrorIs(t, err, ErrInvalid)
}

func skillIDs(skills []Skill) []string {
	out := make([]string, len(skills))
	for i := range skills {
		out[i] = skills[i].ID
	}
	return out
}
