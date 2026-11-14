package criteria

import (
	"fmt"
	"sort"
	"strings"
)

// DefaultLeafScorePrompt is used when ParentModifiers.LeafScorePrompt is empty.
const DefaultLeafScorePrompt = `Score each leaf as how strongly that miss is present. 2 is the main hole. 0 is not this hole. Do not re-score the parent.`

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
	Parent          CriterionID
	Leaves          []Modifier
	LeafScorePrompt string
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

// LeafScorePromptOrDefault returns the host prompt, or DefaultLeafScorePrompt.
func (p ParentModifiers) LeafScorePromptOrDefault() string {
	if strings.TrimSpace(p.LeafScorePrompt) != "" {
		return p.LeafScorePrompt
	}
	return DefaultLeafScorePrompt
}

// FormatLeafRankEvidence lists ranked leaves highest presence first.
// The first leaf is the hole for this parent.
func FormatLeafRankEvidence(registry *CriterionRegistry, mods ParentModifiers, ranks []LeafRank) string {
	if len(ranks) == 0 {
		return ""
	}
	if registry == nil {
		registry = DefaultRegistry()
	}
	var b strings.Builder
	fmt.Fprintf(&b, "LEAF RANK for %s (highest presence first; first leaf is the hole for this parent)\n", mods.Parent)
	for i, r := range ranks {
		name := string(r.ID)
		band := ""
		maxPoints := 2.0
		if desc, err := registry.Get(r.ID); err == nil {
			if strings.TrimSpace(desc.Name) != "" {
				name = desc.Name
			}
			if desc.MaxPoints > 0 {
				maxPoints = desc.MaxPoints
			}
			anchor := FloorAnchor(r.Score, maxPoints)
			band = ScoringBandText(desc.Scoring, anchor)
			if band == "" {
				band = fmt.Sprintf("Anchor %.0f (partial or missing scoring line).", anchor)
			}
		} else {
			for _, leaf := range mods.Leaves {
				if leaf.ID == r.ID && strings.TrimSpace(leaf.Name) != "" {
					name = leaf.Name
					break
				}
			}
			band = strings.TrimSpace(leafDescription(mods, r.ID))
			if band == "" {
				band = "Presence of this miss."
			}
		}
		fmt.Fprintf(&b, "%d. %s (%s): %.2f / %.2f\n   %s\n", i+1, r.ID, name, r.Score, maxPoints, band)
	}
	fmt.Fprintf(&b, "Top leaf is %s. Write the [ ] item from that leaf. Do not invent a different hole for this parent.\n", ranks[0].ID)
	return strings.TrimRight(b.String(), "\n")
}

func leafDescription(mods ParentModifiers, id CriterionID) string {
	for _, leaf := range mods.Leaves {
		if leaf.ID == id {
			return leaf.Description
		}
	}
	return ""
}
