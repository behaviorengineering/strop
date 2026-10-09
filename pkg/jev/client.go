package jev

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Client calls OpenAI-compatible POST /v1/systemone (TypeSafe JEV).
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
	Path       string
}

// NewClient builds a SystemOne client. Path defaults to /v1/systemone.
func NewClient(baseURL string, httpClient *http.Client, path string) *Client {
	if strings.TrimSpace(path) == "" {
		path = "/v1/systemone"
	}
	return &Client{
		BaseURL:    strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		HTTPClient: httpClient,
		Path:       path,
	}
}

type evalBody struct {
	Model     string                      `json:"model"`
	State     string                      `json:"state"`
	Questions map[string]evalBodyQuestion `json:"questions"`
}

type evalBodyQuestion struct {
	Type         string            `json:"type"`
	Instructions string            `json:"instructions,omitempty"`
	Criteria     map[string]string `json:"criteria,omitempty"`
}

type evalResponse struct {
	Answers map[string]evalAnswer `json:"answers"`
}

type evalAnswer struct {
	Type   string          `json:"type"`
	Noul   float64         `json:"noul,omitempty"`
	Choice json.RawMessage `json:"choice,omitempty"`
}

// Eval posts one SystemOne request and returns answers keyed by question id.
func (c *Client) Eval(ctx context.Context, req Request) (map[string]Answer, error) {
	const op = "jev: eval"
	if c == nil {
		return nil, fmt.Errorf("%s: client is nil", op)
	}
	if ctx == nil {
		return nil, fmt.Errorf("%s: context is nil", op)
	}
	if _, ok := ctx.Deadline(); !ok {
		return nil, fmt.Errorf("%s: context must have a deadline", op)
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		return nil, fmt.Errorf("%s: model is empty", op)
	}
	if strings.TrimSpace(req.State) == "" {
		return nil, fmt.Errorf("%s: state is empty", op)
	}
	if len(req.Questions) == 0 {
		return nil, fmt.Errorf("%s: questions required", op)
	}
	bodyQuestions := make(map[string]evalBodyQuestion, len(req.Questions))
	for k, q := range req.Questions {
		bodyQuestions[k] = evalBodyQuestion{
			Type:         strings.TrimSpace(q.Type),
			Instructions: strings.TrimSpace(q.Instructions),
			Criteria:     q.Criteria,
		}
	}
	rawBody, err := json.Marshal(evalBody{
		Model:     model,
		State:     req.State,
		Questions: bodyQuestions,
	})
	if err != nil {
		return nil, fmt.Errorf("%s: encode request: %w", op, err)
	}
	url := c.BaseURL + c.Path
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(rawBody))
	if err != nil {
		return nil, fmt.Errorf("%s: build request: %w", op, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%s: request failed: %w", op, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("%s: read response: %w", op, err)
	}
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, fmt.Errorf("%s: unavailable (status %s)", op, resp.Status)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s: rejected (status %s): %s", op, resp.Status, truncate(string(raw), 200))
	}
	var parsed evalResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("%s: decode response: %w", op, err)
	}
	if len(parsed.Answers) == 0 {
		return nil, fmt.Errorf("%s: empty answers", op)
	}
	out := make(map[string]Answer, len(parsed.Answers))
	for k, a := range parsed.Answers {
		out[k] = Answer{
			Type:   a.Type,
			Noul:   a.Noul,
			Choice: append([]byte(nil), a.Choice...),
		}
	}
	return out, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
