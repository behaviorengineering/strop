package openaibatch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

const (
	completionWindow    = "24h"
	defaultPollInterval = 2 * time.Second
)

// Client calls OpenAI-compatible /v1/files and /v1/batches (not chat/completions sync).
type Client struct {
	BaseURL       string
	APIKey        string
	Headers       map[string]string
	HTTPClient    *http.Client
	PollEvery     time.Duration
	MaxJSONLBytes int
}

// RunChatBatch uploads lines, polls until terminal, and returns per-custom_id results.
func (c *Client) RunChatBatch(ctx context.Context, lines []ChatLine) (map[string]LineResult, error) {
	if c == nil {
		return nil, fmt.Errorf("openaibatch: client is nil")
	}
	if ctx == nil {
		return nil, fmt.Errorf("openaibatch: context is required")
	}
	base, err := GatewayRoot(c.BaseURL)
	if err != nil {
		return nil, err
	}
	if err := validateChatLines(lines); err != nil {
		return nil, err
	}

	maxBytes := c.maxJSONLBytes()
	jsonl, err := encodeChatJSONL(lines)
	if err != nil {
		return nil, err
	}
	if len(jsonl) > maxBytes {
		chunks, err := SplitUnderCap(lines, maxBytes)
		if err != nil {
			return nil, err
		}
		merged := make(map[string]LineResult)
		for _, chunk := range chunks {
			size, err := EncodedJSONLSize(chunk)
			if err != nil {
				return nil, err
			}
			if len(chunk) == 1 && size > maxBytes {
				return nil, &OverCapError{
					CustomID: chunk[0].CustomID,
					Bytes:    size,
					MaxBytes: maxBytes,
				}
			}
			part, err := c.RunChatBatch(ctx, chunk)
			if err != nil {
				return nil, err
			}
			for k, v := range part {
				merged[k] = v
			}
		}
		return merged, nil
	}

	fileID, err := c.uploadJSONL(ctx, base, jsonl)
	if err != nil {
		return nil, err
	}
	batchID, err := c.createBatch(ctx, base, fileID)
	if err != nil {
		return nil, err
	}
	meta, err := c.pollTerminal(ctx, base, batchID)
	if err != nil {
		return nil, err
	}
	if meta.Status != "completed" {
		return nil, fmt.Errorf("openaibatch: batch status %s", meta.Status)
	}
	if strings.TrimSpace(meta.OutputFileID) == "" {
		return nil, fmt.Errorf("openaibatch: completed batch missing output_file_id")
	}
	raw, err := c.downloadFile(ctx, base, meta.OutputFileID)
	if err != nil {
		return nil, err
	}
	return parseOutputLines(raw)
}

// CancelBatch requests cancellation of an in-flight batch (best effort).
func (c *Client) CancelBatch(ctx context.Context, batchID string) error {
	if c == nil {
		return fmt.Errorf("openaibatch: client is nil")
	}
	if ctx == nil {
		return fmt.Errorf("openaibatch: context is required")
	}
	base, err := GatewayRoot(c.BaseURL)
	if err != nil {
		return err
	}
	id := strings.TrimSpace(batchID)
	if id == "" {
		return fmt.Errorf("openaibatch: batch id is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/batches/"+id+"/cancel", nil)
	if err != nil {
		return wrap("cancel batch request", err)
	}
	c.setHeaders(req)
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return wrap("cancel batch", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusNotImplemented {
		slog.Debug("openaibatch: cancel not implemented", "status", resp.StatusCode)
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return httpErr("cancel batch", resp.StatusCode, string(raw))
	}
	return nil
}

func (c *Client) maxJSONLBytes() int {
	if c != nil && c.MaxJSONLBytes > 0 {
		return c.MaxJSONLBytes
	}
	return DefaultMaxJSONLBytes
}

func (c *Client) httpClient() *http.Client {
	if c != nil && c.HTTPClient != nil {
		return c.HTTPClient
	}
	return http.DefaultClient
}

func (c *Client) pollInterval() time.Duration {
	if c != nil && c.PollEvery > 0 {
		return c.PollEvery
	}
	return defaultPollInterval
}

func (c *Client) setHeaders(req *http.Request) {
	if c == nil || req == nil {
		return
	}
	if strings.TrimSpace(c.APIKey) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(c.APIKey))
	}
	for k, v := range c.Headers {
		if strings.TrimSpace(k) == "" {
			continue
		}
		req.Header.Set(k, v)
	}
}

