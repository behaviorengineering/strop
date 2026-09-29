# openaibatch

Product-neutral HTTP client for the OpenAI Batch API (`POST /v1/files`, `POST /v1/batches`, poll, download output, cancel).

`BaseURL` must be the gateway **root** (no `/v1` suffix). Use `GatewayRoot` to normalize URLs that end in `/v1` or `/v1/chat/completions`.

## Usage

```go
client := &openaibatch.Client{
    BaseURL: "https://api.example.com",
    APIKey:  os.Getenv("API_KEY"),
}
results, err := client.RunChatBatch(ctx, []openaibatch.ChatLine{
    {CustomID: "job-1", Model: "gpt-4o-mini", Messages: []map[string]string{
        {"role": "user", "content": "Hello"},
    }},
})
```

Per-line failures appear in `LineResult.ErrMessage`; the batch still returns all parsed lines.

Large inputs are split under `DefaultMaxJSONLBytes` (10 MiB) into sequential batch runs.

## Live smoke

```bash
STROP_LIVE_OPENAI_BATCH=1 \
STROP_LIVE_BASE_URL=https://api.example.com \
STROP_LIVE_MODEL=your-model \
STROP_LIVE_API_KEY=your-key \
go test ./pkg/openaibatch/ -run TestLiveOpenAIBatch -count=1 -timeout 6m
```

If `GET /v1/batches` returns 404, the test passes without creating a batch.
