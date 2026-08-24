package orchestration

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/behaviorengineering/strop/evaluation"
	"github.com/behaviorengineering/strop/streaming"
)

type fakeSectionRunner struct {
	t            *testing.T
	calls        []SectionFieldRequest
	failFirst    map[string]bool
	seenAttempts map[string]int
}

func (f *fakeSectionRunner) Run(_ context.Context, req SectionFieldRequest, _ streaming.EventChannel) (*SectionFieldResponse, error) {
	f.calls = append(f.calls, req)
	if f.seenAttempts == nil {
		f.seenAttempts = map[string]int{}
	}
	f.seenAttempts[req.SectionID]++
	attempt := f.seenAttempts[req.SectionID]

	if req.SectionID == "description" {
		_, hasTldr := req.LockedOutput["tldr"]
		assert.False(f.t, hasTldr, "upcoming tldr must not be locked")
	}

	score := 8.0
	out := "out-" + req.SectionID
	fb := "ok-" + req.SectionID
	if f.failFirst[req.SectionID] && attempt == 1 {
		score = 3.0
		out = "weak-" + req.SectionID
		fb = "retry-" + req.SectionID
	}
	if attempt > 1 {
		assert.Equal(f.t, "weak-"+req.SectionID, req.PreviousFailedOutput)
		assert.Contains(f.t, req.PreviousFeedback, "retry-")
	}

	return &SectionFieldResponse{
		OutputText: out,
		Rationale:  "r-" + req.SectionID,
		Eval: &evaluation.AggregatedEvaluation{
			WeightedScore:        score,
			CriterionScores:      map[string]float64{"clarity": score},
			ConsolidatedFeedback: fb,
		},
	}, nil
}

func testSectionDefinition() DocumentSectionDefinition {
	return DocumentSectionDefinition{
		Name:         "test_sections",
		SectionIDs:   []string{"teaser", "description", "tldr", "fluff", "going_deeper"},
		MaxAttempts:  2,
		MinPassScore: 7.0,
	}
}

func stringMapCodec() SectionCodec[map[string]string] {
	return SectionCodec[map[string]string]{
		ToMap: func(d map[string]string) map[string]string {
			if d == nil {
				return map[string]string{}
			}
			out := make(map[string]string, len(d))
			for k, v := range d {
				out[k] = v
			}
			return out
		},
		FromMap: func(m map[string]string) map[string]string {
			if m == nil {
				return map[string]string{}
			}
			out := make(map[string]string, len(m))
			for k, v := range m {
				out[k] = v
			}
			return out
		},
	}
}

func TestSectionWalkStrategy_emptyThenAggregate(t *testing.T) {
	t.Parallel()
	seed := map[string]string{
		"description": "d",
		"tldr":        "tl",
		"fluff":       "f",
		"going_deeper": "g",
	}
	runner := &fakeSectionRunner{t: t, failFirst: map[string]bool{"description": true}}
	strat := NewSectionWalkStrategy(SectionWalkConfig[map[string]string]{
		Sections: testSectionDefinition(),
		Seed:     seed,
		Version:  1,
		VersionFeedback: "outer-feedback",
		Runner:   runner,
		Codec:    stringMapCodec(),
		EmptyResultErr: fmt.Errorf("empty"),
	})
	result, err := RunCompositionLoop(context.Background(), strat, nil)
	require.NoError(t, err)
	require.NotNil(t, result)

	st, ok := result.OutputState.(*SectionWalkState[map[string]string])
	require.True(t, ok)
	assert.Equal(t, "", st.Draft["teaser"])
	assert.Equal(t, "out-description", st.Draft["description"])

	eval, ok := result.EvalPayload.(*evaluation.AggregatedEvaluation)
	require.True(t, ok)
	require.NotNil(t, eval)
	assert.Len(t, st.Scores, 4)
	assert.InDelta(t, meanScore(st.Scores), result.Score, 0.001)

	require.NotEmpty(t, runner.calls)
	assert.Equal(t, "outer-feedback", runner.calls[0].PreviousFeedback)
	assert.Equal(t, "description", runner.calls[0].SectionID)
}

func TestSectionWalkStrategy_sourceTextFunc(t *testing.T) {
	t.Parallel()
	en := map[string]string{
		"teaser": "EN-t", "description": "EN-d", "tldr": "EN-tl",
		"fluff": "EN-f", "going_deeper": "EN-g",
	}
	seedES := map[string]string{}
	runner := &fakeSectionRunner{t: t}
	codec := stringMapCodec()
	codec.SourceText = func(_ map[string]string, sectionID string) string {
		return en[sectionID]
	}
	strat := NewSectionWalkStrategy(SectionWalkConfig[map[string]string]{
		Sections: testSectionDefinition(),
		Seed:     seedES,
		Version:  1,
		Runner:   runner,
		Codec:    codec,
		EmptyResultErr: fmt.Errorf("empty"),
	})
	result, err := RunCompositionLoop(context.Background(), strat, nil)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, runner.calls, 5)
	assert.Equal(t, "EN-t", runner.calls[0].SourceText)
	st := result.OutputState.(*SectionWalkState[map[string]string])
	assert.Equal(t, "out-teaser", st.Draft["teaser"])
}

func meanScore(scores []float64) float64 {
	if len(scores) == 0 {
		return 0
	}
	var sum float64
	for _, s := range scores {
		sum += s
	}
	return sum / float64(len(scores))
}

func TestNewSectionWalkStrategy_nilRunnerPanics(t *testing.T) {
	assert.Panics(t, func() {
		NewSectionWalkStrategy(SectionWalkConfig[map[string]string]{
			Sections: testSectionDefinition(),
			Codec:    stringMapCodec(),
		})
	})
}

func TestNewSectionWalkStrategy_nilCodecPanics(t *testing.T) {
	assert.Panics(t, func() {
		NewSectionWalkStrategy(SectionWalkConfig[map[string]string]{
			Sections: testSectionDefinition(),
			Runner:   &fakeSectionRunner{t: t},
		})
	})
}
