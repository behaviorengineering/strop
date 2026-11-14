package criteria

import (
	"sort"
)

// Modifier is one leaf classifier under a parent criterion.
// IDs and rubrics are owned by the host pack that registers them.
type Modifier struct {
	ID          CriterionID
	Name        string
	Description string
}

// ParentModifiers is a parent criterion plus optional leaf classifiers.
// Score the parent first. Rank leaves only when the parent is below max.
type ParentModifiers struct {
	Parent CriterionID
	Leaves []Modifier
}

// LeafIDs returns leaf classifier IDs in pack order.
func (p ParentModifiers) LeafIDs() []CriterionID {
	ids := make([]CriterionID, 0, len(p.Leaves))
	for _, leaf := range p.Leaves {
		if leaf.ID == "" {
			continue
		}
		ids = append(ids, leaf.ID)
	}
	return ids
}

// LeafRank is one leaf score after ranking (highest presence first).
type LeafRank struct {
	ID    CriterionID `json:"id"`
	Score float64     `json:"score"`
}

// ShouldRankLeaves is true when the parent score is below its max (a ding).
// Skip leaf ranking when the parent already sits at max.
func ShouldRankLeaves(parentScore, parentMax float64) bool {
	if parentMax <= 0 {
		parentMax = 2
	}
	return parentScore < parentMax-1e-9
}

// RankLeaves sorts leaf scores highest first, then by ID.
// Missing IDs score 0. RankLeaves does not decide whether ranking should run.
func RankLeaves(mods ParentModifiers, scores map[string]float64) []LeafRank {
	ids := mods.LeafIDs()
	if len(ids) == 0 {
		return nil
	}
	ranks := make([]LeafRank, 0, len(ids))
	for _, id := range ids {
		score := 0.0
		if scores != nil {
			score = scores[string(id)]
		}
		ranks = append(ranks, LeafRank{ID: id, Score: score})
	}
	sort.SliceStable(ranks, func(i, j int) bool {
		if ranks[i].Score != ranks[j].Score {
			return ranks[i].Score > ranks[j].Score
		}
		return ranks[i].ID < ranks[j].ID
	})
	return ranks
}
