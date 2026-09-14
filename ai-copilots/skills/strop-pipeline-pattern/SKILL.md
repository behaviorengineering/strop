---
name: strop-pipeline-pattern
description: >-
  Pipeline layout on strop: JobRunner, per-job clients and modules, typed inputs,
  chained evaluators with criteria, one table per job, and XML-first structured output.
  Use when adding a pipeline or job, aligning clients to strop, or wiring evaluation workflows.
---

# strop pipeline pattern

**Principle:** Each pipeline has a `clients` package using shared **`strop/dspy/runner.JobRunner`**. Per-job **clients** hold the runner and delegate Generate/Evaluate; per-job **modules** hold signatures and prompts. Typed **inputs** implement `GeneratorInput` (`ToMap`, `GetVersion`) and `EvaluationInput` (`EvaluationMap`).

**Related:** `.cursor/skills/strop-orchestration/SKILL.md`, `.cursor/skills/dspy-xml-structured-output/SKILL.md`, `.cursor/skills/dspy-prompt-engineering/SKILL.md`, `.cursor/skills/dspy-module-patterns/SKILL.md`, `.cursor/skills/dspy-pipeline-isolation/SKILL.md`, `.cursor/skills/golang-quality/SKILL.md` (CONSTRAINT 16 durable AI dumps; CONSTRAINT 17 module isolation). Product-specific overlays (YouTube, sayings paths) may exist as a **project** skill — load both; do not invent a second job pattern.

---

## 1. Layout (per pipeline)

| Piece | Location | Purpose |
|-------|----------|---------|
| **Runner** | Injected `*runner.JobRunner` | `Generate`, `EvaluateWorkflow` |
| **types.go** | `internal/pipelines/<pipeline>/clients/` | Input structs: `ToMap()`, `EvaluationMap()`, `GetVersion()` |
| **{job}_client.go** | Same | Delegates to `r.Generate` / `r.EvaluateWorkflow` |
| **{job}_modules.go** | Same | Signatures, prompts, generator/evaluator config only |
| **signature_helpers.go** | Same | Field helpers for signatures |
| **evaluation_shared.go** | Same | Chained evaluator config, consolidator, shared score prompt |
| **register.go** | Same | Registers generators + evaluation workflows |
| **constants.go** | Same | Job keys, step names, field names |
| **Services** | `internal/pipelines/<pipeline>/services/` | Business logic; uses clients, persists versions |
| **Database** | `internal/pipelines/<pipeline>/database/` | **One versioned table per job** |

**Naming:** Singular job name: `topic_client.go` + `topic_modules.go`, not `topics_*`.

**One job, one table:** Do not merge multiple jobs' outputs into one table.

---

## 2. Add a new job (checklist)

- [ ] `XInput` in `types.go` with `ToMap()`, `EvaluationMap()`, `GetVersion()`.
- [ ] `XClient` with `GenerateX` and **`EvaluateX`** (always both).
- [ ] `{job}_modules.go`: generator config + `ChainedEvaluatorConfig` with `CriterionIDs`.
- [ ] Versioned table + repo with `CreateVersionAndSupersedeOlder`.
- [ ] Register generator **and** evaluation workflow in `register.go`.
- [ ] Service uses typed input; no duplicate runner logic.
- [ ] Refinement jobs: `strop/orchestration` — see `strop-orchestration` skill.
- [ ] Regenerate: explicit options type (`strop/regenerate.RegenerateOptions` or app alias).

---

## 3. Evaluators (chained + consolidation)

**Requirement:** Every generator job MUST have **chained** evaluators (feedback analysis → score generation) with **criteria**, then **consolidation**. No simple one-step evaluators.

**Shared signature:** `dspy.DefaultChainedEvaluatorSignature()` — do not redefine the envelope.

**Flow:** Chained evaluators → consolidation merges feedback from multiple evaluators.

**Criteria alignment:** The criterion set for a job = **union of `CriterionIDs`** from in-loop evaluators. Use the same set for generator guidance (if any), evaluators, and human review. Do not add review-only criteria that no evaluator scores.

**Empty `feedback`:** Contract violation — mandatory-field validation MUST fail so `RetryModuleInterceptor` re-runs; never inject synthetic checklist lines.

**AI cadence (reader-facing prose):** Jobs that emit human-readable explanatory prose (essay composition/polish, claims thoughts, video TL;DW, newsletter-like jobs) MUST attach `strop/evaluation/aicadence` via `dspy.AppendAICadenceEvaluator` and prefer a **cheap** model for that role through `CreateChainedEvaluatorsFromConfig` `roleProviders` — the same class of MUST as wiring `RetryModuleInterceptor`. Score only `no_aphorism_stack`. Do **not** copy cadence prompts into each `{job}_modules.go`. Do **not** attach on vernacular-aphorism jobs (sayings fluff/ideation) or structural extractors (chapters/speakers). Optional Go belt: `aicadence.HeuristicFeedback` plus product phrase banks.

**Envelope vs inner keys:** `EvaluateWorkflow` builds `generator_input` / `generator_output` containers. Inner keys are **per job** — prompts and Go maps MUST agree.

