---
name: strop-orchestration
description: >-
  strop orchestration: JobRunner-only jobs vs refinement loops, field/section/phase
  composition walks, RegenerateOptions, and max-versions policy. Use when implementing
  generate-evaluate-refine flows, composition strategies, or choosing which loop API to use.
---

# strop orchestration

**Module:** `github.com/behaviorengineering/strop`. Full package map: strop `README.md`.

**Related:** `.cursor/skills/strop-pipeline-pattern/SKILL.md`, `.cursor/skills/dspy-go-debugging/SKILL.md` (§5 orchestration).

---

## When to load

- Adding a job that may need more than one generate → evaluate round.
- Choosing between field-walk, section-walk, and phase-walk composition.
- Debugging refinement exit, max-versions, per-item loops, or composition pass/fail.

---

## Decision tree: which API?

| Shape | Use |
|-------|-----|
| Single generator call, no iterative refinement | `dspy/runner.JobRunner` only — `Generate` then optional `EvaluateWorkflow` |
| One string field per step, strop pass/fail, flat `map[string]string` draft | `orchestration.NewFieldWalkStrategy` |
| One section per step, strop pass/fail, **typed** document draft | `DocumentSectionDefinition` + `orchestration.NewSectionWalkStrategy` |
| Multi-field phases, **app** decides pass/fail per phase | `DocumentArcDefinition` + `orchestration.NewPhaseWalkStrategy` |

**Do not use PhaseWalk for single-section polish jobs.** Section refinement belongs on **SectionWalk** (or FieldWalk for flat maps).

---

## Refinement loop (single entity)

**Package:** `strop/orchestration`

| Item | Purpose |
|------|---------|
| `RefinementStrategy` | `LoadContext`, `GenerateAndEvaluate`, `SaveVersion`, `ContextID` — implement per job |
| `RunRefinementLoop` | Runs generate → evaluate → stop or recurse with feedback |
| `StoppingPolicy` | Satisfied by `strop/refinement.ServiceInterface` |

**Regenerate / max versions:** Use `strop/regenerate.RegenerateOptions{Force, Message}` at the service entry. Cap math: `strop/regenerate.ShouldSkipMaxVersions`, `EffectiveMaxVersions`. Do not duplicate cap logic in each service.

**Healing:** If score decreases, the loop MUST attempt self-healing before stopping for regression (`maxHealingAttempts >= 1`).

**Early exit:** Successful early exit MUST return a concrete selected ID — never `uuid.Nil`. Fallback order: pending → approved → any existing record.

---

## Per-item refinement

**When:** Multiple items (chapters, ideas, quotes) each refined independently; **one** `SaveVersion` at the end.

| Item | Purpose |
|------|---------|
| `PerItemRefinementStrategy` | `ItemCount`, `GenerateAndEvaluateOne`, seed state in `LoadContext` |
| `RunPerItemRefinementLoop` | All items |
| `RunPerItemRefinementLoopWithIndices` | Subset only — seed unchanged slots from selected version |

**Partial regen pattern:**

1. Load selected version row; return domain error if baseline missing.
2. Deep-copy per-item slice into strategy seed; re-sync length/identity in `LoadContext`.
3. Expose scoped service entry (e.g. `RegenerateXForIndices`).
4. Persist **full** slice in one `SaveVersion`.

---

## Composition strategies

Both implement `CompositionStrategy` for `RunCompositionLoop` (ordered phases, per-phase retry, lock on pass).

### Field-walk

- One owned field per phase (phase ID = field key).
- Strop pass/fail: `MinPassScore`, non-empty output, empty source auto-pass.
- Prior fields locked. Result: `FieldWalkState` + aggregated evals.

### Section-walk

- Typed draft via `SectionCodec[T]` (`ToMap` / `FromMap`, optional `SourceText`).
- One section ID per phase; same strop pass/fail rules.
- Result: `SectionWalkState[T]` + aggregated evals.

### Phase-walk

- `PhaseWalkOwnedFields` maps phase ID → writable field keys.
- **App runner** decides pass/fail, score, feedback per phase.
- Set `MergeOnFail: true` when the runner merges generator output before evaluation.
- Optional `PhaseWalkFinalize` for typed final artifact.

**Portable recipes:** `DocumentSectionDefinition`, `DocumentArcDefinition` in `strop/orchestration` — helpers for phase order, locks, owned fields, `MinPassScore`.

**Strop does not include:** generator/evaluator prompt text, DSPy modules, or domain alignment checks — app supplies runners and instruction providers.

---

## Evaluator integration

- `EvaluateWorkflow(ctx, job, genInput, generatorOutput, eventChan)` — do not hand-build `generator_input` maps.
- `ToMap()` is generate-only. `EvaluationMap()` is the evaluator view.
- Empty evaluator `feedback` is a contract violation — validation MUST fail so retry runs.

---

## Bootstrap order (any strop consumer)

1. Map provider YAML → `dspy.ProviderConfig`.
2. Adapt app logger → `strop/log.Logger`.
3. Empty `registry.ModuleRegistry`.
4. `factory.LLMFactory` → `InterceptorSetup` → generator/evaluator/feedback factories.
5. Register output validators / mandatory fields before `CreateGenerator`.
6. Optionally `SetStructuredOutputHooks` for product parse/format behaviour.
7. Register generators, evaluators, workflows.
8. Run jobs through `runner.JobRunner`.

Default validation: `ValidateMandatoryFields` from the signature when no extra validator is registered.
