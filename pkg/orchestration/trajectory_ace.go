package orchestration

import (
	"context"
	"errors"

	"github.com/behaviorengineering/strop/pkg/dspy/ace"
)

// recordACEPipelineStep mirrors pipeline attempts into an ambient ACE recorder when enabled.
func recordACEPipelineStep(ctx context.Context, rec PipelineAttempt) {
	recorder := ace.RecorderFromContext(ctx)
	if recorder == nil {
		return
	}
	tool := rec.Scope
	if tool == "" {
		tool = "pipeline"
	}
	reasoning := rec.Feedback
	if rec.Err != "" {
		reasoning = rec.Err
	}
	var err error
	if rec.Err != "" && !rec.Passed {
		err = errors.New(rec.Err)
	}
	input := map[string]any{
		"attempt":       rec.Attempt,
		"failure_class": string(rec.FailureClass),
	}
	output := map[string]any{
		"passed": rec.Passed,
		"score":  rec.Score,
		"action": rec.Action,
	}
	recorder.RecordStep(rec.Action, tool, reasoning, input, output, err)
}
