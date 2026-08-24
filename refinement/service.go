package refinement

import (
	"context"
	"strings"

	stroplog "github.com/behaviorengineering/strop/log"

	"github.com/google/uuid"
)

// ContentRecord represents any content record that can be refined (pipeline-agnostic).
type ContentRecord interface {
	GetID() uuid.UUID
	GetStatus() string
	GetEvaluationFeedback() *string
	GetScore() *float64
}

// NextVersionFromCount returns the next version number (existingCount + 1).
// Use for pipelines that track versions by count only (e.g. YouTube structural jobs).
func NextVersionFromCount(existingCount int) int {
	return existingCount + 1
}

// VersionInfo contains calculated version and feedback information.
type VersionInfo struct {
	StartVersion     int
	PreviousFeedback string
	NonRejectedCount int
}

// MaxVersionsResult is the generic result when max versions is exceeded.
// Callers map LastPendingID to their pipeline's result type (e.g. ProcessResult).
type MaxVersionsResult struct {
	Stopped       bool
	LastPendingID uuid.UUID
}

// ProblemDiagnosis describes what went wrong when a score decreased (used by self-healing).
type ProblemDiagnosis struct {
	ProblemType    string
	Description    string
	AffectedFields []string
}

// HealingResult is the result of a self-healing attempt.
type HealingResult struct {
	ShouldRetry        bool
	CorrectiveFeedback string
	ProblemType        string
	ProblemDescription string
}

// ServiceInterface defines the refinement contract: versioning, stopping conditions, and self-healing.
// One concept: refine until good or max, with recovery (self-healing) when score decreases.
type ServiceInterface interface {
	// Versioning and stopping
	CalculateVersionAndFeedback(records []ContentRecord) VersionInfo
	CheckMaxVersions(startVersion, maxVersions int, records []ContentRecord, contextID uuid.UUID) (MaxVersionsResult, bool)
	ShouldStopDueToScoreDecrease(records []ContentRecord, contextID uuid.UUID) (bool, uuid.UUID)
	CheckStoppingConditions(currentScore, previousScore float64, version int, contextID, selectedID uuid.UUID, feedback string) (shouldStop bool, returnID uuid.UUID)
	// Self-healing (part of refinement: recover from score decrease with corrective feedback)
	MaxHealingAttempts() int
	AttemptHealing(ctx context.Context, diagnosis *ProblemDiagnosis, originalFeedback string, previousScore, currentScore float64) (*HealingResult, error)
}

type service struct {
	logger             stroplog.Logger
	rejectedStatus     string
	pendingStatus      string
	maxHealingAttempts int
}

// NewService creates a refinement service (versioning + stopping + self-healing).
// rejectedStatus and pendingStatus are pipeline-specific strings (e.g. "rejected", "pending").
func NewService(logger stroplog.Logger, rejectedStatus, pendingStatus string, maxHealingAttempts int) ServiceInterface {
	if logger == nil {
		panic("logger cannot be nil")
	}
	if maxHealingAttempts < 0 {
		panic("maxHealingAttempts must be >= 0")
	}
	return &service{
		logger:             logger,
		rejectedStatus:     rejectedStatus,
		pendingStatus:      pendingStatus,
		maxHealingAttempts: maxHealingAttempts,
	}
}

// CalculateVersionAndFeedback calculates start version and feedback.
// Records are assumed ordered by version/created_at (e.g. version ASC, created_at ASC);
// the last record with non-empty feedback is used so regeneration sees the most recent rejection feedback.
func (s *service) CalculateVersionAndFeedback(records []ContentRecord) VersionInfo {
	var nonRejectedCount int
	var latestFeedback string
	for _, r := range records {
		if r.GetEvaluationFeedback() != nil && *r.GetEvaluationFeedback() != "" {
			latestFeedback = *r.GetEvaluationFeedback()
		}
		if r.GetStatus() != s.rejectedStatus {
			nonRejectedCount++
		}
	}
	startVersion := nonRejectedCount + 1
	if nonRejectedCount == 0 && len(records) > 0 {
		s.logger.WithFields(map[string]interface{}{
			"start_version":  startVersion,
			"total_existing": len(records),
		}).Debug("Resetting version counter after rejection, using rejection feedback")
	}
	return VersionInfo{
		StartVersion:     startVersion,
		PreviousFeedback: latestFeedback,
		NonRejectedCount: nonRejectedCount,
	}
}

