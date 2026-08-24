package factory

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	kitlog "github.com/behaviorengineering/strop/log"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
)

// OpenAIVisionLLMWrapper adds OpenAI-compatible multimodal GenerateWithContent
// support. dspy-go's OpenAILLM embeds BaseLLM and rejects multimodal content;
// this wrapper posts chat.completions messages with image_url data URLs instead.
// Remove once upstream OpenAILLM implements GenerateWithContent.
type OpenAIVisionLLMWrapper struct {
	core.LLM
	apiKey     string
	baseURL    string
	modelID    string
	httpClient *http.Client
	logger     kitlog.Logger
}

// NewOpenAIVisionLLMWrapper creates a wrapper that implements multimodal chat
// completions against an OpenAI-compatible base URL (e.g. Polypus /v1).
func NewOpenAIVisionLLMWrapper(
	wrapped core.LLM,
	apiKey string,
	baseURL string,
	modelID string,
	timeout time.Duration,
	logger kitlog.Logger,
) *OpenAIVisionLLMWrapper {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return &OpenAIVisionLLMWrapper{
		LLM:     wrapped,
		apiKey:  apiKey,
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		modelID: modelID,
		httpClient: &http.Client{
			Timeout:   timeout,
			Transport: http.DefaultTransport,
		},
		logger: logger,
	}
}

// GenerateWithContent sends multimodal content via OpenAI chat.completions.
func (w *OpenAIVisionLLMWrapper) GenerateWithContent(
	ctx context.Context,
	content []core.ContentBlock,
	options ...core.GenerateOption,
) (*core.LLMResponse, error) {
	parts, err := contentBlocksToOpenAIParts(content)
	if err != nil {
		return nil, err
	}
	if len(parts) == 0 {
		return nil, fmt.Errorf("no content provided for OpenAI vision request")
	}

	opts := core.NewGenerateOptions()
	for _, opt := range options {
		opt(opts)
	}

	reqBody := map[string]interface{}{
		"model": w.modelID,
		"messages": []map[string]interface{}{
			{
				"role":    "user",
				"content": parts,
			},
		},
	}
	if opts.Temperature > 0 {
		reqBody["temperature"] = opts.Temperature
	}
	if opts.MaxTokens > 0 {
		reqBody["max_tokens"] = opts.MaxTokens
	}
	if opts.TopP > 0 {
		reqBody["top_p"] = opts.TopP
	}

	return w.makeRequest(ctx, reqBody)
}

// StreamGenerateWithContent streams multimodal chat.completions (SSE).
// Falls back to a single full-response chunk when the provider rejects stream=true.
func (w *OpenAIVisionLLMWrapper) StreamGenerateWithContent(
	ctx context.Context,
	content []core.ContentBlock,
	options ...core.GenerateOption,
) (*core.StreamResponse, error) {
	parts, err := contentBlocksToOpenAIParts(content)
	if err != nil {
		return nil, err
	}
	if len(parts) == 0 {
		return nil, fmt.Errorf("no content provided for OpenAI vision request")
	}

	opts := core.NewGenerateOptions()
	for _, opt := range options {
		opt(opts)
	}

	reqBody := map[string]interface{}{
		"model": w.modelID,
		"messages": []map[string]interface{}{
			{
				"role":    "user",
				"content": parts,
			},
		},
		"stream": true,
	}
	if opts.Temperature > 0 {
		reqBody["temperature"] = opts.Temperature
	}
	if opts.MaxTokens > 0 {
		reqBody["max_tokens"] = opts.MaxTokens
	}
	if opts.TopP > 0 {
		reqBody["top_p"] = opts.TopP
	}

	streamCtx, cancel := context.WithCancel(ctx)
	chunks := make(chan core.StreamChunk, 16)
	go func() {
		defer close(chunks)
		defer cancel()

		gotContent, err := w.streamChatCompletions(streamCtx, reqBody, chunks)
		if err == nil {
			return
		}
		if gotContent {
			chunks <- core.StreamChunk{Done: true, Error: err}
			return
		}
		// Some OpenAI-compatible vision backends reject stream=true; fall back.
		if w.logger != nil {
			w.logger.WithError(err).Debug("OpenAI vision SSE failed; falling back to non-streaming")
		}
		response, genErr := w.GenerateWithContent(streamCtx, content, options...)
		if genErr != nil {
			chunks <- core.StreamChunk{Done: true, Error: genErr}
			return
		}
		chunks <- core.StreamChunk{Content: response.Content, Done: true, Usage: response.Usage}
	}()
	return &core.StreamResponse{ChunkChannel: chunks, Cancel: cancel}, nil
}