func (c *Client) uploadJSONL(ctx context.Context, base, content string) (string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	if err := w.WriteField("purpose", "batch"); err != nil {
		return "", wrap("upload file", err)
	}
	fw, err := w.CreateFormFile("file", "batch.jsonl")
	if err != nil {
		return "", wrap("upload file", err)
	}
	if _, err := fw.Write([]byte(content)); err != nil {
		return "", wrap("upload file", err)
	}
	if err := w.Close(); err != nil {
		return "", wrap("upload file", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/files", &buf)
	if err != nil {
		return "", wrap("upload file", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	c.setHeaders(req)
	return c.doJSONID(req, "upload file")
}

func (c *Client) createBatch(ctx context.Context, base, inputFileID string) (string, error) {
	body := fmt.Sprintf(
		`{"input_file_id":%q,"endpoint":%q,"completion_window":%q}`,
		inputFileID, endpointChatCompletions, completionWindow,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/batches", strings.NewReader(body))
	if err != nil {
		return "", wrap("create batch", err)
	}
	req.Header.Set("Content-Type", "application/json")
	c.setHeaders(req)
	id, err := c.doJSONID(req, "create batch")
	if err != nil {
		return "", err
	}
	return id, nil
}

type batchMeta struct {
	ID           string `json:"id"`
	Status       string `json:"status"`
	OutputFileID string `json:"output_file_id"`
}

func (c *Client) pollTerminal(ctx context.Context, base, batchID string) (batchMeta, error) {
	var last batchMeta
	var cancelSent bool
	for {
		if err := ctx.Err(); err != nil {
			if !cancelSent {
				cancelSent = true
				_ = c.CancelBatch(context.WithoutCancel(ctx), batchID)
			}
			return last, wrap("poll batch", err)
		}
		meta, err := c.getBatch(ctx, base, batchID)
		if err != nil {
			return last, err
		}
		last = meta
		switch meta.Status {
		case "completed", "failed", "expired", "cancelled":
			return meta, nil
		case "validating", "in_progress":
		default:
			return meta, fmt.Errorf("openaibatch: unexpected batch status %q", meta.Status)
		}
		interval := c.pollInterval()
		t := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			t.Stop()
			if !cancelSent {
				cancelSent = true
				_ = c.CancelBatch(context.WithoutCancel(ctx), batchID)
			}
			return last, wrap("poll batch", ctx.Err())
		case <-t.C:
		}
	}
}

func (c *Client) getBatch(ctx context.Context, base, batchID string) (batchMeta, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/v1/batches/"+batchID, nil)
	if err != nil {
		return batchMeta{}, wrap("get batch", err)
	}
	c.setHeaders(req)
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return batchMeta{}, wrap("get batch", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return batchMeta{}, httpErr("get batch", resp.StatusCode, string(raw))
	}
	var meta batchMeta
	if err := json.Unmarshal(raw, &meta); err != nil {
		return batchMeta{}, wrap("get batch", err)
	}
	return meta, nil
}

func (c *Client) downloadFile(ctx context.Context, base, fileID string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/v1/files/"+fileID+"/content", nil)
	if err != nil {
		return nil, wrap("download file", err)
	}
	c.setHeaders(req)
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, wrap("download file", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, httpErr("download file", resp.StatusCode, string(raw))
	}
	return raw, err
}

func (c *Client) doJSONID(req *http.Request, op string) (string, error) {
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return "", wrap(op, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", httpErr(op, resp.StatusCode, string(raw))
	}
	var parsed struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", wrap(op, err)
	}
	if strings.TrimSpace(parsed.ID) == "" {
		return "", fmt.Errorf("openaibatch: %s: empty id in response", op)
	}
	return parsed.ID, nil
}
