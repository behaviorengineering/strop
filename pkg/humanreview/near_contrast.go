package humanreview

import "strings"

// RetrievalHit is a lightweight search hit used to pick near + contrast demos
// before hydrating full LearningArtifact rows from Postgres.
type RetrievalHit struct {
	ID               string
	Pipeline         string
	DistinctiveMove  string
	ObjectiveSummary string
	QualityStatus    string
}

// SelectNearAndContrast picks the first hit as near and the first later hit with a
// different non-empty distinctive move as contrast. Same-move clones are skipped.
// If no contrast exists, contrast is nil (caller returns one demo).
func SelectNearAndContrast(hits []RetrievalHit) (near *RetrievalHit, contrast *RetrievalHit) {
	if len(hits) == 0 {
		return nil, nil
	}
	first := hits[0]
	near = &first
	nearMove := normalizeMove(near.DistinctiveMove)
	for i := 1; i < len(hits); i++ {
		move := normalizeMove(hits[i].DistinctiveMove)
		if move == "" {
			continue
		}
		if nearMove != "" && move == nearMove {
			continue
		}
		hit := hits[i]
		contrast = &hit
		return near, contrast
	}
	return near, nil
}

// SelectNearAndContrastPreferActive prefers an active (non-penalized) hit for near
// when one exists; contrast may still be penalized. Quarantined hits should already
// be filtered out by the caller.
func SelectNearAndContrastPreferActive(hits []RetrievalHit) (near *RetrievalHit, contrast *RetrievalHit) {
	if len(hits) == 0 {
		return nil, nil
	}
	nearIdx := 0
	for i := range hits {
		if hits[i].QualityStatus != QualityStatusPenalized {
			nearIdx = i
			break
		}
	}
	nearHit := hits[nearIdx]
	near = &nearHit
	nearMove := normalizeMove(near.DistinctiveMove)
	for i := nearIdx + 1; i < len(hits); i++ {
		move := normalizeMove(hits[i].DistinctiveMove)
		if move == "" {
			continue
		}
		if nearMove != "" && move == nearMove {
			continue
		}
		hit := hits[i]
		contrast = &hit
		return near, contrast
	}
	for i := 0; i < nearIdx; i++ {
		move := normalizeMove(hits[i].DistinctiveMove)
		if move == "" {
			continue
		}
		if nearMove != "" && move == nearMove {
			continue
		}
		hit := hits[i]
		contrast = &hit
		return near, contrast
	}
	return near, nil
}

func normalizeMove(move string) string {
	return strings.ToLower(strings.TrimSpace(move))
}