| Item | Location (strop) |
|------|------------------|
| `DefaultChainedEvaluatorSignature` | `strop/dspy/chained_evaluator.go` |
| `ChainedEvaluatorConfig` | `strop/dspy/chained_evaluator_config.go` |
| `CreateChainedEvaluatorsFromConfig` | `strop/dspy/factory/evaluator_factory.go` |
| `AppendAICadenceEvaluator` / `aicadence` | `strop/dspy/aicadence_append.go`, `strop/evaluation/aicadence` |
| `ConsolidatorPromptBuilder` | `strop/dspy/chained_evaluator.go` |
| Criterion prompt builders | `strop/evaluation/criteria/prompt_builder.go` |

Register product rubrics at container startup; strop ships portable process/quality rubrics only.

---

## 4. WithXMLFormatting in Create*

**Rule:** Apply **`dspy.WithXMLFormatting`** inside Create* functions, not at call sites.

| Module kind | Where |
|-------------|--------|
| Generators | `CreateGeneratorModule` (+ `SharedInstructions.GeneratorObjectiveRecitation`) |
| Chained evaluator | `CreateChainedEvaluatorModule` |
| Consolidator | `CreateDefaultConsolidatorModule` |

Optional **Persona** renders first in the instruction when non-empty.

---

## 5. Structured output (XML-first)

**Enforce:** Do not describe generator output as JSON inside XML tags. Use nested XML or repeated `<item>` for lists.

| Pattern | Rule |
|---------|------|
| Scalar | One XML tag per field; text content directly in tag |
| List | Description includes **"list of"** or **"array"**; repeated `<item>` children |
| Map | Description includes **"map"** or **"dictionary"** — NEVER **"object"** for maps |
| Robustness | Split into explicit top-level fields — no monolithic catch-all blob |

Parser details: `.cursor/skills/dspy-xml-structured-output/SKILL.md`.

---

## 6. Generator rationale (centralized)

**Single source:** `strop/dspy/signature_helpers.go`, `strop/dspy/constants.go` — do not paste long duplicate rationale prose into each `{job}_modules.go`.

| Helper | When |
|--------|------|
| `SharedInstructions.GeneratorObjectiveRecitation` | Appended by `CreateGeneratorModule` |
| `RationaleDescriptionWithContext(taskFocus)` | Default `rationale` field description |
| `RationaleDescriptionWithExtra(taskFocus, extra)` | Module-specific emit-order or length rules |

Contract: VOICE/MUST/ANTI_PATTERN first, then action-chain bullets (~5 lines unless job defines longer plan). Plain text only inside rationale.

Evaluators use `DefaultChainedEvaluatorSignature()` — they do **not** get generator objective recitation.

---

## 7. EvaluateWorkflow contract

```go
// Generate
r.Generate(ctx, jobKey, config, input.ToMap(), eventChan)

// Evaluate — always use EvaluationMap for evaluator envelope
r.EvaluateWorkflow(ctx, job, genInput, generatorOutput, eventChan)
```

Do not hand-build `generator_input` maps in services.

---

## 8. Durable TraceDir, module traces, and runreport (AI testing)

**CONSTRAINT:** When wiring `RLMConfig.TraceDir`, `dspy.AttachModuleTrace`, or `runreport.Config.Dir`, MUST point them at a durable work-story root that survives process exit. MUST NOT nest the only dump under an analysis/cache `MkdirTemp` that `defer os.RemoveAll` deletes.

- Enforcement: Pair every TraceDir / module-trace dir / runreport Dir with the cleanup path of its parent tree; confirm dumps outlive that cleanup; log or return the durable root.
- Violation: STOP, retarget dumps (or copy before cleanup), expose the path, re-check.

`InterceptorSetup` always attaches dspy-go `TracingInterceptor`. It is a no-op until the host calls `dspy.AttachModuleTrace` (puts a TraceSession on ctx). That dump is the CoT/Predict I/O story (inputs + outputs JSONL), sibling to RLM `TraceDir`.

CORRECT:
```go
workStory := opts.WorkStoryDir // durable; not teaching branch
rlmCfg.TraceDir = filepath.Join(workStory, "rlm-traces", task)
runReport.Dir = filepath.Join(workStory, "logs", "runs")
ctx, closeTrace, err := dspy.AttachModuleTrace(ctx, filepath.Join(workStory, "module-traces"), map[string]any{"job": "digest"})
defer func() { _ = closeTrace() }()
```

PROHIBITED:
```go
defer os.RemoveAll(analysisDir)
rlmCfg.TraceDir = filepath.Join(analysisDir, "rlm-traces", task)
```

OTEL / OpenInference spans remain required (golang-quality C15). They do not replace on-disk JSONL/JSON for local AI debugging (golang-quality C16).

When `RLMComplete` runs with TraceDir set, strop also appends `rlm_inputs.jsonl` in that directory with the **full** context and query (and later the final answer). Prefer that sidecar for fixtures: dspy-go's session metadata still truncates context to ~500 chars for display.

---

## 9. Isolate generators and evaluators before the chain

**CONSTRAINT:** When adding or changing a pipeline generator or evaluator, MUST capture Process inputs from `AttachModuleTrace` / TraceDir dumps and exercise that module alone (offline zip gates + env-gated live opt-in) before relying on a full JobRunner reseed. Discrete machine contracts MUST stay on structured signature fields.

- Enforcement: Host has `testdata` fixture + `LIVE_*` replay test (or documented offline-only rationale) for touched tasks.
- Violation: STOP, extract a module-trace span, add replay, re-check.

Full practice: `.cursor/skills/dspy-pipeline-isolation/SKILL.md` (golang-quality C17). Module-trace dumps from §8 are the fixture source for CoT; RLM fixtures prefer TraceDir/`rlm_inputs.jsonl`.
