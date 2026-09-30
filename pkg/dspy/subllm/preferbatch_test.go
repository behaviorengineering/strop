package subllm

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/behaviorengineering/strop/pkg/openaibatch"

	dspyrlm "github.com/XiaoConstantine/dspy-go/pkg/modules/rlm"
)

type recordingSubLLM struct {
	batchCalls atomic.Int32
}

func (r *recordingSubLLM) Query(context.Context, string) (dspyrlm.QueryResponse, error) {
	return dspyrlm.QueryResponse{}, nil
}

func (r *recordingSubLLM) QueryBatched(ctx context.Context, prompts []string) ([]dspyrlm.QueryResponse, error) {
	r.batchCalls.Add(1)
	out := make([]dspyrlm.QueryResponse, len(prompts))
	for i := range prompts {
		out[i] = dspyrlm.QueryResponse{Response: "sync"}
	}
	return out, nil
}

type stubBatchRunner struct {
	calls atomic.Int32
	err   error
}

func (s *stubBatchRunner) RunChatBatch(context.Context, []openaibatch.ChatLine) (map[string]openaibatch.LineResult, error) {
	s.calls.Add(1)
	if s.err != nil {
		return nil, s.err
	}
	return map[string]openaibatch.LineResult{
		"rlm-0": {Content: "b0"},
		"rlm-1": {Content: "b1"},
	}, nil
}

func TestPreferBatch404ProbeFallsBackAndLatches(t *testing.T) {
	sync := &recordingSubLLM{}
	var probes atomic.Int32
	client := openaibatch.Client{BaseURL: "http://example.com"}
	pb := NewPreferBatch(sync, client, "m",
		WithBatchProbe(func(context.Context) (bool, bool, error) {
			probes.Add(1)
			return false, false, nil
		}),
		WithBatchRunner(&stubBatchRunner{}),
	)
	ctx := context.Background()
	_, err := pb.QueryBatched(ctx, []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	if sync.batchCalls.Load() != 1 {
		t.Fatalf("sync calls=%d", sync.batchCalls.Load())
	}
	if probes.Load() != 1 {
		t.Fatalf("probes=%d", probes.Load())
	}
	_, err = pb.QueryBatched(ctx, []string{"c", "d"})
	if err != nil {
		t.Fatal(err)
	}
	if probes.Load() != 1 {
		t.Fatalf("expected latched probe, probes=%d", probes.Load())
	}
	if sync.batchCalls.Load() != 2 {
		t.Fatalf("sync calls=%d", sync.batchCalls.Load())
	}
}

func TestPreferBatchSinglePromptUsesSync(t *testing.T) {
	sync := &recordingSubLLM{}
	runner := &stubBatchRunner{}
	client := openaibatch.Client{BaseURL: "http://example.com"}
	pb := NewPreferBatch(sync, client, "m",
		WithBatchProbe(func(context.Context) (bool, bool, error) { return true, false, nil }),
		WithBatchRunner(runner),
	)
	_, err := pb.QueryBatched(context.Background(), []string{"only"})
	if err != nil {
		t.Fatal(err)
	}
	if runner.calls.Load() != 0 {
		t.Fatal("batch should not run for single prompt")
	}
}

func TestPreferBatchShortDeadlineUsesSync(t *testing.T) {
	sync := &recordingSubLLM{}
	runner := &stubBatchRunner{}
	client := openaibatch.Client{BaseURL: "http://example.com"}
	pb := NewPreferBatch(sync, client, "m",
		WithMinBatchDeadline(30*time.Second),
		WithBatchProbe(func(context.Context) (bool, bool, error) { return true, false, nil }),
		WithBatchRunner(runner),
	)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := pb.QueryBatched(ctx, []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	if runner.calls.Load() != 0 {
		t.Fatal("expected sync fallback for short deadline")
	}
}

func TestPreferBatchThrottleOnCreateFallsBackThisCall(t *testing.T) {
	sync := &recordingSubLLM{}
	runner := &stubBatchRunner{err: fmt.Errorf("openaibatch: create batch: %w", &openaibatch.HTTPError{Op: "create batch", StatusCode: http.StatusTooManyRequests})}
	client := openaibatch.Client{BaseURL: "http://example.com"}
	pb := NewPreferBatch(sync, client, "m",
		WithBatchProbe(func(context.Context) (bool, bool, error) { return true, false, nil }),
		WithBatchRunner(runner),
	)
	_, err := pb.QueryBatched(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	if sync.batchCalls.Load() != 1 {
		t.Fatal("expected sync fallback")
	}
	// Still available: batch runner should be tried again.
	runner.err = nil
	_, err = pb.QueryBatched(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	if runner.calls.Load() != 2 {
		t.Fatalf("batch calls=%d", runner.calls.Load())
	}
}

func TestPreferBatchPostSubmitDoesNotSync(t *testing.T) {
	sync := &recordingSubLLM{}
	runner := &stubBatchRunner{err: fmt.Errorf("openaibatch: poll batch: context deadline exceeded")}
	client := openaibatch.Client{BaseURL: "http://example.com"}
	pb := NewPreferBatch(sync, client, "m",
		WithBatchProbe(func(context.Context) (bool, bool, error) { return true, false, nil }),
		WithBatchRunner(runner),
	)
	_, err := pb.QueryBatched(context.Background(), []string{"a", "b"})
	if err == nil {
		t.Fatal("expected poll error")
	}
	if sync.batchCalls.Load() != 0 {
		t.Fatal("must not sync after batch submit")
	}
}

func TestPreferBatchHappyPath(t *testing.T) {
	sync := &recordingSubLLM{}
	runner := &stubBatchRunner{}
	client := openaibatch.Client{BaseURL: "http://example.com"}
	pb := NewPreferBatch(sync, client, "m",
		WithBatchProbe(func(context.Context) (bool, bool, error) { return true, false, nil }),
		WithBatchRunner(runner),
	)
	got, err := pb.QueryBatched(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Response != "b0" || got[1].Response != "b1" {
		t.Fatalf("got=%v", got)
	}
	if sync.batchCalls.Load() != 0 {
		t.Fatal("sync should not run on happy batch path")
	}
}
