package registry

import (
	"context"
	"testing"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/behaviorengineering/strop/evaluation"
)

// Test role types for different pipelines.
type SayingsAgentRole string
type SayingsFeedbackRole string
type VideosAgentRole string
type VideosFeedbackRole string

// Test constants.
const (
	SayingsRoleEvaluator1 SayingsAgentRole    = "sayings_evaluator_1"
	SayingsRoleEvaluator2 SayingsAgentRole    = "sayings_evaluator_2"
	SayingsRoleFeedback1  SayingsFeedbackRole = "sayings_feedback_1"
	VideosRoleEvaluator1  VideosAgentRole     = "videos_evaluator_1"
	VideosRoleFeedback1   VideosFeedbackRole  = "videos_feedback_1"
)

// Mock module for testing. Implements core.Module (Process, Clone, GetSignature, SetSignature, SetLLM, GetDisplayName, GetModuleType).
type mockModule struct {
	id string
}

func (m *mockModule) Process(ctx context.Context, inputs map[string]interface{}, opts ...core.Option) (map[string]interface{}, error) {
	return inputs, nil
}

func (m *mockModule) Clone() core.Module {
	return &mockModule{id: m.id}
}

func (m *mockModule) GetSignature() core.Signature {
	return core.Signature{}
}

func (m *mockModule) SetSignature(signature core.Signature) {}

func (m *mockModule) SetLLM(llm core.LLM) {}

func (m *mockModule) GetDisplayName() string {
	return "mock_" + m.id
}

func (m *mockModule) GetModuleType() string {
	return "mock"
}

func newMockModule(id string) core.Module {
	return &mockModule{id: id}
}

// TestConvertExpertMap tests the ConvertExpertMap helper function.
func TestConvertExpertMap(t *testing.T) {
	t.Run("converts sayings expert map", func(t *testing.T) {
		sayingsMap := map[SayingsFeedbackRole]core.Module{
			SayingsRoleFeedback1: newMockModule("module1"),
		}

		result := ConvertExpertMap(sayingsMap)

		assert.Equal(t, 1, len(result))
		key := evaluation.ExpertKey(SayingsRoleFeedback1)
		assert.Equal(t, newMockModule("module1"), result[key].Module)
		assert.Equal(t, key, result[key].Key)
	})

	t.Run("handles nil map", func(t *testing.T) {
		var nilMap map[SayingsFeedbackRole]core.Module
		result := ConvertExpertMap(nilMap)
		assert.Nil(t, result)
	})

	t.Run("handles empty map", func(t *testing.T) {
		emptyMap := make(map[SayingsFeedbackRole]core.Module)
		result := ConvertExpertMap(emptyMap)
		assert.NotNil(t, result)
		assert.Equal(t, 0, len(result))
	})
}

func TestConvertEvaluatorMap(t *testing.T) {
	t.Run("converts string-like evaluator role map", func(t *testing.T) {
		sayingsMap := map[SayingsAgentRole]core.Module{
			SayingsRoleEvaluator1: newMockModule("module1"),
			SayingsRoleEvaluator2: newMockModule("module2"),
		}

		result := ConvertEvaluatorMap(sayingsMap)

		assert.Equal(t, 2, len(result))
		assert.Equal(t, newMockModule("module1"), result[evaluation.EvaluatorKey(SayingsRoleEvaluator1)])
		assert.Equal(t, newMockModule("module2"), result[evaluation.EvaluatorKey(SayingsRoleEvaluator2)])
	})

	t.Run("handles nil map", func(t *testing.T) {
		var nilMap map[SayingsAgentRole]core.Module
		result := ConvertEvaluatorMap(nilMap)
		assert.Nil(t, result)
	})
}

// TestModuleRegistry_MultiPipelineSupport tests that the registry supports multiple pipelines.
func TestModuleRegistry_MultiPipelineSupport(t *testing.T) {
	registry := NewModuleRegistry()

	// Register sayings pipeline evaluators
	sayingsEvaluators := map[SayingsAgentRole]core.Module{
		SayingsRoleEvaluator1: newMockModule("sayings_eval_1"),
		SayingsRoleEvaluator2: newMockModule("sayings_eval_2"),
	}
	registry.RegisterEvaluators("sayings_translation", WrapEvaluators(ConvertEvaluatorMap(sayingsEvaluators), nil))

	// Register videos pipeline evaluators
	videosEvaluators := map[VideosAgentRole]core.Module{
		VideosRoleEvaluator1: newMockModule("videos_eval_1"),
	}
	registry.RegisterEvaluators("videos_transcript", WrapEvaluators(ConvertEvaluatorMap(videosEvaluators), nil))

	// Retrieve sayings evaluators
	retrievedSayings, err := registry.GetEvaluators("sayings_translation")
	require.NoError(t, err)
	assert.Equal(t, 2, len(retrievedSayings))
	assert.Equal(t, newMockModule("sayings_eval_1"), retrievedSayings[evaluation.EvaluatorKey(SayingsRoleEvaluator1)].Module)
	assert.Equal(t, newMockModule("sayings_eval_2"), retrievedSayings[evaluation.EvaluatorKey(SayingsRoleEvaluator2)].Module)

	// Retrieve videos evaluators
	retrievedVideos, err := registry.GetEvaluators("videos_transcript")
	require.NoError(t, err)
	assert.Equal(t, 1, len(retrievedVideos))
	assert.Equal(t, newMockModule("videos_eval_1"), retrievedVideos[evaluation.EvaluatorKey(VideosRoleEvaluator1)].Module)
}

