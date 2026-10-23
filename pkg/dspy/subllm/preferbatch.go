package subllm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/behaviorengineering/strop/pkg/openaibatch"

	dspyrlm "github.com/XiaoConstantine/dspy-go/pkg/modules/rlm"
)

const (
	defaultProbeTimeout     = 5 * time.Second
	defaultMinBatchDeadline = 30 * time.Second
	batchListPath           = "/v1/batches"
)

// batchRunner is the batch HTTP surface PreferBatch uses (usually openaibatch.Client).
type batchRunner interface {
	RunChatBatch(ctx context.Context, lines []openaibatch.ChatLine) (map[string]openaibatch.LineResult, error)
}

// PreferBatch routes QueryBatched to OpenAI Batch when available and falls back to sync.
type PreferBatch struct {
	sync     dspyrlm.SubLLMClient
	batch    batchRunner
	oaClient *openaibatch.Client
	model    string
	probe    func(ctx context.Context) (available bool, throttle bool, err error)

	mu          sync.Mutex
	probeDone   bool
	unavailable bool

	minBatchDeadline time.Duration
}

// PreferBatchOption configures PreferBatch.
type PreferBatchOption func(*PreferBatch)

// WithMinBatchDeadline sets the minimum remaining context deadline required to use batch.
func WithMinBatchDeadline(d time.Duration) PreferBatchOption {
	return func(p *PreferBatch) {
		if d > 0 {
			p.minBatchDeadline = d
		}
	}
}

// WithBatchProbe overrides the GET /v1/batches availability probe (tests).
func WithBatchProbe(fn func(context.Context) (available bool, throttle bool, err error)) PreferBatchOption {
	return func(p *PreferBatch) {
		p.probe = fn
	}
}

// WithBatchRunner overrides the batch HTTP client (tests).
func WithBatchRunner(r batchRunner) PreferBatchOption {
	return func(p *PreferBatch) {
		if r != nil {
			p.batch = r
		}
	}
}

// NewPreferBatch wraps sync with OpenAI Batch for multi-prompt QueryBatched calls.
func NewPreferBatch(sync dspyrlm.SubLLMClient, client openaibatch.Client, model string, opts ...PreferBatchOption) dspyrlm.SubLLMClient {
	if sync == nil {
		return nil
	}
	model = strings.TrimSpace(model)
	p := &PreferBatch{
		sync:             sync,
		batch:            &client,
		oaClient:         &client,
		model:            model,
		minBatchDeadline: defaultMinBatchDeadline,
	}
	if strings.TrimSpace(client.BaseURL) != "" {
		p.probe = defaultBatchProbe(&client)
	}
	for _, opt := range opts {
		if opt != nil {
			opt(p)
		}
	}
	return p
}

// Query delegates to the sync sub-client.
func (p *PreferBatch) Query(ctx context.Context, prompt string) (dspyrlm.QueryResponse, error) {
	return p.sync.Query(ctx, prompt)
}

// QueryBatched prefers batch for len>1 when the gateway supports it.
func (p *PreferBatch) QueryBatched(ctx context.Context, prompts []string) ([]dspyrlm.QueryResponse, error) {
	if p == nil || p.sync == nil {
		return nil, fmt.Errorf("subllm: prefer batch: client is nil")
	}
	if len(prompts) == 0 {
		return nil, nil
	}
	if len(prompts) == 1 || p.shouldUseSync(ctx) || p.model == "" || p.batch == nil {
		return p.sync.QueryBatched(ctx, prompts)
	}
	available, throttle, err := p.ensureAvailable(ctx)
	if err != nil && !throttle {
		return nil, err
	}
	if throttle || !available {
		return p.sync.QueryBatched(ctx, prompts)
	}

	lines, ids := chatLinesForPrompts(p.model, prompts)
	results, err := p.batch.RunChatBatch(ctx, lines)
	if err != nil {
		if p.allowSyncFallback(err) {
			return p.sync.QueryBatched(ctx, prompts)
		}
		return nil, err
	}
	return mapBatchResults(ids, results), nil
}

