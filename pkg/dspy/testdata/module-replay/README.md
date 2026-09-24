# Module-replay fixtures (strop)

Portable Process-span captures for isolating generators and evaluators without a
full host pipeline. Shape matches host spans (`fields` + `recorded_outputs`).

| Kind | Fixture | Offline gate |
|------|---------|--------------|
| Generator | `generator_span.json` | recorded outputs non-empty; required keys present |
| Evaluator | `evaluator_span.json` | `criterion_scores` parse through workflow helpers |

## Capture

1. Run a host job with `dspy.AttachModuleTrace` on a durable work-story path.
2. Copy one `Predict:<task>` (or evaluator) span into a JSON file here with
   `fields` (Process inputs) and `recorded_outputs` (parsed outs).
3. Keep content product-neutral when committing into strop.

## Live replay

Default `go test` stays offline. Opt in:

```bash
STROP_LIVE_MODULE_REPLAY=1 \
STROP_LIVE_BASE_URL=http://127.0.0.1:1320/v1 \
STROP_LIVE_MODEL=your-model \
STROP_LIVE_API_KEY=dummy \
  go test ./pkg/dspy/ -run LiveModuleReplay -count=1 -v -timeout 5m
```

`STROP_LIVE_API_SCHEMA` defaults to `openai`. Polypus (or any OpenAI-compatible
endpoint) must answer `GET {base}/models`.
