package openaibatch

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLiveOpenAIBatch(t *testing.T) {
	if os.Getenv("STROP_LIVE_OPENAI_BATCH") != "1" {
		t.Skip("set STROP_LIVE_OPENAI_BATCH=1 for live OpenAI-compatible batch smoke")
	}
	base := strings.TrimSpace(os.Getenv("STROP_LIVE_BASE_URL"))
	if base == "" {
		base = "http://127.0.0.1:1320"
	}
	root, err := GatewayRoot(base)
	if err != nil {
		t.Fatal(err)
	}
	apiKey := os.Getenv("STROP_LIVE_API_KEY")
	model := strings.TrimSpace(os.Getenv("STROP_LIVE_MODEL"))
	if model == "" {
		t.Fatal("STROP_LIVE_MODEL is required for live batch smoke")
	}

	probeClient := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, root+"/v1/batches", nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(apiKey) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))
	}
	resp, err := probeClient.Do(req)
	if err != nil {
		t.Fatalf("probe GET /v1/batches: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		t.Log("batch surface not exposed (404); skipping create")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	client := &Client{
		BaseURL:    root,
		APIKey:     apiKey,
		HTTPClient: probeClient,
		PollEvery:  2 * time.Second,
	}
	results, err := client.RunChatBatch(ctx, []ChatLine{
		{
			CustomID: "live-1",
			Model:    model,
			Messages: []map[string]string{{"role": "user", "content": "Reply with the single word: ok"}},
			MaxTokens: 16,
		},
		{
			CustomID: "live-2",
			Model:    model,
			Messages: []map[string]string{{"role": "user", "content": "Reply with the single word: yes"}},
			MaxTokens: 16,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for id, res := range results {
		if strings.TrimSpace(res.Content) == "" && strings.TrimSpace(res.ErrMessage) == "" {
			t.Fatalf("line %s: empty content and error", id)
		}
	}
}
