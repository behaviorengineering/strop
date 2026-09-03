package factory

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// thinkingRoundTripper adds the Gemma 4 chat-template option to chat requests.
type thinkingRoundTripper struct {
	base http.RoundTripper
}

func configureThinkingTransport(llm httpClientGetter) error {
	if llm == nil {
		return fmt.Errorf("LLM is nil")
	}

	client := llm.GetHTTPClient()
	if client == nil {
		return fmt.Errorf("LLM HTTP client is nil")
	}

	client.Transport = thinkingRoundTripper{base: client.Transport}
	return nil
}

func (t thinkingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, fmt.Errorf("HTTP request is nil")
	}
	if req.Body == nil || !strings.HasSuffix(req.URL.Path, "/chat/completions") {
		return t.roundTrip(req)
	}

	defer req.Body.Close()

	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, fmt.Errorf("read chat completion request: %w", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode chat completion request: %w", err)
	}

	templateKwargs := map[string]any{}
	if existing, ok := payload["chat_template_kwargs"].(map[string]any); ok {
		for key, value := range existing {
			templateKwargs[key] = value
		}
	}
	templateKwargs["enable_thinking"] = true
	payload["chat_template_kwargs"] = templateKwargs

	updatedBody, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode chat completion request: %w", err)
	}

	updatedReq := req.Clone(req.Context())
	updatedReq.Body = io.NopCloser(bytes.NewReader(updatedBody))
	updatedReq.ContentLength = int64(len(updatedBody))
	updatedReq.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(updatedBody)), nil
	}

	return t.roundTrip(updatedReq)
}

func (t thinkingRoundTripper) roundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}
