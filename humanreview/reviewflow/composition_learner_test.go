package reviewflow_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/behaviorengineering/strop/humanreview"
	"github.com/behaviorengineering/strop/humanreview/reviewflow"
)

type fakePack struct {
	*humanreview.StaticLearningPack
	candidates []humanreview.LearningCandidate
	extractErr error
	valid      bool
	chain      map[string]interface{}
	approved   map[string]interface{}
}

func (p *fakePack) ExtractAfterApproval(context.Context, *humanreview.HumanEvaluation) ([]humanreview.LearningCandidate, error) {
	if p.extractErr != nil {
		return nil, p.extractErr
	}
	return p.candidates, nil
}

func (p *fakePack) AccountabilityContext(context.Context, *humanreview.HumanEvaluation) (map[string]interface{}, map[string]interface{}, error) {
	return p.chain, p.approved, nil
}

func (p *fakePack) PreviewCandidate(content map[string]interface{}) string {
	return reviewflow.FormatCandidatePreview(content)
}

func (p *fakePack) ValidateCandidate(humanreview.LearningCandidate) bool {
	return p.valid
}

type fakeLearningService struct {
	matches   []*humanreview.LearningArtifact
	conflicts []*humanreview.LearningArtifact
	merged    int
	stored    int
	removed   int
	accountable []humanreview.AccountableCandidate
	decisions []humanreview.QualityDecision
}

func (s *fakeLearningService) StoreLearning(context.Context, *humanreview.LearningArtifact) error {
	s.stored++
	return nil
}
func (s *fakeLearningService) GetExamplesForGeneration(context.Context, humanreview.Job, humanreview.Step, map[string]interface{}, int) ([]*humanreview.LearningArtifact, error) {
	return nil, nil
}
func (s *fakeLearningService) GetGuidesForGeneration(context.Context, humanreview.Job, humanreview.Step, map[string]interface{}, int) ([]string, error) {
	return nil, nil
}
func (s *fakeLearningService) FindCandidatesForMerge(context.Context, humanreview.Job, humanreview.Step, string, humanreview.RetrievalSnapshot) ([]*humanreview.LearningArtifact, error) {
	return nil, nil
}
func (s *fakeLearningService) FindMergePeers(context.Context, humanreview.Job, humanreview.Step, string, humanreview.RetrievalSnapshot) ([]*humanreview.LearningArtifact, []*humanreview.LearningArtifact, error) {
	return s.matches, s.conflicts, nil
}
func (s *fakeLearningService) MergeIntoExisting(context.Context, uuid.UUID, *humanreview.LearningArtifact) error {
	s.merged++
	return nil
}
func (s *fakeLearningService) UpsertItemObjective(context.Context, *humanreview.ItemObjective) error {
	return nil
}
func (s *fakeLearningService) GetItemObjective(context.Context, humanreview.PipelineType, uuid.UUID, humanreview.Job) (*humanreview.ItemObjective, error) {
	return nil, nil
}
func (s *fakeLearningService) RemoveFromIndex(context.Context, uuid.UUID) error {
	s.removed++
	return nil
}
func (s *fakeLearningService) ReindexApproved(context.Context) (int, error) { return 0, nil }
func (s *fakeLearningService) RecordDemoUse(context.Context, humanreview.PipelineType, uuid.UUID, humanreview.Job, humanreview.Step, uuid.UUID, humanreview.DemoSelection) error {
	return nil
}
func (s *fakeLearningService) RecordRejectForVersion(context.Context, humanreview.PipelineType, humanreview.Job, uuid.UUID) error {
	return nil
}
func (s *fakeLearningService) ListAccountableCandidates(context.Context, uuid.UUID, humanreview.Job) ([]humanreview.AccountableCandidate, error) {
	return s.accountable, nil
}
func (s *fakeLearningService) ApplyQualityDecision(_ context.Context, decision humanreview.QualityDecision) error {
	s.decisions = append(s.decisions, decision)
	return nil
}

type fakeStore struct {
	created []*humanreview.LearningArtifact
	pending []*humanreview.LearningArtifact
	updated []*humanreview.LearningArtifact
}

