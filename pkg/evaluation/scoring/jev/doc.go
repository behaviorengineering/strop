// Package jev implements scoring.Backend using dspy-go experimental TypeSafe System One (decide + typesafe).
//
// This is the only strop package that may import github.com/XiaoConstantine/dspy-go/pkg/experimental.
// Hosts opt in via ChainedEvaluatorConfig.ScoreBackendFactory when TYPESAFE_API_KEY is set.
// List evaluator role keys in STROP_JEV_SCORE_ROLES (comma-separated), e.g. process_evaluator for sayings.
//
// Scores are mapped to each criterion's registry MaxPoints using discrete decide.Score anchors (0..max for typical 0-2 rubrics).
package jev
