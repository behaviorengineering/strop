// Package humanreview holds portable human-review domain types and helpers.
//
// Product Job/Step constants register via app packs (e.g. sayings jobpack).
// Persistence, DSPy score-proposal adapters, and CLI state machines stay in the app.
package humanreview

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/behaviorengineering/strop/evaluation/criteria"
)

// PipelineHistoryEntry is one version/attempt in a pipeline execution history.
type PipelineHistoryEntry struct {
	Inputs      string   `json:"inputs"`
	Outputs     string   `json:"outputs"`
	Feedback    string   `json:"feedback"`
	Attachments []string `json:"attachments"`
}

// PipelineHistory is the full execution history for a review session.
type PipelineHistory struct {
	PipelineType string                 `json:"pipeline_type"`
	History      []PipelineHistoryEntry `json:"history"`
}

// CriterionScoreStatus is the status of a criterion score evaluation.
type CriterionScoreStatus string

const (
	CriterionScoreStatusProposed  CriterionScoreStatus = "proposed"
	CriterionScoreStatusReviewing CriterionScoreStatus = "reviewing"
	CriterionScoreStatusAgreed    CriterionScoreStatus = "agreed"
)

// Job identifies a reviewable generation workflow (product packs define concrete values).
type Job string

// Step identifies an action within a job (product packs define generation steps).
type Step string

// Portable evaluation step used when filtering learning/examples by evaluate action.
const StepEvaluate Step = "evaluate"

// PipelineType identifies which product pipeline owns a review session.
type PipelineType string

// Evaluation status constants.
const (
	StatusInProgress       = "in_progress"
	StatusCompleted        = "completed"
	StatusReadyForLearning = "ready_for_learning"
	StatusRejected         = "rejected"
)

// CriterionScoreEvaluation tracks AI proposal and human agreement for one criterion.
type CriterionScoreEvaluation struct {
	CriterionName        string  `json:"criterion_name"`
	CriterionDescription string  `json:"criterion_description"`
	MaxPoints            float64 `json:"max_points"`

	ProposedScore float64  `json:"proposed_score"`
	Rationale     string   `json:"rationale"`
	Analysis      string   `json:"analysis"`
	Evidence      []string `json:"evidence"`

	HumanAgrees  *bool    `json:"human_agrees"`
	HumanScore   *float64 `json:"human_score"`
	HumanComment string   `json:"human_comment"`

	FinalScore     *float64             `json:"final_score"`
	FinalRationale string               `json:"final_rationale"`
	Status         CriterionScoreStatus `json:"status"`

	FirstVersionAtMax       *int   `json:"first_version_at_max,omitempty"`
	VersionsWithImprovement int    `json:"versions_with_improvement,omitempty"`
	SmellPassedAtV1         bool   `json:"smell_passed_at_v1,omitempty"`
	PerformanceSummary      string `json:"performance_summary,omitempty"`

	ProposedAt time.Time  `json:"proposed_at"`
	ReviewedAt *time.Time `json:"reviewed_at"`
	AgreedAt   *time.Time `json:"agreed_at"`
}

// HumanEvaluation is one human-review session for a root entity + job.
type HumanEvaluation struct {
	ID           uuid.UUID    `json:"id" db:"id"`
	PipelineType PipelineType `json:"pipeline_type" db:"pipeline_type"`
	RootEntityID uuid.UUID    `json:"root_entity_id" db:"root_entity_id"`
	Job          Job          `json:"job" db:"job"`
	Step         Step         `json:"step" db:"step"`

	PipelineHistory      PipelineHistory            `json:"pipeline_history" db:"pipeline_history"`
	CriterionEvaluations []CriterionScoreEvaluation `json:"criterion_evaluations" db:"criterion_evaluations"`

	HumanAlignmentAgrees     *bool      `json:"human_alignment_agrees" db:"human_alignment_agrees"`
	HumanAlignmentComment    string     `json:"human_alignment_comment" db:"human_alignment_comment"`
	HumanAlignmentReviewedAt *time.Time `json:"human_alignment_reviewed_at" db:"human_alignment_reviewed_at"`

	Status     string  `json:"status" db:"status"`
	TotalScore float64 `json:"total_score" db:"total_score"`

	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// GetCriterionEvaluation finds a criterion evaluation by display name.
func (e *HumanEvaluation) GetCriterionEvaluation(criterionName string) (*CriterionScoreEvaluation, error) {
	for i := range e.CriterionEvaluations {
		if e.CriterionEvaluations[i].CriterionName == criterionName {
			return &e.CriterionEvaluations[i], nil
		}
	}
	return nil, fmt.Errorf("criterion not found: %s", criterionName)
}

// IsCompleted reports whether status is completed.
func (e *HumanEvaluation) IsCompleted() bool {
	return e.Status == StatusCompleted
}

// CriterionDescription is a rubric definition used when proposing scores.
type CriterionDescription struct {
	Name        string
	Description string
	MaxPoints   float64
	Category    string
}

func convertCriteria(crits []criteria.CriterionDescription) []CriterionDescription {
	result := make([]CriterionDescription, len(crits))
	for i, crit := range crits {
		result[i] = CriterionDescription{
			Name:        crit.Name,
			Description: crit.Description,
			MaxPoints:   crit.MaxPoints,
			Category:    crit.Category,
		}
	}
	return result
}

// DeduplicateCriterionIDs returns criterion IDs with duplicates removed (first wins).
func DeduplicateCriterionIDs(ids []criteria.CriterionID) []criteria.CriterionID {
	if len(ids) == 0 {
		return ids
	}
	seen := make(map[criteria.CriterionID]struct{}, len(ids))
	out := make([]criteria.CriterionID, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// DeduplicateCriterionEvaluationsByName removes duplicate rows by display name (first wins).
func DeduplicateCriterionEvaluationsByName(evals []CriterionScoreEvaluation) ([]CriterionScoreEvaluation, bool) {
	if len(evals) == 0 {
		return evals, false
	}
	seen := make(map[string]struct{}, len(evals))
	out := make([]CriterionScoreEvaluation, 0, len(evals))
	changed := false
	for _, eval := range evals {
		if _, ok := seen[eval.CriterionName]; ok {
			changed = true
			continue
		}
		seen[eval.CriterionName] = struct{}{}
		out = append(out, eval)
	}
	return out, changed
}

// GetCriterionDescriptions resolves rubric copy from the shared criteria registry.
func GetCriterionDescriptions(criterionIDs []criteria.CriterionID) []CriterionDescription {
	criterionIDs = DeduplicateCriterionIDs(criterionIDs)
	crits, err := criteria.DefaultRegistry().GetMultiple(criterionIDs)
	if err != nil {
		return getDefaultProcessCriteria()
	}
	return convertCriteria(crits)
}

func getDefaultProcessCriteria() []CriterionDescription {
	processCriteria := []criteria.CriterionID{
		criteria.CriterionIDCompleteness,
		criteria.CriterionIDFeedbackAdherence,
		criteria.CriterionIDContextAwareness,
		criteria.CriterionIDIterationEfficiency,
		criteria.CriterionIDDepthOfImprovement,
		criteria.CriterionIDConsistencyAcrossVersions,
	}
	crits, err := criteria.DefaultRegistry().GetMultiple(processCriteria)
	if err != nil {
		return []CriterionDescription{}
	}
	return convertCriteria(crits)
}
