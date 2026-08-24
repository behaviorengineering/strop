package humanreview

import "testing"

func TestLearningConstants(t *testing.T) {
	if LearningReviewStatusPending != "pending" || ArtifactTypeGeneratorExample != "generator_example" {
		t.Fatalf("unexpected learning constants")
	}
}
