package jev

import (
	"fmt"
	"math"
	"strings"

	"github.com/behaviorengineering/strop/pkg/evaluation/criteria"

	"github.com/XiaoConstantine/dspy-go/pkg/experimental/decide"
)

const maxDecideScoreLevels = 10

// discreteLevelCount returns how many decide.Score levels to use for a criterion max (inclusive 0..max).
func discreteLevelCount(maxPoints float64) int {
	if maxPoints <= 0 {
		return 3
	}
	if maxPoints <= float64(maxDecideScoreLevels-1) {
		n := int(math.Round(maxPoints)) + 1
		if n < 2 {
			return 2
		}
		return n
	}
	return maxDecideScoreLevels
}

func anchorValue(index, levelCount int, maxPoints float64) float64 {
	if levelCount <= 1 {
		return 0
	}
	return (float64(index) / float64(levelCount-1)) * maxPoints
}

func scoreLevelsForCriterion(crit criteria.CriterionDescription, rubricOverride, scorePrompt string) []decide.ScoreLevel {
	maxPoints := crit.MaxPoints
	if maxPoints <= 0 {
		maxPoints = 2
	}
	levelCount := discreteLevelCount(maxPoints)
	descriptions := scoreLevelDescriptions(crit, rubricOverride, scorePrompt, levelCount, maxPoints)
	levels := make([]decide.ScoreLevel, levelCount)
	for i := 0; i < levelCount; i++ {
		levels[i] = decide.Level(anchorValue(i, levelCount, maxPoints), descriptions[i])
	}
	return levels
}

func scoreLevelDescriptions(crit criteria.CriterionDescription, rubricOverride, scorePrompt string, levelCount int, maxPoints float64) []string {
	base := strings.TrimSpace(rubricOverride)
	if base == "" {
		base = strings.TrimSpace(crit.Description)
		if crit.Scoring != "" {
			base = base + "\n" + crit.Scoring
		}
	}
	if base == "" {
		base = "Score this criterion based on feedback and generator output."
	}
	if scorePrompt != "" {
		base = base + "\n\nRole score prompt (context):\n" + truncatePrompt(scorePrompt, 4000)
	}
	out := make([]string, levelCount)
	for i := 0; i < levelCount; i++ {
		anchor := anchorValue(i, levelCount, maxPoints)
		switch i {
		case 0:
			out[i] = fmt.Sprintf("Anchor %.2f: does not meet the criterion. %s", anchor, base)
		case levelCount - 1:
			out[i] = fmt.Sprintf("Anchor %.2f: fully meets the criterion (max %.2f). %s", anchor, maxPoints, base)
		default:
			out[i] = fmt.Sprintf("Anchor %.2f (partial). %s", anchor, base)
		}
	}
	return out
}

func clampScore(score, maxPoints float64) float64 {
	if maxPoints <= 0 {
		return score
	}
	if score < 0 {
		return 0
	}
	if score > maxPoints {
		return maxPoints
	}
	return score
}

func truncatePrompt(prompt string, maxLen int) string {
	if maxLen <= 0 || len(prompt) <= maxLen {
		return prompt
	}
	return prompt[:maxLen] + "..."
}