func (w *OpenAIVisionLLMWrapper) streamChatCompletions(
	ctx context.Context,
	reqBody map[string]interface{},
	chunks chan<- core.StreamChunk,
) (gotContent bool, err error) {
	url := w.chatCompletionsURL()
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return false, fmt.Errorf("failed to marshal OpenAI vision stream request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return false, fmt.Errorf("failed to create OpenAI vision stream request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if strings.TrimSpace(w.apiKey) != "" {
		req.Header.Set("Authorization", "Bearer "+w.apiKey)
	}

	// Avoid the non-stream client's overall Timeout killing long SSE reads.
	client := &http.Client{Transport: w.httpClient.Transport}
	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Errorf("OpenAI vision stream request failed: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && w.logger != nil {
			w.logger.WithError(closeErr).Warn("Failed to close OpenAI vision stream body")
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("OpenAI vision stream API failed with status %d: %s", resp.StatusCode, string(body))
	}

	scanner := bufio.NewScanner(resp.Body)
	// Vision XML can be large; raise the default token size.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return gotContent, ctx.Err()
		default:
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			chunks <- core.StreamChunk{Done: true}
			return gotContent, nil
		}

		var delta openAIVisionStreamChunk
		if err := json.Unmarshal([]byte(data), &delta); err != nil {
			continue
		}
		if len(delta.Choices) == 0 {
			continue
		}
		choice := delta.Choices[0]
		if text := choice.Delta.Content; text != "" {
			gotContent = true
			chunks <- core.StreamChunk{Content: text}
		}
		if choice.FinishReason != nil && *choice.FinishReason != "" {
			chunks <- core.StreamChunk{Done: true}
			return gotContent, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return gotContent, fmt.Errorf("error reading OpenAI vision stream: %w", err)
	}
	if !gotContent {
		return false, fmt.Errorf("OpenAI vision stream produced no content")
	}
	chunks <- core.StreamChunk{Done: true}
	return true, nil
}

type openAIVisionStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
}

func contentBlocksToOpenAIParts(content []core.ContentBlock) ([]map[string]interface{}, error) {
	parts := make([]map[string]interface{}, 0, len(content))
	for _, block := range content {
		switch block.Type {
		case core.FieldTypeText:
			text := block.Text
			if strings.TrimSpace(text) == "" {
				continue
			}
			parts = append(parts, map[string]interface{}{
				"type": "text",
				"text": text,
			})
		case core.FieldTypeImage:
			if len(block.Data) == 0 {
				return nil, fmt.Errorf("image content block has empty data")
			}
			mimeType := strings.TrimSpace(block.MimeType)
			if mimeType == "" {
				mimeType = "image/png"
			}
			dataURL := fmt.Sprintf(
				"data:%s;base64,%s",
				mimeType,
				base64.StdEncoding.EncodeToString(block.Data),
			)
			parts = append(parts, map[string]interface{}{
				"type": "image_url",
				"image_url": map[string]interface{}{
					"url": dataURL,
				},
			})
		case core.FieldTypeAudio:
			return nil, fmt.Errorf("audio content blocks are not supported by OpenAIVisionLLMWrapper")
		default:
			return nil, fmt.Errorf("unsupported content block type %q", block.Type)
		}
	}
	return parts, nil
}

func (w *OpenAIVisionLLMWrapper) chatCompletionsURL() string {
	return w.baseURL + "/chat/completions"
}

func (w *OpenAIVisionLLMWrapper) makeRequest(ctx context.Context, reqBody map[string]interface{}) (*core.LLMResponse, error) {
	url := w.chatCompletionsURL()

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal OpenAI vision request: %w", err)
	}

	if w.logger != nil {
		w.logger.WithFields(map[string]interface{}{
			"model":      w.modelID,
			"url":        url,
			"body_bytes": len(jsonData),
		}).Debug("Making OpenAI-compatible multimodal chat.completions request")
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenAI vision request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(w.apiKey) != "" {
		req.Header.Set("Authorization", "Bearer "+w.apiKey)
	}

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("OpenAI vision request failed: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && w.logger != nil {
			w.logger.WithError(closeErr).Warn("Failed to close OpenAI vision response body")
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read OpenAI vision response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		if w.logger != nil {
			w.logger.WithFields(map[string]interface{}{
				"status_code": resp.StatusCode,
				"body":        string(body),
			}).Error("OpenAI vision request failed")
		}
		return nil, fmt.Errorf("OpenAI vision API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var chatResp openAIVisionChatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal OpenAI vision response: %w", err)
	}
	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("OpenAI vision response contained no choices")
	}

	content, err := extractOpenAIVisionMessageContent(chatResp.Choices[0].Message.Content)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(content) == "" {
		return nil, fmt.Errorf("OpenAI vision response content was empty")
	}

	usage := &core.TokenInfo{
		PromptTokens:     chatResp.Usage.PromptTokens,
		CompletionTokens: chatResp.Usage.CompletionTokens,
		TotalTokens:      chatResp.Usage.TotalTokens,
	}

	return &core.LLMResponse{
		Content: content,
		Usage:   usage,
		Metadata: map[string]any{
			"finish_reason": chatResp.Choices[0].FinishReason,
			"id":            chatResp.ID,
			"model":         chatResp.Model,
		},
	}, nil
}

type openAIVisionChatResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		FinishReason string `json:"finish_reason"`
		Message      struct {
			Content json.RawMessage `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// extractOpenAIVisionMessageContent accepts string content or an array of
// text parts (some OpenAI-compatible proxies return either).
func extractOpenAIVisionMessageContent(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", fmt.Errorf("OpenAI vision message content is null")
	}

	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return asString, nil
	}

	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return "", fmt.Errorf("unsupported OpenAI vision message content shape: %w", err)
	}

	var b strings.Builder
	for _, part := range parts {
		if part.Type != "" && part.Type != "text" {
			continue
		}
		if strings.TrimSpace(part.Text) == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString(part.Text)
	}
	return b.String(), nil
}
