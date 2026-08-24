package factory

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	stropdspy "github.com/behaviorengineering/strop/dspy"
	stroplog "github.com/behaviorengineering/strop/log"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
)

// GroundingLLMWrapper wraps a Gemini LLM and adds Google Search grounding support.
// When grounding is enabled, it intercepts Generate() and GenerateWithContent() calls
// and makes direct HTTP requests with the google_search tool.
type GroundingLLMWrapper struct {
	core.LLM
	groundingConfig *stropdspy.GroundingConfig
	apiKey          string
	baseURL         string
	modelID         string
	httpClient      *http.Client
	logger          stroplog.Logger
}

// NewGroundingLLMWrapper creates a new wrapper that adds Google Search grounding to Gemini LLM requests.
func NewGroundingLLMWrapper(
	wrapped core.LLM,
	groundingConfig *stropdspy.GroundingConfig,
	apiKey string,
	baseURL string,
	modelID string,
	timeout time.Duration,
	logger stroplog.Logger,
) *GroundingLLMWrapper {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return &GroundingLLMWrapper{
		LLM:             wrapped,
		groundingConfig: groundingConfig,
		apiKey:          apiKey,
		baseURL:         baseURL,
		modelID:         modelID,
		httpClient: &http.Client{
			Timeout:   timeout,
			Transport: http.DefaultTransport,
		},
		logger: logger,
	}
}

// Generate intercepts Generate calls and adds grounding if enabled.
func (g *GroundingLLMWrapper) Generate(ctx context.Context, prompt string, options ...core.GenerateOption) (*core.LLMResponse, error) {
	if g.groundingConfig == nil {
		// No grounding, delegate to wrapped LLM
		return g.LLM.Generate(ctx, prompt, options...)
	}

	// Grounding enabled - make custom request with tools
	return g.generateWithGrounding(ctx, prompt, options...)
}

// GenerateWithContent intercepts GenerateWithContent calls and adds grounding if enabled.
func (g *GroundingLLMWrapper) GenerateWithContent(ctx context.Context, content []core.ContentBlock, options ...core.GenerateOption) (*core.LLMResponse, error) {
	if g.groundingConfig == nil {
		// No grounding, delegate to wrapped LLM
		return g.LLM.GenerateWithContent(ctx, content, options...)
	}

	// Grounding enabled - make custom request with tools
	return g.generateWithContentAndGrounding(ctx, content, options...)
}

// generateWithGrounding makes a direct HTTP request with Google Search tool for text prompts.
func (g *GroundingLLMWrapper) generateWithGrounding(ctx context.Context, prompt string, options ...core.GenerateOption) (*core.LLMResponse, error) {
	opts := core.NewGenerateOptions()
	for _, opt := range options {
		opt(opts)
	}

	// Build request body with Google Search grounding
	reqBody := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]interface{}{
					{
						"text": prompt,
					},
				},
			},
		},
		// Google Search grounding tool - Gemini 2.5+ models use simplified config
		// Note: dynamic_threshold is not supported in Gemini 2.0+ models
		// The model autonomously decides when to use search grounding
		"tools": []map[string]interface{}{
			{
				"google_search": map[string]interface{}{},
			},
		},
		"generationConfig": map[string]interface{}{
			"temperature":     opts.Temperature,
			"maxOutputTokens": opts.MaxTokens,
			"topP":            opts.TopP,
		},
	}

	return g.makeRequest(ctx, reqBody)
}

// generateWithContentAndGrounding makes a direct HTTP request with Google Search tool for multimodal content.
func (g *GroundingLLMWrapper) generateWithContentAndGrounding(ctx context.Context, content []core.ContentBlock, options ...core.GenerateOption) (*core.LLMResponse, error) {
	opts := core.NewGenerateOptions()
	for _, opt := range options {
		opt(opts)
	}

	// Convert ContentBlocks to Gemini format
	parts := make([]map[string]interface{}, 0, len(content))
	for _, block := range content {
		part := make(map[string]interface{})
		switch block.Type {
		case core.FieldTypeText:
			part["text"] = block.Text
		case core.FieldTypeImage:
			// Gemini expects base64-encoded data
			part["inline_data"] = map[string]interface{}{
				"mime_type": block.MimeType,
				"data":      base64.StdEncoding.EncodeToString(block.Data),
			}
		case core.FieldTypeAudio:
			// Gemini expects base64-encoded data
			part["inline_data"] = map[string]interface{}{
				"mime_type": block.MimeType,
				"data":      base64.StdEncoding.EncodeToString(block.Data),
			}
		default:
			// Fallback to text
			part["text"] = fmt.Sprintf("[Unsupported content type: %s]", block.Type)
		}
		parts = append(parts, part)
	}

	// Build request body with Google Search grounding
	reqBody := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": parts,
			},
		},
		// Google Search grounding tool - Gemini 2.5+ models use simplified config
		// Note: dynamic_threshold is not supported in Gemini 2.0+ models
		// The model autonomously decides when to use search grounding
		"tools": []map[string]interface{}{
			{
				"google_search": map[string]interface{}{},
			},
		},
		"generationConfig": map[string]interface{}{
			"temperature":     opts.Temperature,
			"maxOutputTokens": opts.MaxTokens,
			"topP":            opts.TopP,
		},
	}

	return g.makeRequest(ctx, reqBody)
}