// CheckMaxVersions checks if we've exceeded max versions; returns generic result for caller to map.
func (s *service) CheckMaxVersions(
	startVersion int,
	maxVersions int,
	records []ContentRecord,
	contextID uuid.UUID,
) (MaxVersionsResult, bool) {
	if startVersion > maxVersions {
		s.logger.WithField("context_id", contextID).Debug("Maximum versions already generated")
		for i := len(records) - 1; i >= 0; i-- {
			if records[i].GetStatus() == s.pendingStatus {
				return MaxVersionsResult{Stopped: true, LastPendingID: records[i].GetID()}, true
			}
		}
		return MaxVersionsResult{Stopped: true}, true
	}
	return MaxVersionsResult{}, false
}

// ShouldStopDueToScoreDecrease checks if we should stop due to score decrease.
func (s *service) ShouldStopDueToScoreDecrease(records []ContentRecord, contextID uuid.UUID) (bool, uuid.UUID) {
	if len(records) < 2 {
		return false, uuid.Nil
	}
	last := records[len(records)-1]
	secondLast := records[len(records)-2]
	if last.GetStatus() == s.rejectedStatus || secondLast.GetStatus() == s.rejectedStatus {
		return false, uuid.Nil
	}
	lastScore, secondLastScore := last.GetScore(), secondLast.GetScore()
	if lastScore == nil || secondLastScore == nil {
		return false, uuid.Nil
	}
	if *lastScore < *secondLastScore {
		s.logger.WithFields(map[string]interface{}{
			"context_id":     contextID,
			"last_score":     *lastScore,
			"previous_score": *secondLastScore,
		}).Debug("Stopping refinement: score decreased in previous execution")
		return true, secondLast.GetID()
	}
	return false, uuid.Nil
}

// CheckStoppingConditions checks stopping conditions during the refinement loop.
func (s *service) CheckStoppingConditions(
	currentScore, previousScore float64,
	version int,
	contextID, selectedID uuid.UUID,
	feedback string,
) (shouldStop bool, returnID uuid.UUID) {
	if currentScore >= 10.0 {
		// Do not treat as perfect if consolidated feedback contains unchecked items ([ ]).
		// The evaluator can output 10.0 while feedback still lists actionable items.
		if strings.Contains(feedback, "[ ]") {
			s.logger.WithFields(map[string]interface{}{
				"version":    version,
				"score":      currentScore,
				"context_id": contextID,
			}).Debug("Score 10.0 but feedback has unchecked items; continuing refinement")
			return false, uuid.Nil
		}
		s.logger.WithFields(map[string]interface{}{
			"version":    version,
			"score":      currentScore,
			"context_id": contextID,
		}).Debug("Perfect score achieved, stopping refinement")
		return true, uuid.Nil
	}
	if previousScore >= 0.0 && currentScore < previousScore {
		if isDeterministicQualityCapFeedback(feedback) {
			s.logger.WithFields(map[string]interface{}{
				"version":        version,
				"current_score":  currentScore,
				"previous_score": previousScore,
				"context_id":     contextID,
			}).Debug("Score decreased due to deterministic quality cap; continuing refinement")
			return false, uuid.Nil
		}
		s.logger.WithFields(map[string]interface{}{
			"version":        version,
			"current_score":  currentScore,
			"previous_score": previousScore,
			"context_id":     contextID,
		}).Debug("Score decreased, stopping refinement")
		return true, selectedID
	}
	return false, uuid.Nil
}

func isDeterministicQualityCapFeedback(feedback string) bool {
	lower := strings.ToLower(feedback)
	return strings.Contains(lower, "reader quality") ||
		strings.Contains(lower, "ideation plan") ||
		strings.Contains(lower, "pun-domain")
}

