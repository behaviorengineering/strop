package humanreview

// MergeAction is the human or non-interactive choice for a learning candidate.
type MergeAction string

const (
	// MergeActionUpdate merges into an existing approved artifact (identity match).
	MergeActionUpdate MergeAction = "update"
	// MergeActionCreate creates a new pending learning artifact.
	MergeActionCreate MergeAction = "create"
	// MergeActionSkip drops the candidate.
	MergeActionSkip MergeAction = "skip"
)

// SelectConflictCandidates returns approved peers that cannot merge by distinctive-move identity.
// Includes empty distinctive moves on either side (treated as conflict / novel relative to merge).
func SelectConflictCandidates(artifacts []*LearningArtifact, snapshot RetrievalSnapshot) []*LearningArtifact {
	var conflicts []*LearningArtifact
	for _, artifact := range artifacts {
		if artifact == nil {
			continue
		}
		existing := SnapshotFromContent(artifact.ArtifactContent)
		if !CanMergeByIdentity(snapshot, existing) {
			conflicts = append(conflicts, artifact)
		}
	}
	return conflicts
}

// DefaultInteractiveMergeAction picks the highlighted default for the merge prompt.
// Identity match → Update; otherwise Create (including distinctive-move conflict).
func DefaultInteractiveMergeAction(matches []*LearningArtifact) MergeAction {
	if len(matches) > 0 {
		return MergeActionUpdate
	}
	return MergeActionCreate
}

// NonInteractiveMergeAction applies Slice 3 policy without a TTY:
// identity match → Update (no duplicate); otherwise Create (no auto-merge on conflict).
func NonInteractiveMergeAction(matches []*LearningArtifact) MergeAction {
	if len(matches) > 0 {
		return MergeActionUpdate
	}
	return MergeActionCreate
}

// PrimaryMergeTarget returns the first identity match, or nil.
func PrimaryMergeTarget(matches []*LearningArtifact) *LearningArtifact {
	if len(matches) == 0 {
		return nil
	}
	return matches[0]
}
