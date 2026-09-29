package openaibatch

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	endpointChatCompletions = "/v1/chat/completions"
)

// DefaultMaxJSONLBytes is the OpenAI batch input file size guidance (10 MiB).
const DefaultMaxJSONLBytes = 10 << 20

// ChatLine is one chat-completions row in a batch JSONL file.
type ChatLine struct {
	CustomID  string
	Model     string
	Messages  []map[string]string
	MaxTokens int
}

// LineResult is one parsed output row keyed by custom_id.
type LineResult struct {
	Content          string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	ErrMessage       string
}

func encodeChatJSONL(lines []ChatLine) (string, error) {
	if err := validateChatLines(lines); err != nil {
		return "", err
	}
	var b strings.Builder
	for _, ln := range lines {
		body := map[string]any{
			"model":    ln.Model,
			"messages": ln.Messages,
		}
		if ln.MaxTokens > 0 {
			body["max_tokens"] = ln.MaxTokens
		}
		bodyRaw, err := json.Marshal(body)
		if err != nil {
			return "", wrap("encode jsonl body", err)
		}
		row := map[string]any{
			"custom_id": ln.CustomID,
			"method":    "POST",
			"url":       endpointChatCompletions,
			"body":      json.RawMessage(bodyRaw),
		}
		line, err := json.Marshal(row)
		if err != nil {
			return "", wrap("encode jsonl row", err)
		}
		b.Write(line)
		b.WriteByte('\n')
	}
	return b.String(), nil
}

func validateChatLines(lines []ChatLine) error {
	if len(lines) == 0 {
		return fmt.Errorf("openaibatch: no batch lines")
	}
	model := strings.TrimSpace(lines[0].Model)
	if model == "" {
		return fmt.Errorf("openaibatch: model is required")
	}
	seen := make(map[string]struct{}, len(lines))
	for _, ln := range lines {
		id := strings.TrimSpace(ln.CustomID)
		if id == "" {
			return fmt.Errorf("openaibatch: custom_id is required")
		}
		if _, dup := seen[id]; dup {
			return fmt.Errorf("openaibatch: duplicate custom_id %q", id)
		}
		seen[id] = struct{}{}
		if strings.TrimSpace(ln.Model) != model {
			return fmt.Errorf("openaibatch: all lines must use the same model")
		}
	}
	return nil
}

// EncodedJSONLSize returns the byte length of the JSONL encoding for lines.
func EncodedJSONLSize(lines []ChatLine) (int, error) {
	s, err := encodeChatJSONL(lines)
	if err != nil {
		return 0, err
	}
	return len(s), nil
}

// SplitUnderCap groups lines into chunks whose encoded JSONL size is at most maxBytes.
// A single line larger than maxBytes is returned alone in a chunk (RunChatBatch returns OverCapError).
func SplitUnderCap(lines []ChatLine, maxBytes int) ([][]ChatLine, error) {
	if maxBytes <= 0 {
		return nil, fmt.Errorf("openaibatch: max bytes must be positive")
	}
	if len(lines) == 0 {
		return nil, nil
	}
	var chunks [][]ChatLine
	var cur []ChatLine
	for _, ln := range lines {
		try := append(append([]ChatLine{}, cur...), ln)
		size, err := EncodedJSONLSize(try)
		if err != nil {
			return nil, err
		}
		if size <= maxBytes {
			cur = try
			continue
		}
		if len(cur) > 0 {
			chunks = append(chunks, cur)
			cur = nil
		}
		oneSize, err := EncodedJSONLSize([]ChatLine{ln})
		if err != nil {
			return nil, err
		}
		if oneSize > maxBytes {
			chunks = append(chunks, []ChatLine{ln})
			continue
		}
		cur = []ChatLine{ln}
	}
	if len(cur) > 0 {
		chunks = append(chunks, cur)
	}
	return chunks, nil
}

func parseOutputLines(raw []byte) (map[string]LineResult, error) {
	out := make(map[string]LineResult)
	for _, line := range splitJSONL(raw) {
		if len(line) == 0 {
			continue
		}
		var row struct {
			CustomID string `json:"custom_id"`
			Response struct {
				StatusCode int `json:"status_code"`
				Body       struct {
					Choices []struct {
						Message struct {
							Content string `json:"content"`
						} `json:"message"`
					} `json:"choices"`
					Usage struct {
						PromptTokens     int `json:"prompt_tokens"`
						CompletionTokens int `json:"completion_tokens"`
						TotalTokens      int `json:"total_tokens"`
					} `json:"usage"`
				} `json:"body"`
			} `json:"response"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(line, &row); err != nil {
			return nil, wrap("parse output line", err)
		}
		id := strings.TrimSpace(row.CustomID)
		if id == "" {
			continue
		}
		res := LineResult{}
		if row.Error != nil && strings.TrimSpace(row.Error.Message) != "" {
			res.ErrMessage = strings.TrimSpace(row.Error.Message)
			out[id] = res
			continue
		}
		if row.Response.StatusCode != 0 && (row.Response.StatusCode < 200 || row.Response.StatusCode >= 300) {
			res.ErrMessage = fmt.Sprintf("http %d", row.Response.StatusCode)
			out[id] = res
			continue
		}
		if len(row.Response.Body.Choices) == 0 {
			res.ErrMessage = "missing choices"
			out[id] = res
			continue
		}
		res.Content = strings.TrimSpace(row.Response.Body.Choices[0].Message.Content)
		res.PromptTokens = row.Response.Body.Usage.PromptTokens
		res.CompletionTokens = row.Response.Body.Usage.CompletionTokens
		res.TotalTokens = row.Response.Body.Usage.TotalTokens
		out[id] = res
	}
	return out, nil
}

func splitJSONL(raw []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i := 0; i < len(raw); i++ {
		if raw[i] != '\n' {
			continue
		}
		line := bytesTrimSpace(raw[start:i])
		if len(line) > 0 {
			lines = append(lines, line)
		}
		start = i + 1
	}
	if start < len(raw) {
		line := bytesTrimSpace(raw[start:])
		if len(line) > 0 {
			lines = append(lines, line)
		}
	}
	return lines
}

func bytesTrimSpace(b []byte) []byte {
	return bytes.TrimSpace(b)
}
