package humanreview

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Learning review / artifact status constants.
const (
	LearningReviewStatusPending  = "pending"
	LearningReviewStatusApproved = "approved"
	LearningReviewStatusRejected = "rejected"
)

// Artifact type constants for learning artifacts.
const (
	ArtifactTypeGeneratorExample   = "generator_example"
	ArtifactTypeEvaluatorExample   = "evaluator_example"
	ArtifactTypeComponentAlignment = "component_alignment"
	ArtifactTypeContentRule        = "content_rule"
)

// Learning artifact quality_status values (demo accountability).
const (
	QualityStatusActive      = "active"
	QualityStatusPenalized   = "penalized"
	QualityStatusQuarantined = "quarantined"
)

// AccountabilityAction is the judge / human decision for a flagged demo.
const (
	AccountabilityActionNone       = "none"
	AccountabilityActionPenalize   = "penalize"
	AccountabilityActionQuarantine = "quarantine"
	AccountabilityActionIgnore     = "ignore"
)

// DemoSelection is the near/contrast pair SetDemos on one Generate call.
type DemoSelection struct {
	NearID     string // UUID string; empty if none
	ContrastID string
	Slot       string // Optional section/phase id when one version records multiple demos.
}

// GenerationDemoUse records which demos sat on one composition version (or one slot within it).
type GenerationDemoUse struct {
	ID                 uuid.UUID    `json:"id"`
	PipelineType       PipelineType `json:"pipeline_type"`
	RootEntityID       uuid.UUID    `json:"root_entity_id"`
	Job                Job          `json:"job"`
	Step               Step         `json:"step"`
	VersionID          uuid.UUID    `json:"version_id"`
	Slot               string       `json:"slot,omitempty"`
	NearArtifactID     uuid.UUID    `json:"near_artifact_id"`
	ContrastArtifactID *uuid.UUID   `json:"contrast_artifact_id,omitempty"`
	CreatedAt          time.Time    `json:"created_at"`
	RejectRecordedAt   *time.Time   `json:"reject_recorded_at,omitempty"`
}

// RejectedDemoVersion is one rejected composition version that used a candidate demo.
type RejectedDemoVersion struct {
	VersionID uuid.UUID `json:"version_id"`
	WasNear   bool      `json:"was_near"`
}

// AccountableCandidate is a demo that crossed reject thresholds and may be judged.
type AccountableCandidate struct {
	Artifact          *LearningArtifact
	ItemRejectCount   int
	GlobalRejectCount int
	WasNearOnItem     bool
	RejectedVersions  []RejectedDemoVersion
}

// QualityDecision applies a human/judge accountability outcome.
type QualityDecision struct {
	ArtifactID uuid.UUID
	Action     string // AccountabilityAction*
	Why        string
}

// LearningArtifact is the portable learning record (no vector field).
type LearningArtifact struct {
	ID                 uuid.UUID              `json:"id"`
	EvaluationID       *uuid.UUID             `json:"evaluation_id,omitempty"`
	ArtifactType       string                 `json:"artifact_type"`
	ArtifactContent    map[string]interface{} `json:"artifact_content"`
	Status             string                 `json:"status"`
	Job                *Job                   `json:"job,omitempty"`
	Step               *Step                  `json:"step,omitempty"`
	Context            map[string]interface{} `json:"context,omitempty"`
	CreatedAt          time.Time              `json:"created_at"`
	ReviewedAt         *time.Time             `json:"reviewed_at,omitempty"`
	UpdatedAt          time.Time              `json:"updated_at"`
	RejectPresentCount int                    `json:"reject_present_count"`
	RejectNearCount    int                    `json:"reject_near_count"`
	QualityStatus      string                 `json:"quality_status"`
	QualityScore       int                    `json:"quality_score"`
	LastEvaluatedAt    *time.Time             `json:"last_evaluated_at,omitempty"`
}

