package skills

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSelectBulletsCapsPolarityAndScope(t *testing.T) {
	t.Parallel()
	bullets := []Bullet{
		{ID: "b", Scope: "conversation", Text: "Ask one question.", Polarity: PolarityHelpful},
		{ID: "a", Scope: "conversation", Text: "Restate the anchor.", Polarity: PolarityHelpful},
		{ID: "c", Scope: "conversation", Text: "Do not invent a fact.", Polarity: PolarityHarmful},
		{ID: "d", Scope: "other", Text: "Stay in the other scope.", Polarity: PolarityHelpful},
	}
	got, err := SelectBullets(bullets, BulletRequest{Scope: "conversation", HelpfulCap: 1, HarmfulCap: 5})
	require.NoError(t, err)
	require.Equal(t, []string{"a", "c"}, bulletIDs(got))

	_, err = SelectBullets(bullets, BulletRequest{})
	require.ErrorIs(t, err, ErrInvalid)

	bad := append(bullets, Bullet{ID: "e", Scope: "conversation", Text: "tip", Polarity: "maybe"})
	_, err = SelectBullets(bad, BulletRequest{Scope: "conversation"})
	require.ErrorIs(t, err, ErrInvalid)
}

func TestTrajectoryRequiresTaskAndUniqueIDs(t *testing.T) {
	t.Parallel()
	require.NoError(t, (Trajectory{TaskID: "task-1", Stage: "locked"}).Validate())
	err := (Trajectory{Stage: "locked", SkillIDs: []string{"alpha.read"}}).Validate()
	require.ErrorIs(t, err, ErrInvalid)
	err = (Trajectory{TaskID: "task-1", Stage: "locked", BulletIDs: []string{"a", "a"}}).Validate()
	require.ErrorIs(t, err, ErrInvalid)
}

func bulletIDs(bullets []Bullet) []string {
	out := make([]string, len(bullets))
	for i := range bullets {
		out[i] = bullets[i].ID
	}
	return out
}
