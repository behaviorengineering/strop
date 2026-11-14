package criteria

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// FloorAnchor returns the highest whole-point rubric rung that is still <= score.
// A 1.41 / 2 score lands on the 1-point band, not the 2-point band.
func FloorAnchor(score, maxPoints float64) float64 {
	if maxPoints <= 0 {
		maxPoints = 2
	}
	if math.IsNaN(score) || score < 0 {
		return 0
	}
	anchor := math.Floor(score + 1e-9)
	maxAnchor := math.Floor(maxPoints + 1e-9)
	if anchor > maxAnchor {
		return maxAnchor
	}
	return anchor
}

// ScoringBandText returns the Scoring line for one whole-point anchor (0, 1, 2, ...).
// Empty when Scoring has no matching "N point(s):" line.
func ScoringBandText(scoring string, anchor float64) string {
	scoring = strings.TrimSpace(scoring)
	if scoring == "" {
		return ""
	}
	n := int(math.Round(anchor))
	if n < 0 {
		n = 0
	}
	patterns := []string{
		fmt.Sprintf("%d points:", n),
		fmt.Sprintf("%d point:", n),
	}
	if n == 1 {
		patterns = []string{"1 point:", "1 points:"}
	}
	for _, pattern := range patterns {
		idx := strings.Index(scoring, pattern)
		if idx == -1 {
			continue
		}
		rest := strings.TrimSpace(scoring[idx+len(pattern):])
		cut := len(rest)
		for next := 0; next <= 10; next++ {
			if next == n {
				continue
			}
			for _, nextPat := range []string{
				fmt.Sprintf("%d points:", next),
				fmt.Sprintf("%d point:", next),
			} {
				if found := strings.Index(rest, nextPat); found != -1 && found < cut {
					cut = found
				}
			}
		}
		line := strings.TrimSpace(rest[:cut])
		line = strings.TrimRight(line, ".")
		if line != "" {
			return line
		}
	}
	return ""
}

// FormatScoreEvidence lists each criterion's floor band, lowest score first.
// The band sentence is the rubric line we already wrote; the score backend does not invent it.
func FormatScoreEvidence(registry *CriterionRegistry, scores map[string]float64) string {
	if len(scores) == 0 {
		return ""
	}
	if registry == nil {
		registry = DefaultRegistry()
	}
	type row struct {
		id     string
		score  float64
		max    float64
		anchor float64
		band   string
	}
	rows := make([]row, 0, len(scores))
	for id, score := range scores {
		crit, err := registry.Get(CriterionID(id))
		maxPoints := 2.0
		scoring := ""
		if err == nil {
			if crit.MaxPoints > 0 {
				maxPoints = crit.MaxPoints
			}
			scoring = crit.Scoring
		}
		anchor := FloorAnchor(score, maxPoints)
		band := ScoringBandText(scoring, anchor)
		if band == "" {
			band = fmt.Sprintf("Anchor %.0f (partial or missing scoring line).", anchor)
		}
		rows = append(rows, row{
			id:     id,
			score:  score,
			max:    maxPoints,
			anchor: anchor,
			band:   band,
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].score != rows[j].score {
			return rows[i].score < rows[j].score
		}
		return rows[i].id < rows[j].id
	})
	var b strings.Builder
	b.WriteString("SCORE EVIDENCE (lowest score first; band text is the existing rubric line for the floor anchor, not a new judgment)\n")
	for i, r := range rows {
		fmt.Fprintf(&b, "%d. %s: %.2f / %.2f -> %.0f-point band\n   %s\n", i+1, r.id, r.score, r.max, r.anchor, r.band)
	}
	return strings.TrimRight(b.String(), "\n")
}
