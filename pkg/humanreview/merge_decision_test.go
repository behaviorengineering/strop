package humanreview

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestDefaultInteractiveMergeAction(t *testing.T) {
	t.Parallel()
	assert.Equal(t, MergeActionUpdate, DefaultInteractiveMergeAction([]*LearningArtifact{{ID: uuid.New()}}))
	assert.Equal(t, MergeActionCreate, DefaultInteractiveMergeAction(nil))
}

func TestNonInteractiveMergeAction_matchUpdates(t *testing.T) {
	t.Parallel()
	assert.Equal(t, MergeActionUpdate, NonInteractiveMergeAction([]*LearningArtifact{{ID: uuid.New()}}))
	assert.Equal(t, MergeActionCreate, NonInteractiveMergeAction(nil))
}

func TestSelectConflictCandidates_excludesIdentityMatches(t *testing.T) {
	t.Parallel()
	match := &LearningArtifact{
		ArtifactContent: PutSnapshot(map[string]interface{}{}, RetrievalSnapshot{DistinctiveMove: "pun in title"}),
	}
	conflict := &LearningArtifact{
		ArtifactContent: PutSnapshot(map[string]interface{}{}, RetrievalSnapshot{DistinctiveMove: "split on speaker"}),
	}
	got := SelectConflictCandidates(
		[]*LearningArtifact{match, conflict},
		RetrievalSnapshot{DistinctiveMove: "pun in title"},
	)
	assert.Equal(t, []*LearningArtifact{conflict}, got)
}

func TestPrimaryMergeTarget(t *testing.T) {
	t.Parallel()
	assert.Nil(t, PrimaryMergeTarget(nil))
	first := &LearningArtifact{ID: uuid.New()}
	assert.Equal(t, first, PrimaryMergeTarget([]*LearningArtifact{first, {ID: uuid.New()}}))
}