// RunChatLines prefers OpenAI Batch for len>1 when the gateway supports it.
// Sync fallback posts each line sequentially and preserves custom_id.
func (p *PreferBatch) RunChatLines(ctx context.Context, lines []openaibatch.ChatLine) (map[string]openaibatch.LineResult, error) {
	if p == nil || p.sync == nil {
		return nil, fmt.Errorf("subllm: prefer batch: client is nil")
	}
	if len(lines) == 0 {
		return nil, nil
	}
	if len(lines) == 1 || p.shouldUseSync(ctx) || p.batch == nil {
		return p.syncChatLines(ctx, lines)
	}
	available, throttle, err := p.ensureAvailable(ctx)
	if err != nil && !throttle {
		return nil, err
	}
	if throttle || !available {
		return p.syncChatLines(ctx, lines)
	}
	normalized := normalizeChatLines(p.model, lines)
	results, err := p.batch.RunChatBatch(ctx, normalized)
	if err != nil {
		if p.allowSyncFallback(err) {
			return p.syncChatLines(ctx, normalized)
		}
		return nil, err
	}
	return results, nil
}

func normalizeChatLines(model string, lines []openaibatch.ChatLine) []openaibatch.ChatLine {
	out := make([]openaibatch.ChatLine, len(lines))
	for i, ln := range lines {
		out[i] = ln
		if strings.TrimSpace(out[i].Model) == "" {
			out[i].Model = model
		}
	}
	return out
}

func (p *PreferBatch) syncChatLines(ctx context.Context, lines []openaibatch.ChatLine) (map[string]openaibatch.LineResult, error) {
	if p.oaClient == nil {
		return nil, fmt.Errorf("subllm: prefer batch: openai client is nil")
	}
	out := make(map[string]openaibatch.LineResult, len(lines))
	for _, ln := range lines {
		id := strings.TrimSpace(ln.CustomID)
		if id == "" {
			return nil, fmt.Errorf("subllm: prefer batch: custom_id is required")
		}
		content, err := p.postChatCompletion(ctx, ln)
		if err != nil {
			out[id] = openaibatch.LineResult{ErrMessage: err.Error()}
			continue
		}
		out[id] = openaibatch.LineResult{Content: content}
	}
	return out, nil
}

func (p *PreferBatch) postChatCompletion(ctx context.Context, ln openaibatch.ChatLine) (string, error) {
	base, err := openaibatch.GatewayRoot(p.oaClient.BaseURL)
	if err != nil {
		return "", err
	}
	model := strings.TrimSpace(ln.Model)
	if model == "" {
		model = strings.TrimSpace(p.model)
	}
	if model == "" {
		return "", fmt.Errorf("subllm: prefer batch: model is required")
	}
	body := map[string]any{
		"model":    model,
		"messages": ln.Messages,
	}
	if ln.MaxTokens > 0 {
		body["max_tokens"] = ln.MaxTokens
	}
	rawBody, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/chat/completions", bytes.NewReader(rawBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(p.oaClient.APIKey) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(p.oaClient.APIKey))
	}
	for k, v := range p.oaClient.Headers {
		if strings.TrimSpace(k) != "" {
			req.Header.Set(k, v)
		}
	}
	httpClient := p.oaClient.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("subllm: chat completion: http %d: %s", resp.StatusCode, string(raw))
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", err
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("subllm: chat completion: empty choices")
	}
	return parsed.Choices[0].Message.Content, nil
}

func (p *PreferBatch) shouldUseSync(ctx context.Context) bool {
	if ctx == nil {
		return true
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		return false
	}
	return time.Until(deadline) < p.minBatchDeadline
}

