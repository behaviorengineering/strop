package evaluation

import "time"

// IndividualEvaluation is the result from a single evaluator.
// Used by the evaluation workflow; no pipeline-specific imports.
type IndividualEvaluation struct {
	AgentID         string             `json:"agent_id"`
	AgentName       string             `json:"agent_name"`
	Score           float64            `json:"score"`
	CriterionScores map[string]float64 `json:"criterion_scores"`
	Feedback        string             `json:"feedback"`
	Rationale       string             `json:"rationale,omitempty"`
	// Error is for in-memory use; not serialized to JSON (error type marshals poorly).
	Error error `json:"-"`
	// ErrorMessage is the string form for JSON/API; set this when setting Error so serialization is useful.
	ErrorMessage string `json:"error,omitempty"`
}

// AggregatedEvaluation is the result of multi-agent evaluation.
type AggregatedEvaluation struct {
	WeightedScore        float64            `json:"weighted_score"`
	CriterionScores      map[string]float64 `json:"criterion_scores"`
	ConsolidatedFeedback string             `json:"consolidated_feedback"`
	AgentScores          map[string]float64 `json:"agent_scores"`
	AgentFeedback        map[string]string  `json:"agent_feedback"`
	AgentRationale       map[string]string  `json:"agent_rationale,omitempty"`
	EvaluationTime       time.Duration      `json:"evaluation_time"`
}

// RoleInfo provides role metadata for the evaluation workflow.
// Pipelines implement this; the workflow never imports pipeline code.
// EvaluatorKey and ConsolidatorKey are distinct types; labels are display only.
// ExpertKey is a separate identity for feedback-analysis experts (not part of RoleInfo).
type RoleInfo interface {
	EvaluatorName(key EvaluatorKey) string
	HasEvaluator(key EvaluatorKey) bool
	EvaluatorWeight(key EvaluatorKey) float64
	ConsolidatorKey() ConsolidatorKey
	ConsolidatorName() string
}
