---
name: openai-batch
description: >-
  OpenAI Batch for RLM QueryBatched fan-out: pkg/openaibatch HTTP client,
  PreferBatch SubLLMClient, CreateRLM defaults, strict sync fallback. Use when
  wiring RLM REPL sub-queries, digest ledger batch paths, or reviewing batch vs
  sync LLM spend.
---

# OpenAI Batch (RLM fan-out)

**Module:** `github.com/behaviorengineering/strop` (`pkg/openaibatch`, `pkg/dspy/subllm`, `pkg/dspy` RLM config).

**Related:** `.cursor/skills/inference-pace/SKILL.md` (outer controller `Generate` pacing; batch does not replace the admit gate on sync paths).

---

## When to load

- Adding or reviewing RLM jobs that call `QueryBatched` / `QueryBatchedRaw` from the REPL.
- Implementing multi-prompt fan-out (ledger slices, parallel sub-questions in one turn).
- Choosing between OpenAI Batch (`/v1/files`, `/v1/batches`) and parallel sync `Generate`.
- Reviewing strop `CreateRLM` / `RLMConfig.CreateModule` wiring.

---

## Default (product-neutral)

| Piece | Role |
| --- | --- |
| **`openaibatch.Client`** | Upload JSONL, create batch, poll, download output, best-effort cancel |
| **`subllm.PreferBatch`** | `QueryBatched` prefers batch when probe + deadline gates pass |
| **`CreateRLM`** | OpenAI-schema providers: set gateway root + API key on `RLMConfig.OpenAIBatch`, model on `BatchModel` **before** `CreateLLM` |
| **`rlm.New`** | Install explicit `SubLLMClient`; MUST NOT use `NewFromLLM` as the batch install path |

**CONSTRAINT:** Outer RLM controller turns stay sync `Generate` (optionally inference-pace wrapped). MUST NOT batch multi-turn controller loops across turns.

**CONSTRAINT:** Batch applies to independent prompts inside **one** `QueryBatched` call (`len > 1`), not turn *n+1* after turn *n*.

**CONSTRAINT:** When `RLMConfig.MaxTokens > 0`, dspy-go serializes `QueryBatched` into sequential `Query`. Batch cannot run. MUST leave `MaxTokens` at 0 for batch fan-out or accept sync-only sub-queries.

**CONSTRAINT:** After a batch job is submitted for a set of prompts, MUST NOT also run sync `Generate` for the same prompts (double-spend).

Enforcement: trace `QueryBatched` → `PreferBatch` → `RunChatBatch`; confirm no sync fallback on `PostSubmitBatchError`.

Violation: STOP; return error or wait on batch; do not fan out sync duplicates.

CORRECT:
```text
ON CreateRLM (openai schema): capture BatchModel + OpenAIBatch.BaseURL/APIKey before SetWrapLLM
ON CreateModule: rlm.New(rootLLM, PreferBatch(sync, openaibatch.Client, model), opts...)
ON QueryBatched len>1: probe GET /v1/batches (5s) → RunChatBatch → map LineResult per custom_id
ON 404/401/not-ready/model reject: latch unavailable for process; later calls sync only
ON 429/503: sync this call only; batch stays enabled
```

PROHIBITED:
```text
NewFromLLM without PreferBatch when batch surface exists
IF batch poll failed after create → sync the same prompts again
IF MaxTokens>0 → claim batch is enabled for QueryBatched
Host product names in pkg/openaibatch or PreferBatch
```

---

## Gates (PreferBatch)

| Condition | Behavior |
| --- | --- |
| `len(prompts) == 1` | Sync only (no batch HTTP) |
| Remaining ctx deadline `< 30s` | Sync only |
| `STROP_OPENAI_BATCH=0` or `OpenAIBatch.Disabled` | Sync only |
| Empty batch base URL or model | Sync only |
| Probe 404 / 401 / not-ready | Latch unavailable; sync thereafter |
| Create/upload 404 / 401 / 400 / 422 (model not batchable) | Latch unavailable; sync this call |
| 429 / 503 on probe or pre-submit | Sync **this call**; do not latch |
| `IsOverCap` single line | Sync this call |
| Poll / download after submit | Error only; no sync fallback |

Prompt strings in batch JSONL are the final REPL strings (`QueryBatched` already prepends context; `QueryBatchedRaw` is as-is).

---

## HTTP client (`pkg/openaibatch`)

- `BaseURL` is gateway **root** (no `/v1`). Use `GatewayRoot` to normalize.
- Auth: `APIKey` → `Authorization: Bearer`; optional `Headers` on every files/batches request.
- Input JSONL capped at 10 MiB; larger fan-outs split into sequential batch runs.
- Per-line failures: `LineResult.ErrMessage`; whole batch still returns the map.
- Disable for tests: do not set `STROP_LIVE_OPENAI_BATCH=1` in CI.

Live smoke (operator):

```bash
STROP_LIVE_OPENAI_BATCH=1 STROP_LIVE_BASE_URL=https://api.example.com \
  STROP_LIVE_MODEL=your-model STROP_LIVE_API_KEY=secret \
  go test ./pkg/openaibatch/ -run TestLiveOpenAIBatch -count=1 -timeout 6m
```

---

## Review checklist (binary)

- [ ] `CreateModule` uses `rlm.New` with `PreferBatch` when batch is enabled
- [ ] Model and batch auth captured before LLM wrap (`BatchModel`, `OpenAIBatch`)
- [ ] `MaxTokens == 0` when batch fan-out is required
- [ ] No sync fallback after submit (`PostSubmitBatchError` path)
- [ ] 404/401 latched vs 429/503 this-call-only documented in code paths
- [ ] Outer loop pacing (inference-pace) still on sync `Generate` paths
- [ ] Product-neutral naming in library code and examples
