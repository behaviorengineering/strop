package ace

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
	"github.com/stretchr/testify/require"
)

func TestNewManager_requiresPathAndBinding(t *testing.T) {
	t.Parallel()
	_, err := NewManager(Config{Enabled: true}, "e1", "job")
	require.Error(t, err)

	_, err = NewManager(Config{Enabled: true, LearningsPath: filepath.Join(t.TempDir(), "a.md")}, "", "job")
	require.Error(t, err)

	_, err = NewManager(Config{Enabled: false, LearningsPath: filepath.Join(t.TempDir(), "a.md")}, "e1", "job")
	require.Error(t, err)
}

func TestManager_CheckBindingAndSession(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "learnings.md")
	m, err := NewManager(Config{Enabled: true, LearningsPath: path}, "entity-a", "refine")
	require.NoError(t, err)
	defer func() { require.NoError(t, m.Close()) }()

	require.NoError(t, m.CheckBinding("entity-a", "refine"))
	require.Error(t, m.CheckBinding("entity-b", "refine"))
	require.Error(t, m.CheckBinding("entity-a", "other"))

	ctx := WithManager(context.Background(), m)
	require.Same(t, m, FromContext(ctx))
	require.Nil(t, FromContext(context.Background()))

	rec := m.StartTrajectory("entity-a", "refine", "version 1")
	require.NotNil(t, rec)
	rec.RecordStep("refine", "", "score 8.0 feedback ok", nil, nil, nil)
	m.EndTrajectory(ctx, rec, OutcomePartial)
	_ = m.LearningsContext()
}

func TestMergePlaybookIntoGuides(t *testing.T) {
	t.Parallel()
	require.Equal(t, "", MergePlaybookIntoGuides("", ""))
	got := MergePlaybookIntoGuides("", "[L001] tip")
	require.Contains(t, got, CiteInstruction)
	require.Contains(t, got, "[L001] tip")
	got = MergePlaybookIntoGuides("<item>guide</item>", "[L001] tip")
	require.Contains(t, got, "<item>guide</item>")
	require.Contains(t, got, "[L001] tip")
}

func TestCatchInterceptor_recordsOnError(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "learnings.md")
	m, err := NewManager(Config{Enabled: true, LearningsPath: path}, "e1", "job")
	require.NoError(t, err)
	defer func() { require.NoError(t, m.Close()) }()

	rec := m.StartTrajectory("e1", "job", "v1")
	ctx := WithRecorder(context.Background(), rec)
	boom := errors.New("module boom")
	_, gotErr := CatchInterceptor()(ctx, nil, &core.ModuleInfo{ModuleName: "gen"},
		func(context.Context, map[string]any, ...core.Option) (map[string]any, error) {
			return nil, boom
		},
	)
	require.ErrorIs(t, gotErr, boom)

	// No recorder: still returns error, no panic.
	_, gotErr = CatchInterceptor()(context.Background(), nil, &core.ModuleInfo{ModuleName: "gen"},
		func(context.Context, map[string]any, ...core.Option) (map[string]any, error) {
			return nil, boom
		},
	)
	require.ErrorIs(t, gotErr, boom)

	m.EndTrajectory(ctx, rec, OutcomeFailure)
}