func (s *fakeStore) Create(_ context.Context, artifact *humanreview.LearningArtifact) error {
	s.created = append(s.created, artifact)
	s.pending = append(s.pending, artifact)
	return nil
}
func (s *fakeStore) GetByID(context.Context, uuid.UUID) (*humanreview.LearningArtifact, error) {
	return nil, nil
}
func (s *fakeStore) Update(_ context.Context, artifact *humanreview.LearningArtifact) error {
	s.updated = append(s.updated, artifact)
	return nil
}
func (s *fakeStore) Delete(context.Context, uuid.UUID) error { return nil }
func (s *fakeStore) GetPendingByEvaluationID(context.Context, uuid.UUID) ([]*humanreview.LearningArtifact, error) {
	return s.pending, nil
}
func (s *fakeStore) FindByEvaluationJobStepAndType(context.Context, uuid.UUID, humanreview.Job, humanreview.Step, string) (*humanreview.LearningArtifact, error) {
	return nil, nil
}
func (s *fakeStore) ListByEvaluationJobStepAndType(context.Context, uuid.UUID, humanreview.Job, humanreview.Step, string) ([]*humanreview.LearningArtifact, error) {
	return s.pending, nil
}
func (s *fakeStore) GetApprovedByJobStep(context.Context, humanreview.Job, humanreview.Step, string, map[string]interface{}, int) ([]*humanreview.LearningArtifact, error) {
	return nil, nil
}

type autoMergeUI struct{ action humanreview.MergeAction }

func (u autoMergeUI) PromptMerge(context.Context, humanreview.LearningCandidate, humanreview.RetrievalSnapshot, []*humanreview.LearningArtifact, []*humanreview.LearningArtifact) (humanreview.MergeAction, error) {
	return u.action, nil
}

type autoReviewUI struct{ approve bool }

func (u autoReviewUI) PromptApprove(context.Context, *humanreview.LearningArtifact) (bool, error) {
	return u.approve, nil
}

type autoAccountUI struct{ action string }

func (u autoAccountUI) PromptAction(context.Context, humanreview.AccountableCandidate, string, string) (string, error) {
	return u.action, nil
}

func TestCompositionLearner_createAndApprove(t *testing.T) {
	job := humanreview.Job("compose_job")
	pack := &fakePack{
		StaticLearningPack: humanreview.NewStaticLearningPack("pipe", job),
		valid:              true,
		candidates: []humanreview.LearningCandidate{{
			Type: humanreview.ArtifactTypeGeneratorExample,
			Content: map[string]interface{}{
				"job":  string(job),
				"step": "compose",
				"output": map[string]interface{}{
					"hook": "h",
				},
			},
		}},
	}
	learning := &fakeLearningService{}
	store := &fakeStore{}
	learner, err := reviewflow.NewCompositionLearner(reviewflow.CompositionLearnerDeps{
		Learning:  learning,
		Store:     store,
		Pack:      pack,
		MergeUI:   autoMergeUI{action: humanreview.MergeActionCreate},
		ReviewUI:  autoReviewUI{approve: true},
		AccountUI: autoAccountUI{action: humanreview.AccountabilityActionIgnore},
	})
	require.NoError(t, err)

	eval := &humanreview.HumanEvaluation{
		ID:           uuid.New(),
		RootEntityID: uuid.New(),
		Job:          job,
		Status:       humanreview.StatusReadyForLearning,
	}
	require.NoError(t, learner.AfterApproval(context.Background(), eval))
	assert.Len(t, store.created, 1)
	assert.Equal(t, 1, learning.stored)
}

func TestCompositionLearner_createsPerSectionCandidates(t *testing.T) {
	job := humanreview.Job("post_polish_generation")
	pack := &fakePack{
		StaticLearningPack: humanreview.NewStaticLearningPack("sayings", job),
		valid:              true,
		candidates: []humanreview.LearningCandidate{
			{
				Type: humanreview.ArtifactTypeGeneratorExample,
				Content: map[string]interface{}{
					"job":  string(job),
					"step": "post_polish",
					"input": map[string]interface{}{
						"focus_section": "tldr",
						"section_text":  "src",
					},
					"output": map[string]interface{}{"polished_section": "out1"},
					"context": map[string]interface{}{
						"section_id": "tldr",
					},
				},
			},
			{
				Type: humanreview.ArtifactTypeGeneratorExample,
				Content: map[string]interface{}{
					"job":  string(job),
					"step": "post_polish",
					"input": map[string]interface{}{
						"focus_section": "fluff",
						"section_text":  "src",
					},
					"output": map[string]interface{}{"polished_section": "out2"},
					"context": map[string]interface{}{
						"section_id": "fluff",
					},
				},
			},
		},
	}
	learning := &fakeLearningService{}
	store := &fakeStore{}
	learner, err := reviewflow.NewCompositionLearner(reviewflow.CompositionLearnerDeps{
		Learning: learning,
		Store:    store,
		Pack:     pack,
		MergeUI:  autoMergeUI{action: humanreview.MergeActionCreate},
		ReviewUI: autoReviewUI{approve: true},
	})
	require.NoError(t, err)

	eval := &humanreview.HumanEvaluation{
		ID:           uuid.New(),
		RootEntityID: uuid.New(),
		Job:          job,
		Status:       humanreview.StatusReadyForLearning,
	}
	require.NoError(t, learner.AfterApproval(context.Background(), eval))
	assert.Len(t, store.created, 2)
	assert.Equal(t, 2, learning.stored)
}

