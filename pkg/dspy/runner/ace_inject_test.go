package runner

import (
	"context"
	"path/filepath"
	"testing"

	stropdspy "github.com/behaviorengineering/strop/pkg/dspy"
	"github.com/behaviorengineering/strop/pkg/dspy/ace"

	"github.com/stretchr/testify/require"
)

func TestAppendACEPlaybook_skipsWhenNilManager(t *testing.T) {
	t.Parallel()
	inputs := map[string]interface{}{stropdspy.FieldRetrievedGuides: "existing"}
	require.NoError(t, appendACEPlaybook(context.Background(), "job", inputs))
	require.Equal(t, "existing", inputs[stropdspy.FieldRetrievedGuides])
}

func TestAppendACEPlaybook_mergesWhenManagerPresent(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "learnings.md")
	m, err := ace.NewManager(ace.Config{Enabled: true, LearningsPath: path}, "e1", "job")
	require.NoError(t, err)
	defer func() { require.NoError(t, m.Close()) }()

	rec := m.StartTrajectory("e1", "job", "seed")
	rec.RecordStep("refine", "", "ok", nil, nil, nil)
	m.EndTrajectory(context.Background(), rec, ace.OutcomeSuccess)

	ctx := ace.WithManager(context.Background(), m)
	inputs := map[string]interface{}{
		stropdspy.FieldRetrievedGuides: "<item>guide</item>",
	}
	require.NoError(t, appendACEPlaybook(ctx, "job", inputs))
	got, _ := inputs[stropdspy.FieldRetrievedGuides].(string)
	if got != "<item>guide</item>" {
		require.Contains(t, got, "<item>guide</item>")
		require.Contains(t, got, ace.CiteInstruction)
	}
}

func TestAppendACEPlaybook_failsOnJobMismatch(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "learnings.md")
	m, err := ace.NewManager(ace.Config{Enabled: true, LearningsPath: path}, "e1", "job")
	require.NoError(t, err)
	defer func() { require.NoError(t, m.Close()) }()

	ctx := ace.WithManager(context.Background(), m)
	inputs := map[string]interface{}{}
	err = appendACEPlaybook(ctx, "other-job", inputs)
	require.Error(t, err)
}
