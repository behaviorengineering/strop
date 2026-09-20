package humanreview

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemoryItemObjectiveStore_uniqueByPipelineRootJob(t *testing.T) {
	t.Parallel()
	store := NewMemoryItemObjectiveStore()
	root := uuid.New()
	first := &ItemObjective{
		PipelineType: "demo",
		RootEntityID: root,
		Job:          "compose_job",
		Payload:      map[string]interface{}{"summary": "first"},
	}
	require.NoError(t, store.Upsert(context.Background(), first))

	second := &ItemObjective{
		PipelineType: "demo",
		RootEntityID: root,
		Job:          "compose_job",
		Payload:      map[string]interface{}{"summary": "updated"},
	}
	require.NoError(t, store.Upsert(context.Background(), second))

	got, err := store.Get(context.Background(), "demo", root, "compose_job")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "updated", got.Payload["summary"])

	otherJob := &ItemObjective{
		PipelineType: "demo",
		RootEntityID: root,
		Job:          "other_job",
		Payload:      map[string]interface{}{"summary": "other"},
	}
	require.NoError(t, store.Upsert(context.Background(), otherJob))

	firstRow, err := store.Get(context.Background(), "demo", root, "compose_job")
	require.NoError(t, err)
	require.NotNil(t, firstRow)
	assert.Equal(t, "updated", firstRow.Payload["summary"])

	otherRow, err := store.Get(context.Background(), "demo", root, "other_job")
	require.NoError(t, err)
	require.NotNil(t, otherRow)
	assert.Equal(t, "other", otherRow.Payload["summary"])

	missing, err := store.Get(context.Background(), "other_pipeline", root, "compose_job")
	require.NoError(t, err)
	assert.Nil(t, missing)
}

func TestSnapshotFromContent_roundTrip(t *testing.T) {
	t.Parallel()
	snapshot := RetrievalSnapshot{
		ObjectiveSummary: "keep the bind",
		DistinctiveMove:  "pun in title",
		DoNotReuseFor:    "unrelated family",
		LoadBearing:      "explanation wordplay",
		Extras:           map[string]interface{}{"flag": true},
	}
	content := PutSnapshot(nil, snapshot)
	got := SnapshotFromContent(content)
	assert.Equal(t, snapshot.ObjectiveSummary, got.ObjectiveSummary)
	assert.Equal(t, snapshot.DistinctiveMove, got.DistinctiveMove)
	assert.Equal(t, snapshot.DoNotReuseFor, got.DoNotReuseFor)
	assert.Equal(t, snapshot.LoadBearing, got.LoadBearing)
	require.NotNil(t, got.Extras)
	assert.Equal(t, true, got.Extras["flag"])
}

func TestSnapshotFromContent_missingIsEmpty(t *testing.T) {
	t.Parallel()
	got := SnapshotFromContent(map[string]interface{}{"input": "x"})
	assert.Equal(t, RetrievalSnapshot{}, got)
}
