package orchestration

import (
	"context"
	"errors"
	"testing"
)

func TestClassifyFailure(t *testing.T) {
	if ClassifyFailure(errors.New("status 429")) != FailureTransientInfra {
		t.Fatal("429")
	}
	if ClassifyFailure(context.Canceled) != FailureCancelled {
		t.Fatal("cancelled")
	}
	if ClassifyFailure(errors.New("composition phase x failed after 3 attempts: low score")) != FailureSemanticGate {
		t.Fatal("semantic")
	}
}

func TestPipelineTrajectorySnapshot(t *testing.T) {
	tr := NewPipelineTrajectory("entity", "job")
	ctx := WithPipelineTrajectory(context.Background(), tr)
	RecordPipelineAttempt(ctx, PipelineAttempt{Scope: "composition:p1", Action: "generate_gate", Attempt: 1, Passed: true})
	if len(tr.Snapshot()) != 1 {
		t.Fatalf("want 1 attempt, got %d", len(tr.Snapshot()))
	}
}

func TestRecordPipelineAttemptNoTrajectory(t *testing.T) {
	RecordPipelineAttempt(context.Background(), PipelineAttempt{Scope: "x"})
}
