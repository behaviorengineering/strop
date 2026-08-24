package humanreview

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDistinctiveMovesConflict_emptyIsConflict(t *testing.T) {
	t.Parallel()
	assert.True(t, DistinctiveMovesConflict("", "pun in title"))
	assert.True(t, DistinctiveMovesConflict("pun in title", ""))
	assert.True(t, DistinctiveMovesConflict("", ""))
	assert.True(t, DistinctiveMovesConflict("   ", "pun in title"))
}

func TestDistinctiveMovesConflict_mismatchIsConflict(t *testing.T) {
	t.Parallel()
	assert.True(t, DistinctiveMovesConflict("pun in title", "split on speaker turn"))
}

func TestCanMergeByIdentity_match(t *testing.T) {
	t.Parallel()
	assert.True(t, CanMergeByIdentity(
		RetrievalSnapshot{DistinctiveMove: "pun in title"},
		RetrievalSnapshot{DistinctiveMove: "pun in title"},
	))
}

func TestSelectMergeCandidates_emptySnapshotYieldsNone(t *testing.T) {
	t.Parallel()
	artifacts := []*LearningArtifact{
		{ArtifactContent: PutSnapshot(map[string]interface{}{}, RetrievalSnapshot{DistinctiveMove: "pun in title"})},
	}
	got := SelectMergeCandidates(artifacts, RetrievalSnapshot{})
	assert.Nil(t, got)
}

func TestSelectMergeCandidates_keepsMatchingOnly(t *testing.T) {
	t.Parallel()
	match := &LearningArtifact{
		ArtifactContent: PutSnapshot(map[string]interface{}{"job": "compose"}, RetrievalSnapshot{DistinctiveMove: "pun in title"}),
	}
	conflict := &LearningArtifact{
		ArtifactContent: PutSnapshot(map[string]interface{}{"job": "compose"}, RetrievalSnapshot{DistinctiveMove: "split on speaker"}),
	}
	empty := &LearningArtifact{
		ArtifactContent: PutSnapshot(map[string]interface{}{"job": "compose"}, RetrievalSnapshot{}),
	}
	got := SelectMergeCandidates([]*LearningArtifact{match, conflict, empty, nil}, RetrievalSnapshot{DistinctiveMove: "pun in title"})
	require.Len(t, got, 1)
	assert.Equal(t, match, got[0])
}
