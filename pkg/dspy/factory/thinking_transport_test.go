package factory

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type captureRoundTripper struct {
	body []byte
}

func (c *captureRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	c.body = body
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{}`)),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

func TestThinkingRoundTripperAddsEnableThinking(t *testing.T) {
	capture := &captureRoundTripper{}
	req, err := http.NewRequest(
		http.MethodPost,
		"https://api.example.com/v1/chat/completions",
		strings.NewReader(`{"model":"@cf/google/gemma-4-26b-a4b-it","messages":[]}`),
	)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}

	resp, err := (thinkingRoundTripper{base: capture}).RoundTrip(req)
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	defer resp.Body.Close()

	var payload map[string]any
	if err := json.Unmarshal(capture.body, &payload); err != nil {
		t.Fatalf("decode forwarded request: %v", err)
	}
	kwargs, ok := payload["chat_template_kwargs"].(map[string]any)
	if !ok {
		t.Fatalf("chat_template_kwargs = %#v, want object", payload["chat_template_kwargs"])
	}
	if enabled, ok := kwargs["enable_thinking"].(bool); !ok || !enabled {
		t.Fatalf("enable_thinking = %#v, want true", kwargs["enable_thinking"])
	}
}

func TestThinkingRoundTripperLeavesNonChatRequestsUnchanged(t *testing.T) {
	capture := &captureRoundTripper{}
	body := `{"model":"@cf/google/gemma-4-26b-a4b-it","input":"hello"}`
	req, err := http.NewRequest(
		http.MethodPost,
		"https://api.example.com/v1/embeddings",
		strings.NewReader(body),
	)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}

	resp, err := (thinkingRoundTripper{base: capture}).RoundTrip(req)
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	defer resp.Body.Close()

	if string(capture.body) != body {
		t.Fatalf("forwarded body = %s, want %s", capture.body, body)
	}
}
