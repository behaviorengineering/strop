package humanreview_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/behaviorengineering/strop/humanreview"
)

func TestCandidateIdentityKey_sectionAndPrinciple(t *testing.T) {
	t.Parallel()

	sectionA := humanreview.CandidateIdentityKey(humanreview.ArtifactTypeGeneratorExample, map[string]interface{}{
		"job":  "post_polish_generation",
		"step": "post_polish",
		"context": map[string]interface{}{
			"section_id": "tldr",
		},
	})
	sectionB := humanreview.CandidateIdentityKey(humanreview.ArtifactTypeGeneratorExample, map[string]interface{}{
		"job":  "post_polish_generation",
		"step": "post_polish",
		"context": map[string]interface{}{
			"section_id": "fluff",
		},
	})
	assert.NotEqual(t, sectionA, sectionB)

	guideA := humanreview.CandidateIdentityKey(humanreview.ArtifactTypeContentRule, map[string]interface{}{
		"job":       "translation_generation",
		"step":      "translate",
		"principle": "Keep proverb meaning first",
	})
	guideB := humanreview.CandidateIdentityKey(humanreview.ArtifactTypeContentRule, map[string]interface{}{
		"job":       "translation_generation",
		"step":      "translate",
		"principle": "Prefer concrete imagery",
	})
	assert.NotEqual(t, guideA, guideB)
	assert.True(t, humanreview.HasCandidateIdentity([]*humanreview.LearningArtifact{{
		ArtifactType:    humanreview.ArtifactTypeContentRule,
		ArtifactContent: map[string]interface{}{"job": "translation_generation", "step": "translate", "principle": "Keep proverb meaning first"},
	}}, humanreview.LearningCandidate{
		Type:    humanreview.ArtifactTypeContentRule,
		Content: map[string]interface{}{"job": "translation_generation", "step": "translate", "principle": "Keep proverb meaning first"},
	}))
}