func (p *PreferBatch) ensureAvailable(ctx context.Context) (available bool, throttle bool, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.probeDone {
		return !p.unavailable, false, nil
	}
	p.probeDone = true
	if p.probe == nil {
		p.unavailable = true
		return false, false, nil
	}
	available, throttle, err = p.probe(ctx)
	if throttle {
		p.probeDone = false
		return false, true, nil
	}
	if err != nil || !available {
		p.unavailable = true
		return false, false, err
	}
	return true, false, nil
}

func (p *PreferBatch) allowSyncFallback(err error) bool {
	if openaibatch.PostSubmitBatchError(err) {
		return false
	}
	if openaibatch.IsNotFound(err) || openaibatch.IsUnauthorized(err) || openaibatch.IsFailedPrecondition(err) {
		p.mu.Lock()
		p.unavailable = true
		p.probeDone = true
		p.mu.Unlock()
		return true
	}
	if openaibatch.IsThrottle(err) || openaibatch.IsOverCap(err) {
		return true
	}
	msg := err.Error()
	if strings.Contains(msg, "openaibatch: upload file") || strings.Contains(msg, "openaibatch: create batch") {
		return true
	}
	return false
}

func chatLinesForPrompts(model string, prompts []string) ([]openaibatch.ChatLine, []string) {
	lines := make([]openaibatch.ChatLine, len(prompts))
	ids := make([]string, len(prompts))
	for i, prompt := range prompts {
		ids[i] = fmt.Sprintf("rlm-%d", i)
		lines[i] = openaibatch.ChatLine{
			CustomID: ids[i],
			Model:    model,
			Messages: []map[string]string{{"role": "user", "content": prompt}},
		}
	}
	return lines, ids
}

func mapBatchResults(ids []string, results map[string]openaibatch.LineResult) []dspyrlm.QueryResponse {
	out := make([]dspyrlm.QueryResponse, len(ids))
	for i, id := range ids {
		lr, ok := results[id]
		if !ok {
			out[i] = dspyrlm.QueryResponse{Response: "Error: missing batch result"}
			continue
		}
		if strings.TrimSpace(lr.ErrMessage) != "" {
			out[i] = dspyrlm.QueryResponse{Response: "Error: " + lr.ErrMessage}
			continue
		}
		out[i] = dspyrlm.QueryResponse{
			Response:         lr.Content,
			PromptTokens:     lr.PromptTokens,
			CompletionTokens: lr.CompletionTokens,
			TotalTokens:      lr.TotalTokens,
		}
	}
	return out
}

func defaultBatchProbe(client *openaibatch.Client) func(context.Context) (bool, bool, error) {
	return func(ctx context.Context) (bool, bool, error) {
		if client == nil {
			return false, false, nil
		}
		root, err := openaibatch.GatewayRoot(client.BaseURL)
		if err != nil {
			return false, false, err
		}
		probeCtx, cancel := context.WithTimeout(ctx, defaultProbeTimeout)
		defer cancel()
		req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, root+batchListPath, nil)
		if err != nil {
			return false, false, err
		}
		if strings.TrimSpace(client.APIKey) != "" {
			req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(client.APIKey))
		}
		for k, v := range client.Headers {
			if strings.TrimSpace(k) != "" {
				req.Header.Set(k, v)
			}
		}
		httpClient := client.HTTPClient
		if httpClient == nil {
			httpClient = http.DefaultClient
		}
		resp, err := httpClient.Do(req)
		if err != nil {
			return false, false, err
		}
		defer func() { _ = resp.Body.Close() }()
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		switch resp.StatusCode {
		case http.StatusOK:
			return true, false, nil
		case http.StatusNotFound, http.StatusUnauthorized:
			return false, false, nil
		case http.StatusTooManyRequests, http.StatusServiceUnavailable:
			return false, true, nil
		default:
			if resp.StatusCode >= 500 {
				return false, true, nil
			}
			return false, false, fmt.Errorf("subllm: batch probe: http %d: %s", resp.StatusCode, string(raw))
		}
	}
}