// MaxHealingAttempts returns the maximum number of healing attempts allowed.
func (s *service) MaxHealingAttempts() int {
	return s.maxHealingAttempts
}

// AttemptHealing builds corrective feedback from a diagnosis (self-healing as part of refinement).
func (s *service) AttemptHealing(
	ctx context.Context,
	diagnosis *ProblemDiagnosis,
	originalFeedback string,
	previousScore, currentScore float64,
) (*HealingResult, error) {
	if diagnosis == nil {
		diagnosis = DiagnoseFromFeedbackOnly(originalFeedback)
	}
	correctiveFeedback := buildCorrectiveFeedbackFromDiagnosis(diagnosis, originalFeedback)
	s.logger.WithFields(map[string]interface{}{
		"problem_type":        diagnosis.ProblemType,
		"score_decrease":      previousScore - currentScore,
		"corrective_feedback": correctiveFeedback,
	}).Debug("Self-healing: Generated corrective feedback")
	return &HealingResult{
		ShouldRetry:        true,
		CorrectiveFeedback: correctiveFeedback,
		ProblemType:        diagnosis.ProblemType,
		ProblemDescription: diagnosis.Description,
	}, nil
}

// DiagnoseFromFeedbackOnly produces a diagnosis when output comparison is not available.
func DiagnoseFromFeedbackOnly(feedback string) *ProblemDiagnosis {
	feedbackLower := strings.ToLower(feedback)
	var problemType string
	var description string
	switch {
	case strings.Contains(feedbackLower, "idiomatic"):
		problemType = "over-idiomatization"
		description = "The attempt to make the content more idiomatic went too far, potentially losing meaning or nuance"
	case strings.Contains(feedbackLower, "literal"):
		problemType = "over-literalization"
		description = "The attempt to make the content more literal went too far, making it awkward or unnatural"
	case strings.Contains(feedbackLower, "semantic") || strings.Contains(feedbackLower, "natural"):
		problemType = "over-correction"
		description = "The attempt to improve the content went too far, changing aspects that should have been preserved"
	default:
		problemType = "general_decrease"
		description = "The content quality decreased, likely due to over-correction or misinterpretation of feedback"
	}
	return &ProblemDiagnosis{
		ProblemType:    problemType,
		Description:    description,
		AffectedFields: []string{},
	}
}

func buildCorrectiveFeedbackFromDiagnosis(diagnosis *ProblemDiagnosis, originalFeedback string) string {
	var sb strings.Builder
	sb.WriteString("SELF-HEALING CORRECTION: The previous attempt to address the feedback led to a score decrease. ")
	sb.WriteString(diagnosis.Description)
	sb.WriteString("\n\n")
	switch diagnosis.ProblemType {
	case "over-idiomatization":
		sb.WriteString("CORRECTIVE GUIDANCE: Keep the content natural and clear. Only add idioms or style if they enhance understanding. Do not over-style at the expense of clarity.\n\n")
	case "over-literalization":
		sb.WriteString("CORRECTIVE GUIDANCE: Preserve accuracy but avoid making the content awkward or unnatural. Maintain clarity while staying faithful to the source.\n\n")
	case "over-correction":
		sb.WriteString("CORRECTIVE GUIDANCE: Make targeted improvements based on the original feedback, but preserve aspects that were working well. Do not change elements that were not mentioned in the feedback.\n\n")
	case "misinterpretation":
		sb.WriteString("CORRECTIVE GUIDANCE: Focus on the specific element(s) mentioned in the original feedback. Preserve other elements exactly unless explicitly asked to change them.\n\n")
	default:
		sb.WriteString("CORRECTIVE GUIDANCE: Review the original feedback carefully and make targeted improvements without over-correcting. Preserve what was working well in the previous version.\n\n")
	}
	sb.WriteString("ORIGINAL FEEDBACK (for reference): ")
	sb.WriteString(originalFeedback)
	return sb.String()
}