func TestCompositionLearner_mergeUpdate(t *testing.T) {
	job := humanreview.Job("compose_job")
	existingID := uuid.New()
	existing := &humanreview.LearningArtifact{
		ID:           existingID,
		ArtifactType: humanreview.ArtifactTypeGeneratorExample,
		ArtifactContent: humanreview.PutSnapshot(map[string]interface{}{
			"job":  string(job),
			"step": "compose",
		}, humanreview.RetrievalSnapshot{DistinctiveMove: "same-move"}),
	}
	pack := &fakePack{
		StaticLearningPack: humanreview.NewStaticLearningPack("pipe", job),
		valid:              true,
		candidates: []humanreview.LearningCandidate{{
			Type: humanreview.ArtifactTypeGeneratorExample,
			Content: humanreview.PutSnapshot(map[string]interface{}{
				"job":  string(job),
				"step": "compose",
			}, humanreview.RetrievalSnapshot{DistinctiveMove: "same-move"}),
		}},
	}
	learning := &fakeLearningService{matches: []*humanreview.LearningArtifact{existing}}
	store := &fakeStore{}
	learner, err := reviewflow.NewCompositionLearner(reviewflow.CompositionLearnerDeps{
		Learning: learning,
		Store:    store,
		Pack:     pack,
		MergeUI:  autoMergeUI{action: humanreview.MergeActionUpdate},
	})
	require.NoError(t, err)

	eval := &humanreview.HumanEvaluation{ID: uuid.New(), RootEntityID: uuid.New(), Job: job}
	require.NoError(t, learner.AfterApproval(context.Background(), eval))
	assert.Equal(t, 1, learning.merged)
	assert.Empty(t, store.created)
}

func TestCompositionLearner_skipInvalidAndNonComposition(t *testing.T) {
	job := humanreview.Job("compose_job")
	pack := &fakePack{
		StaticLearningPack: humanreview.NewStaticLearningPack("pipe", job),
		valid:              false,
		candidates: []humanreview.LearningCandidate{{
			Type:    humanreview.ArtifactTypeGeneratorExample,
			Content: map[string]interface{}{"job": string(job), "step": "compose"},
		}},
	}
	learning := &fakeLearningService{}
	store := &fakeStore{}
	learner, err := reviewflow.NewCompositionLearner(reviewflow.CompositionLearnerDeps{
		Learning: learning,
		Store:    store,
		Pack:     pack,
	})
	require.NoError(t, err)

	eval := &humanreview.HumanEvaluation{ID: uuid.New(), Job: job}
	require.NoError(t, learner.AfterApproval(context.Background(), eval))
	assert.Empty(t, store.created)

	eval.Job = "other"
	require.NoError(t, learner.AfterApproval(context.Background(), eval))
}

func TestCompositionLearner_accountability(t *testing.T) {
	job := humanreview.Job("compose_job")
	artID := uuid.New()
	pack := &fakePack{
		StaticLearningPack: humanreview.NewStaticLearningPack("pipe", job),
		valid:              true,
	}
	learning := &fakeLearningService{
		accountable: []humanreview.AccountableCandidate{{
			Artifact: &humanreview.LearningArtifact{ID: artID},
		}},
	}
	store := &fakeStore{}
	learner, err := reviewflow.NewCompositionLearner(reviewflow.CompositionLearnerDeps{
		Learning:  learning,
		Store:     store,
		Pack:      pack,
		AccountUI: autoAccountUI{action: humanreview.AccountabilityActionPenalize},
	})
	require.NoError(t, err)

	eval := &humanreview.HumanEvaluation{ID: uuid.New(), RootEntityID: uuid.New(), Job: job}
	require.NoError(t, learner.AfterApproval(context.Background(), eval))
	require.Len(t, learning.decisions, 1)
	assert.Equal(t, humanreview.AccountabilityActionPenalize, learning.decisions[0].Action)
	assert.Equal(t, artID, learning.decisions[0].ArtifactID)
}