// LearningService is the portable learning contract used by review flows and JobRunner adapters.
type LearningService interface {
	StoreLearning(ctx context.Context, artifact *LearningArtifact) error
	GetExamplesForGeneration(
		ctx context.Context,
		job Job,
		step Step,
		contextMap map[string]interface{},
		limit int,
	) ([]*LearningArtifact, error)
	// GetGuidesForGeneration returns transferable principle strings from approved content_rule rows.
	GetGuidesForGeneration(
		ctx context.Context,
		job Job,
		step Step,
		contextMap map[string]interface{},
		limit int,
	) ([]string, error)
	FindCandidatesForMerge(
		ctx context.Context,
		job Job,
		step Step,
		artifactType string,
		snapshot RetrievalSnapshot,
	) ([]*LearningArtifact, error)
	// FindMergePeers lists approved artifacts for job/step/type and splits them into
	// identity matches vs distinctive-move conflicts (Postgres filters; no Meilisearch).
	FindMergePeers(
		ctx context.Context,
		job Job,
		step Step,
		artifactType string,
		snapshot RetrievalSnapshot,
	) (matches []*LearningArtifact, conflicts []*LearningArtifact, err error)
	MergeIntoExisting(ctx context.Context, existingID uuid.UUID, updated *LearningArtifact) error
	UpsertItemObjective(ctx context.Context, objective *ItemObjective) error
	GetItemObjective(
		ctx context.Context,
		pipelineType PipelineType,
		rootEntityID uuid.UUID,
		job Job,
	) (*ItemObjective, error)
	// RemoveFromIndex deletes the artifact from the search index (best-effort after reject/delete).
	RemoveFromIndex(ctx context.Context, id uuid.UUID) error
	// ReindexApproved rebuilds search documents for approved generator examples in this pipeline store.
	ReindexApproved(ctx context.Context) (int, error)
	// RecordDemoUse stores near/contrast IDs for a composition version (no-op if NearID empty).
	RecordDemoUse(
		ctx context.Context,
		pipelineType PipelineType,
		rootEntityID uuid.UUID,
		job Job,
		step Step,
		versionID uuid.UUID,
		demos DemoSelection,
	) error
	// RecordRejectForVersion increments reject counters once per version (idempotent).
	RecordRejectForVersion(ctx context.Context, pipelineType PipelineType, job Job, versionID uuid.UUID) error
	// ListAccountableCandidates returns demos that crossed item or global reject thresholds.
	ListAccountableCandidates(
		ctx context.Context,
		rootEntityID uuid.UUID,
		job Job,
	) ([]AccountableCandidate, error)
	// ApplyQualityDecision updates quality fields and reindexes (or stamps cooldown on ignore).
	ApplyQualityDecision(ctx context.Context, decision QualityDecision) error
}

// LearningStore is the portable persistence contract (no vector search).
// GetByID returns (nil, nil) when the row is missing.
type LearningStore interface {
	Create(ctx context.Context, artifact *LearningArtifact) error
	GetByID(ctx context.Context, id uuid.UUID) (*LearningArtifact, error)
	Update(ctx context.Context, artifact *LearningArtifact) error
	Delete(ctx context.Context, id uuid.UUID) error
	GetPendingByEvaluationID(ctx context.Context, evaluationID uuid.UUID) ([]*LearningArtifact, error)
	// FindByEvaluationJobStepAndType returns an existing row or (nil, nil).
	FindByEvaluationJobStepAndType(
		ctx context.Context,
		evaluationID uuid.UUID,
		job Job,
		step Step,
		artifactType string,
	) (*LearningArtifact, error)
	// ListByEvaluationJobStepAndType returns pending+approved rows for eval/job/step/type.
	ListByEvaluationJobStepAndType(
		ctx context.Context,
		evaluationID uuid.UUID,
		job Job,
		step Step,
		artifactType string,
	) ([]*LearningArtifact, error)
	GetApprovedByJobStep(
		ctx context.Context,
		job Job,
		step Step,
		artifactType string,
		contextMap map[string]interface{},
		limit int,
	) ([]*LearningArtifact, error)
}
