// Package jev implements scoring.Backend using dspy-go experimental TypeSafe System One (decide + typesafe).
//
// This is the only strop package that may import github.com/XiaoConstantine/dspy-go/pkg/experimental.
// Hosts opt in via ChainedEvaluatorConfig.ScoreBackendFactory and pass connection fields on Options.
//
// Scores are mapped to each criterion's registry MaxPoints using discrete decide.Score anchors (0..max for typical 0-2 rubrics).
package jev
