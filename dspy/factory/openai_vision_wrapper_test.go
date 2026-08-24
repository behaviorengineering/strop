package factory

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
)

// stubLLM satisfies core.LLM for wrapper construction in tests.
type stubLLM struct {
	*core.BaseLLM
}

func (s *stubLLM) CreateEmbedding(context.Context, string, ...core.EmbeddingOption) (*core.EmbeddingResult, error) {
	return nil, nil
}

func (s *stubLLM) CreateEmbeddings(context.Context, []string, ...core.EmbeddingOption) (*core.BatchEmbeddingResult, error) {
	return nil, nil
}

func (s *stubLLM) Generate(context.Context, string, ...core.GenerateOption) (*core.LLMResponse, error) {
	return nil, nil
}

func (s *stubLLM) GenerateWithJSON(context.Context, string, ...core.GenerateOption) (map[string]any, error) {
	return nil, nil
}

func (s *stubLLM) GenerateWithFunctions(context.Context, string, []map[string]any, ...core.GenerateOption) (map[string]any, error) {
	return nil, nil
}

func (s *stubLLM) GenerateWithTools(context.Context, []core.ChatMessage, []map[string]any, ...core.GenerateOption) (map[string]any, error) {
	return nil, nil
}

func (s *stubLLM) StreamGenerate(context.Context, string, ...core.GenerateOption) (*core.StreamResponse, error) {
	return nil, nil
}

func (s *stubLLM) ProviderName() string { return "stub" }
func (s *stubLLM) ModelID() string      { return "stub-model" }
func (s *stubLLM) Capabilities() []core.Capability {
	return nil
}

func TestContentBlocksToOpenAIParts_textAndImage(t *testing.T) {
	imageData := []byte{0x89, 0x50, 0x4e, 0x47}
	parts, err := contentBlocksToOpenAIParts([]core.ContentBlock{
		core.NewTextBlock("describe this"),
		core.NewImageBlock(imageData, "image/png"),
	})
	require.NoError(t, err)
	require.Len(t, parts, 2)

	assert.Equal(t, "text", parts[0]["type"])
	assert.Equal(t, "describe this", parts[0]["text"])

	assert.Equal(t, "image_url", parts[1]["type"])
	imageURL, ok := parts[1]["image_url"].(map[string]interface{})
	require.True(t, ok)
	url, ok := imageURL["url"].(string)
	require.True(t, ok)
	assert.True(t, strings.HasPrefix(url, "data:image/png;base64,"))
	encoded := strings.TrimPrefix(url, "data:image/png;base64,")
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	require.NoError(t, err)
	assert.Equal(t, imageData, decoded)
}

func TestContentBlocksToOpenAIParts_rejectsEmptyImage(t *testing.T) {
	_, err := contentBlocksToOpenAIParts([]core.ContentBlock{
		core.NewImageBlock(nil, "image/png"),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty data")
}

func TestContentBlocksToOpenAIParts_rejectsAudio(t *testing.T) {
	_, err := contentBlocksToOpenAIParts([]core.ContentBlock{
		core.NewAudioBlock([]byte("x"), "audio/wav"),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "audio")
}

func TestExtractOpenAIVisionMessageContent_string(t *testing.T) {
	got, err := extractOpenAIVisionMessageContent(json.RawMessage(`"hello world"`))
	require.NoError(t, err)
	assert.Equal(t, "hello world", got)
}

func TestExtractOpenAIVisionMessageContent_parts(t *testing.T) {
	raw := json.RawMessage(`[{"type":"text","text":"a"},{"type":"text","text":"b"}]`)
	got, err := extractOpenAIVisionMessageContent(raw)
	require.NoError(t, err)
	assert.Equal(t, "a\nb", got)
}

func TestOpenAIVisionLLMWrapper_StreamGenerateWithContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		assert.Contains(t, string(body), `"stream":true`)

		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		require.True(t, ok)
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"<response>\"},\"finish_reason\":null}]}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":null}]}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"</response>\"},\"finish_reason\":\"stop\"}]}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	wrapper := NewOpenAIVisionLLMWrapper(
		&stubLLM{},
		"test-key",
		server.URL+"/v1",
		"test-model",
		5*time.Second,
		nil,
	)

	stream, err := wrapper.StreamGenerateWithContent(context.Background(), []core.ContentBlock{
		core.NewTextBlock("what do you see?"),
		core.NewImageBlock([]byte("img"), "image/jpeg"),
	})
	require.NoError(t, err)

	var parts []string
	for chunk := range stream.ChunkChannel {
		require.NoError(t, chunk.Error)
		if chunk.Content != "" {
			parts = append(parts, chunk.Content)
		}
		if chunk.Done {
			break
		}
	}
	assert.Equal(t, []string{"<response>", "ok", "</response>"}, parts)
}

func TestOpenAIVisionLLMWrapper_GenerateWithContent(t *testing.T) {
	var gotAuth string
	var gotBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		gotAuth = r.Header.Get("Authorization")
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(body, &gotBody))

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-1",
			"model":"test-model",
			"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"<response>ok</response>"}}],
			"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}
		}`))
	}))
	defer server.Close()

	wrapper := NewOpenAIVisionLLMWrapper(
		&stubLLM{},
		"test-key",
		server.URL+"/v1",
		"test-model",
		5*time.Second,
		nil,
	)

	resp, err := wrapper.GenerateWithContent(context.Background(), []core.ContentBlock{
		core.NewTextBlock("what do you see?"),
		core.NewImageBlock([]byte("img"), "image/jpeg"),
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "<response>ok</response>", resp.Content)
	assert.Equal(t, "Bearer test-key", gotAuth)
	assert.Equal(t, "test-model", gotBody["model"])

	messages, ok := gotBody["messages"].([]interface{})
	require.True(t, ok)
	require.Len(t, messages, 1)
	msg, ok := messages[0].(map[string]interface{})
	require.True(t, ok)
	content, ok := msg["content"].([]interface{})
	require.True(t, ok)
	require.Len(t, content, 2)
	assert.Equal(t, "text", content[0].(map[string]interface{})["type"])
	assert.Equal(t, "image_url", content[1].(map[string]interface{})["type"])
}
