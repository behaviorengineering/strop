package openaibatch

import (
	"strings"
	"testing"
)

func TestEncodeChatJSONLRejects(t *testing.T) {
	_, err := encodeChatJSONL([]ChatLine{
		{CustomID: "a", Model: "m1"},
		{CustomID: "b", Model: "m2"},
	})
	if err == nil {
		t.Fatal("expected mixed model error")
	}
	_, err = encodeChatJSONL([]ChatLine{{CustomID: "", Model: "m"}})
	if err == nil {
		t.Fatal("expected empty custom_id error")
	}
	_, err = encodeChatJSONL([]ChatLine{
		{CustomID: "x", Model: "m"},
		{CustomID: "x", Model: "m"},
	})
	if err == nil {
		t.Fatal("expected duplicate custom_id error")
	}
}

func TestParseOutputLines(t *testing.T) {
	raw := []byte(`{"custom_id":"ok","response":{"status_code":200,"body":{"choices":[{"message":{"content":"hello"}}],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}}}
{"custom_id":"bad","error":{"message":"line failed"}}
{"custom_id":"nope","response":{"status_code":500,"body":{"choices":[]}}}
{"custom_id":"empty","response":{"status_code":200,"body":{"choices":[]}}}
`)
	got, err := parseOutputLines(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got["ok"].Content != "hello" || got["ok"].TotalTokens != 3 {
		t.Fatalf("ok line: %+v", got["ok"])
	}
	if got["bad"].ErrMessage != "line failed" {
		t.Fatalf("bad line: %+v", got["bad"])
	}
	if got["nope"].ErrMessage != "http 500" {
		t.Fatalf("nope line: %+v", got["nope"])
	}
	if got["empty"].ErrMessage != "missing choices" {
		t.Fatalf("empty line: %+v", got["empty"])
	}
}

func TestSplitUnderCap(t *testing.T) {
	lines := []ChatLine{
		{CustomID: "a", Model: "m", Messages: []map[string]string{{"role": "user", "content": strings.Repeat("x", 80)}}},
		{CustomID: "b", Model: "m", Messages: []map[string]string{{"role": "user", "content": strings.Repeat("y", 80)}}},
		{CustomID: "c", Model: "m", Messages: []map[string]string{{"role": "user", "content": "z"}}},
	}
	chunks, err := SplitUnderCap(lines, 400)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
	total := 0
	for _, ch := range chunks {
		total += len(ch)
	}
	if total != 3 {
		t.Fatalf("lost lines: %d", total)
	}
}

func TestIsOverCap(t *testing.T) {
	huge := ChatLine{
		CustomID: "big",
		Model:    "m",
		Messages: []map[string]string{{"role": "user", "content": strings.Repeat("q", 5000)}},
	}
	c := &Client{BaseURL: "http://example.com", MaxJSONLBytes: 200}
	_, err := c.RunChatBatch(t.Context(), []ChatLine{huge})
	if !IsOverCap(err) {
		t.Fatalf("expected over cap, got %v", err)
	}
}
