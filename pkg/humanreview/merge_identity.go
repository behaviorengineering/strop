package humanreview

import "strings"

// DistinctiveMovesConflict is true when either move is empty or they differ.
// Empty distinctive move is treated as novel: merge is not allowed by default.
func DistinctiveMovesConflict(candidate, existing string) bool {
	left := strings.TrimSpace(candidate)
	right := strings.TrimSpace(existing)
	if left == "" || right == "" {
		return true
	}
	return left != right
}

// CanMergeByIdentity is true only when both distinctive moves are non-empty and equal.
func CanMergeByIdentity(candidate, existing RetrievalSnapshot) bool {
	return !DistinctiveMovesConflict(candidate.DistinctiveMove, existing.DistinctiveMove)
}

// SelectMergeCandidates keeps approved-row candidates whose distinctive move matches snapshot.
// Empty snapshot distinctive move yields no candidates (fail closed).
func SelectMergeCandidates(artifacts []*LearningArtifact, snapshot RetrievalSnapshot) []*LearningArtifact {
	if strings.TrimSpace(snapshot.DistinctiveMove) == "" {
		return nil
	}
	var matched []*LearningArtifact
	for _, artifact := range artifacts {
		if artifact == nil {
			continue
		}
		existing := SnapshotFromContent(artifact.ArtifactContent)
		if CanMergeByIdentity(snapshot, existing) {
			matched = append(matched, artifact)
		}
	}
	return matched
}