// geminiResponsePart mirrors the subset of Gemini generateContent candidate.parts we need.
// Models may return multiple parts (e.g. thought vs final answer); using only parts[0].text drops the real output.
type geminiResponsePart struct {
	Text    string `json:"text"`
	Thought *bool  `json:"thought"`
}

// extractGeminiCandidateText joins text from all parts in document order.
// Parts marked thought=true (internal reasoning) are excluded from the primary string; if that yields nothing,
// all non-empty text parts are concatenated so we never return empty when the API sent text in a later part.
func extractGeminiCandidateText(parts []geminiResponsePart) string {
	if len(parts) == 0 {
		return ""
	}
	var primary, fallback strings.Builder
	for _, p := range parts {
		t := strings.TrimSpace(p.Text)
		if t == "" {
			continue
		}
		if fallback.Len() > 0 {
			fallback.WriteString("\n")
		}
		fallback.WriteString(p.Text)
		isThought := p.Thought != nil && *p.Thought
		if isThought {
			continue
		}
		if primary.Len() > 0 {
			primary.WriteString("\n")
		}
		primary.WriteString(p.Text)
	}
	if primary.Len() > 0 {
		return primary.String()
	}
	return fallback.String()
}

// makeRequest makes the actual HTTP request to Gemini API with grounding.
func (g *GroundingLLMWrapper) makeRequest(ctx context.Context, reqBody map[string]interface{}) (*core.LLMResponse, error) {
	// Construct URL
	url := fmt.Sprintf("%s/v1beta/models/%s:generateContent?key=%s", g.baseURL, g.modelID, g.apiKey)

	// Marshal request body
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	if g.logger != nil {
		g.logger.WithFields(map[string]interface{}{
			"model":    g.modelID,
			"url":      url,
			"has_tool": true,
		}).Debug("Making Gemini API request with Google Search grounding")
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Execute request
	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			g.logger.WithError(closeErr).Warn("Failed to close response body")
		}
	}()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		if g.logger != nil {
			g.logger.WithFields(map[string]interface{}{
				"status_code": resp.StatusCode,
				"body":        string(body),
			}).Error("Gemini API request failed")
		}
		return nil, fmt.Errorf("API request failed with status code %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var geminiResp struct {
		Candidates []struct {
			Content struct {
				Parts []geminiResponsePart `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		UsageMetadata struct {
			PromptTokenCount     int `json:"promptTokenCount"`
			CandidatesTokenCount int `json:"candidatesTokenCount"`
			TotalTokenCount      int `json:"totalTokenCount"`
		} `json:"usageMetadata"`
	}

	if err := json.Unmarshal(body, &geminiResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if len(geminiResp.Candidates) == 0 {
		return nil, fmt.Errorf("no candidates in response")
	}

	parts := geminiResp.Candidates[0].Content.Parts
	if len(parts) == 0 {
		return nil, fmt.Errorf("no parts in response candidate")
	}

	content := strings.TrimSpace(extractGeminiCandidateText(parts))
	if content == "" {
		return nil, fmt.Errorf("empty text in all response parts (candidate had %d part(s))", len(parts))
	}
	usage := &core.TokenInfo{
		PromptTokens:     geminiResp.UsageMetadata.PromptTokenCount,
		CompletionTokens: geminiResp.UsageMetadata.CandidatesTokenCount,
		TotalTokens:      geminiResp.UsageMetadata.TotalTokenCount,
	}

	return &core.LLMResponse{
		Content: content,
		Usage:   usage,
	}, nil
}

// All other methods delegate to the wrapped LLM
// (GenerateWithJSON, GenerateWithFunctions, CreateEmbedding, etc. don't need grounding)
