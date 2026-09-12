---
name: strop-human-review
description: >-
  Human review on strop: Gate, reject-and-regenerate, reviewflow engine and ports,
  job/step registry, and regeneration UX. Use when adding review to a pipeline or
  implementing Prompter/Generator/Session adapters.
---

# strop human review

**Packages:** `strop/humanreview`, `strop/humanreview/reviewflow`.

**Principle:** strop ships the **Gate**, feedback normalizers, reviewflow **engine**, and port interfaces. The app implements **ports** (Prompter, Generator, Session), persists evaluations, and wires job/step constants at container startup.

**Related:** `.cursor/skills/strop-pipeline-pattern/SKILL.md`. Product classroom UX (pterm, learning index) stays in the **consumer** repo.

---

## When to load

- Adding human review after generation + evaluation.
- Implementing disagree → feedback → regenerate.
- Wiring criterion review or alignment flows.
- Debugging Gate status, reject-and-regen, or reviewflow state transitions.

---

## Core concepts

| Concept | strop / app |
|---------|-------------|
| **Gate** | `humanreview.Gate` — `Start`, `RecordAlignment`, `ResetAlignment`, `ResetRejected`, `SetStatus` |
| **Reviewflow engine** | `reviewflow` — handler table, max-iteration guard, live states |
| **Ports** | App implements `Prompter`, `Generator`, `Session` |
| **Job / Step** | App registers opaque strings at startup (`jobpack`) |
| **FeedbackNormalizer** | `PassthroughNormalizer` or inject formatter-backed normalizer |
| **ScoreProposer** | Optional adapter (e.g. DSPy CoT module) |

`RecordAlignment` does **not** change evaluation status by itself.

---

## Reject-and-regenerate sequence

1. Record disagree on the Gate.
2. Reject the current artifact.
3. Normalize the comment (`FeedbackNormalizer`).
4. Regenerate with `regenerate.RegenerateOptions{Force: true, Message}`.
5. Call `Gate.Start` **only if** a new version exists (resets `rejected` → `in_progress`).
6. On regen failure or max versions: leave `StatusRejected`.

Do not duplicate this sequence in ad-hoc CLI flags — use Gate + shared helpers (`BuildStoredFeedbackForRejection`, `ExtractStructuredFeedbackFromStored`).

---

## Reviewflow engine

**Live states:** Init, Generation, Alignment, Regeneration, FinalizeCriteria, Completion.

**Terminal states:** Exit, Rejection, Done — engine stops without running those handlers.

**Setup:**

```go
reviewflow.RegisterDefaultHandlers(engine)
engine.SetPrompter(appPrompter)
engine.SetGenerator(appGenerator)
engine.SetSession(appSession)
engine.SetGate(gate)
engine.SetFeedbackNormalizer(normalizer)
```

**App-owned:** Postgres store adapter, learning artifacts, Meilisearch demos (if used), pterm menus.

**Safety:** Enforce max state transitions; return all persistence and session refresh errors — no silent failures.

---

## Regeneration mode UX

When the human disagrees, offer:

1. **Simple regeneration** first (feedback only).
2. **Research / comprehensive** second.

Pass a single flag into the service layer (`skipResearch` / `useResearch`) — do not embed service logic in the flow engine.

---

## Job registry

At startup, register:

- Ordered list of reviewable jobs.
- Display names for prompts and option parsing.
- Criterion IDs aligned with in-loop evaluators (see `strop-pipeline-pattern` §3).

Use one delimiter constant for option strings with status suffixes (e.g. `" ["`) so build and parse stay in sync.

---

## Dependency interface (app pattern)

Keep reviewflow decoupled from the container via a single deps interface:

- **Repos:** human review repo, root-entity repo, per-job content repos.
- **Services:** human review service, per-job processing services for regeneration.
- **Config:** app config, feedback client, optional context regenerator factory.

Pipeline `reviewflow/` package implements handlers; container compat implements deps.

---

## Checklist

- [ ] Job/step constants registered at container startup.
- [ ] Gate wired; reject-and-regen follows strop sequence.
- [ ] Reviewflow ports implemented (no reimplemented state machine in app).
- [ ] Criterion list matches evaluator `CriterionIDs`.
- [ ] Regeneration mode: simple first, research second.
- [ ] Errors from persistence and session refresh returned.
- [ ] Logger for logs; TUI library only for interactive UX.

---

## What strop does not include

- Product prompts or criterion copy.
- Postgres + Meilisearch learning store (app adapter).
- pterm classroom / browser opener UX.
- Pipeline-specific CLI command trees.

Load a **project overlay** skill if the consumer documents sayings/YouTube-specific review paths.
