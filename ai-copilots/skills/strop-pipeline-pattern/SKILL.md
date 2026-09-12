---
name: strop-pipeline-pattern
description: >-
  Pipeline layout on strop: JobRunner, per-job clients and modules, typed inputs,
  chained evaluators with criteria, one table per job, and XML-first structured output.
  Use when adding a pipeline or job, aligning clients to strop, or wiring evaluation workflows.
---

# strop pipeline pattern

**Principle:** Each pipeline has a `clients` package using shared **`strop/dspy/runner.JobRunner`**. Per-job **clients** hold the runner and delegate Generate/Evaluate; per-job **modules** hold signatures and prompts. Typed **inputs** implement `GeneratorInput` (`ToMap`, `GetVersion`) and `EvaluationInput` (`EvaluationMap`).

**Related:** `.cursor/skills/strop-orchestration/SKILL.md`, `.cursor/skills/dspy-xml-structured-output/SKILL.md`, `.cursor/skills/dspy-prompt-engineering/SKILL.md`, `.cursor/skills/dspy-module-patterns/SKILL.md`. Product-specific overlays (YouTube, sayings paths) may exist as a **project** skill — load both; do not invent a second job pattern.

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

**Envelope vs inner keys:** `EvaluateWorkflow` builds `generator_input` / `generator_output` containers. Inner keys are **per job** — prompts and Go maps MUST agree.

| Item | Location (strop) |
|------|------------------|
| `DefaultChainedEvaluatorSignature` | `strop/dspy/chained_evaluator.go` |
| `ChainedEvaluatorConfig` | `strop/dspy/chained_evaluator_config.go` |
| `CreateChainedEvaluatorsFromConfig` | `strop/dspy/factory/evaluator_factory.go` |
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
