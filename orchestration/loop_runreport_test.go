package orchestration

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/behaviorengineering/strop/refinement"
	"github.com/behaviorengineering/strop/runreport"
	"github.com/behaviorengineering/strop/streaming"

	"github.com/google/uuid"
)

type failingRefinementStrategy struct {
	fakeRefinementStrategy
	generateErr error
}

func (s *failingRefinementStrategy) GenerateAndEvaluate(
	context.Context,
	int,
	string,
	interface{},
	streaming.EventChannel,
) (*IterationOutput, error) {
	return nil, s.generateErr
}

func (s *failingRefinementStrategy) RunReportMeta() runreport.Meta {
	return runreport.PipelineJobMeta("test", "refinement")
}

func TestRunRefinementLoopRecordsTerminalErrorInRunReport(t *testing.T) {
	t.Parallel()

	entityID := uuid.New()
	reportDir := t.TempDir()
	ctx := runreport.WithConfig(context.Background(), runreport.Config{
		Enabled:       true,
		Dir:           reportDir,
		MaxAgeHours:   48,
		KeepPerEntity: 2,
	})
	generationErr := errors.New("generator failed")
	strategy := &failingRefinementStrategy{
		generateErr: generationErr,
	}
	policy := refinement.NewService(testStropLogger(), "rejected", "pending", 0)

	_, err := RunRefinementLoop(ctx, entityID, strategy, policy, 3, nil)
	if !errors.Is(err, generationErr) {
		t.Fatalf("expected generation error, got %v", err)
	}

	matches, err := filepath.Glob(filepath.Join(
		reportDir,
		"test",
		"refinement",
		entityID.String(),
		"*.json",
	))
	if err != nil {
		t.Fatalf("find run report: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("found %d run reports, want 1", len(matches))
	}

	data, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read run report: %v", err)
	}
	var report runreport.Report
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("decode run report: %v", err)
	}
	if report.Outcome != runreport.OutcomeFailed {
		t.Fatalf("report outcome = %q, want %q", report.Outcome, runreport.OutcomeFailed)
	}
	if report.Error == "" {
		t.Fatal("expected terminal error in run report")
	}
}