// TestModuleRegistry_MapLookupWithInterfaceKeys tests that map lookups work with interface keys.
func TestModuleRegistry_MapLookupWithInterfaceKeys(t *testing.T) {
	registry := NewModuleRegistry()

	// Register with sayings role type
	sayingsEvaluators := map[SayingsAgentRole]core.Module{
		SayingsRoleEvaluator1: newMockModule("test_module"),
	}
	registry.RegisterEvaluators("test_task", WrapEvaluators(ConvertEvaluatorMap(sayingsEvaluators), nil))

	// Retrieve and lookup using the same concrete type
	retrieved, err := registry.GetEvaluators("test_task")
	require.NoError(t, err)

	// Lookup using concrete type - should work because Go stores concrete types in interface maps
	module, exists := retrieved[evaluation.EvaluatorKey(SayingsRoleEvaluator1)]
	assert.True(t, exists, "module should exist when looked up with concrete type")
	assert.Equal(t, newMockModule("test_module"), module.Module)
}

// TestModuleRegistry_FeedbackAnalyzers tests feedback analyzer registration and retrieval.
func TestModuleRegistry_FeedbackAnalyzers(t *testing.T) {
	registry := NewModuleRegistry()

	// Register sayings feedback analyzers
	sayingsFeedback := map[SayingsFeedbackRole]core.Module{
		SayingsRoleFeedback1: newMockModule("sayings_feedback_1"),
	}
	registry.RegisterFeedbackAnalyzers("sayings_feedback", ConvertExpertMap(sayingsFeedback))

	videosFeedback := map[VideosFeedbackRole]core.Module{
		VideosRoleFeedback1: newMockModule("videos_feedback_1"),
	}
	registry.RegisterFeedbackAnalyzers("videos_feedback", ConvertExpertMap(videosFeedback))

	retrievedSayings, err := registry.GetFeedbackAnalyzers("sayings_feedback")
	require.NoError(t, err)
	assert.Equal(t, 1, len(retrievedSayings))
	assert.Equal(t, newMockModule("sayings_feedback_1"), retrievedSayings[evaluation.ExpertKey(SayingsRoleFeedback1)].Module)

	retrievedVideos, err := registry.GetFeedbackAnalyzers("videos_feedback")
	require.NoError(t, err)
	assert.Equal(t, 1, len(retrievedVideos))
	assert.Equal(t, newMockModule("videos_feedback_1"), retrievedVideos[evaluation.ExpertKey(VideosRoleFeedback1)].Module)
}

// TestModuleRegistry_ErrorHandling tests error cases.
func TestModuleRegistry_ErrorHandling(t *testing.T) {
	registry := NewModuleRegistry()

	t.Run("GetEvaluators returns error for non-existent task", func(t *testing.T) {
		_, err := registry.GetEvaluators("non_existent_task")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "non_existent_task")
	})

	t.Run("GetFeedbackAnalyzers returns error for non-existent task", func(t *testing.T) {
		_, err := registry.GetFeedbackAnalyzers("non_existent_task")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "non_existent_task")
	})

	t.Run("GetEvaluators returns error for nil map", func(t *testing.T) {
		// Register nil map (shouldn't happen in practice, but test edge case)
		registry.RegisterEvaluators("nil_task", nil)
		_, err := registry.GetEvaluators("nil_task")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "nil_task")
	})
}

// TestModuleRegistry_ConcurrentAccess tests thread safety.
func TestModuleRegistry_ConcurrentAccess(t *testing.T) {
	registry := NewModuleRegistry()
	done := make(chan bool)

	// Concurrent writes
	go func() {
		sayingsEvaluators := map[SayingsAgentRole]core.Module{
			SayingsRoleEvaluator1: newMockModule("sayings_1"),
		}
		registry.RegisterEvaluators("sayings_task", WrapEvaluators(ConvertEvaluatorMap(sayingsEvaluators), nil))
		done <- true
	}()

	go func() {
		videosEvaluators := map[VideosAgentRole]core.Module{
			VideosRoleEvaluator1: newMockModule("videos_1"),
		}
		registry.RegisterEvaluators("videos_task", WrapEvaluators(ConvertEvaluatorMap(videosEvaluators), nil))
		done <- true
	}()

	// Wait for both goroutines
	<-done
	<-done

	// Concurrent reads
	go func() {
		_, err := registry.GetEvaluators("sayings_task")
		assert.NoError(t, err)
		done <- true
	}()

	go func() {
		_, err := registry.GetEvaluators("videos_task")
		assert.NoError(t, err)
		done <- true
	}()

	<-done
	<-done
}

// TestModuleRegistry_ModuleModel tests RegisterModuleModel and GetModuleModel (cost-tracking fallback).
func TestModuleRegistry_ModuleModel(t *testing.T) {
	r := NewModuleRegistry()

	// Unregistered returns empty.
	assert.Empty(t, r.GetModuleModel("Style Editor - Feedback Analysis"))

	// Register and retrieve.
	r.RegisterModuleModel("Style Editor - Feedback Analysis", "gemini-2.0-flash")
	assert.Equal(t, "gemini-2.0-flash", r.GetModuleModel("Style Editor - Feedback Analysis"))

	r.RegisterModuleModel("PostGenerator", "claude-3-5-sonnet")
	assert.Equal(t, "claude-3-5-sonnet", r.GetModuleModel("PostGenerator"))

	// Empty name or model does not register.
	r.RegisterModuleModel("", "some-model")
	r.RegisterModuleModel("SomeModule", "")
	assert.Empty(t, r.GetModuleModel(""))
	assert.Empty(t, r.GetModuleModel("SomeModule"))
}

// TestModuleRegistry_TaskIsolation tests that different tasks are isolated.
func TestModuleRegistry_TaskIsolation(t *testing.T) {
	registry := NewModuleRegistry()

	// Register same role type for different tasks
	sayingsTask1Evaluators := map[SayingsAgentRole]core.Module{
		SayingsRoleEvaluator1: newMockModule("task1_module"),
	}
	sayingsTask2Evaluators := map[SayingsAgentRole]core.Module{
		SayingsRoleEvaluator1: newMockModule("task2_module"),
	}

	registry.RegisterEvaluators("task1", WrapEvaluators(ConvertEvaluatorMap(sayingsTask1Evaluators), nil))
	registry.RegisterEvaluators("task2", WrapEvaluators(ConvertEvaluatorMap(sayingsTask2Evaluators), nil))

	// Retrieve and verify isolation
	task1Evaluators, err := registry.GetEvaluators("task1")
	require.NoError(t, err)
	assert.Equal(t, newMockModule("task1_module"), task1Evaluators[evaluation.EvaluatorKey(SayingsRoleEvaluator1)].Module)

	task2Evaluators, err := registry.GetEvaluators("task2")
	require.NoError(t, err)
	assert.Equal(t, newMockModule("task2_module"), task2Evaluators[evaluation.EvaluatorKey(SayingsRoleEvaluator1)].Module)
}

type stubRoleInfo struct {
	names map[evaluation.EvaluatorKey]string
}

func (s stubRoleInfo) EvaluatorName(key evaluation.EvaluatorKey) string { return s.names[key] }
func (s stubRoleInfo) HasEvaluator(key evaluation.EvaluatorKey) bool    { return s.names[key] != "" }
func (s stubRoleInfo) EvaluatorWeight(evaluation.EvaluatorKey) float64  { return 1 }
func (s stubRoleInfo) ConsolidatorKey() evaluation.ConsolidatorKey      { return "cons" }
func (s stubRoleInfo) ConsolidatorName() string                         { return "Consolidator" }

func TestWrapEvaluators_usesRoleInfoLabel(t *testing.T) {
	modules := map[evaluation.EvaluatorKey]core.Module{
		"process_evaluator": newMockModule("process"),
	}
	wrapped := WrapEvaluators(modules, stubRoleInfo{
		names: map[evaluation.EvaluatorKey]string{"process_evaluator": "Process Evaluator"},
	})
	eval := wrapped["process_evaluator"]
	assert.Equal(t, "Process Evaluator", eval.Label)
	start := eval.StartEvent()
	assert.True(t, start.Actor.IsEvaluator())
	assert.False(t, start.Actor.IsConsolidator())
	assert.Equal(t, "Process Evaluator", start.Actor.Label())
}

func TestModuleRegistry_researchConsolidatorIsNotGenerator(t *testing.T) {
	registry := NewModuleRegistry()
	registry.RegisterConsolidator("research_consolidator", "research_consolidator", "research_consolidator", newMockModule("cons"))

	_, err := registry.GetGenerator("research_consolidator")
	assert.Error(t, err)

	cons, err := registry.GetConsolidator("research_consolidator")
	require.NoError(t, err)
	assert.Equal(t, newMockModule("cons"), cons.Module)
	assert.True(t, cons.StartEvent().Actor.IsConsolidator())
}
