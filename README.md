# strop

Toolkit for tempering LLM outputs: evaluate, refine, and gate until they pass. Not a prompt-authoring SDK.

Module: `github.com/behaviorengineering/strop`. Apps map config and logger at the boundary, then run generate → evaluate → optional Gate reject-and-regen.

## Import path

```go
import (
    stropdspy "github.com/behaviorengineering/strop/dspy"
    "github.com/behaviorengineering/strop/dspy/factory"
    "github.com/behaviorengineering/strop/dspy/registry"
    "github.com/behaviorengineering/strop/dspy/runner"
    "github.com/behaviorengineering/strop/orchestration"
    "github.com/behaviorengineering/strop/humanreview"
    "github.com/behaviorengineering/strop/humanreview/reviewflow"
    "github.com/behaviorengineering/strop/jobskip"
)
```

## Packages

| Package | Role |
|---------|------|
| `dspy` | ProviderConfig, generators, chained evaluators, field helpers |
| `dspy/factory` | LLM, generator, evaluator, feedback factories and interceptor wiring |
| `dspy/registry` | Module registry (generators, workflows, formatters) |
| `dspy/runner` | JobRunner (generate + evaluate) |
| `dspy/workflow` | Parallel evaluation workflow |
| `dspy/modules` | DirectivesCoT / Predict helpers |
| `dspy/structured_output` | XML parser and interceptors |
| `dspy/validation` | Generic mandatory-field / token / language validators |
| `dspy/tracing` | OpenInference module interceptor |
| `orchestration` | Refinement, per-item, composition loops, `NewFieldWalkStrategy`, `NewSectionWalkStrategy`, `NewPhaseWalkStrategy`, `DocumentArcDefinition`, `DocumentSectionDefinition` |
| `refinement` | Versioning, stopping, self-healing policy |
| `regenerate` | Force / feedback options for re-runs |
| `streaming` | Inference event channel types, StreamHandler, Actor constructors |
| `runreport` | JSON execution traces |
| `imageread` | Image load + visual brief helpers |
| `log` | Minimal logger interface for strop packages |
| `evaluation` | Aggregation types, criterion registry/prompt builder, typed keys |
| `humanreview` | Gate, FeedbackNormalizer, ScoreProposer, stored-feedback helpers, LearningService / LearningStore |
| `humanreview/reviewflow` | pterm-free engine, live states, Prompter / Generator / Session ports |
| `jobskip` | Per-job generate-queue skip Store + Restore (Labeler / Selector ports) |

## Boundary rules

1. No imports of this repo’s `internal/` packages.
2. No product-specific prompts, job enums, or criterion *copy* (IDs for product criteria may live in strop; rubrics register via app packs).
3. App code maps its config/logger into strop types at the boundary.

## Second-app bootstrap

Map YAML and logger at the edge, then follow this order (same as this repo’s SharedContainer):

1. Map provider YAML into `dspy.ProviderConfig` (this app uses `AIProviderConfig.ToStrop()` (or equivalent)).
2. Adapt the app logger to `strop/log.Logger` (consumer adapts its logger).
3. Create an empty `registry.ModuleRegistry`.
4. Create `factory.LLMFactory`, then `factory.InterceptorSetup` (inject tracing’s `OpenInferenceModuleInterceptor` to avoid a cycle), then configurator + generator/evaluator/feedback factories.
5. Register output validators by module display name (`RegisterOutputValidator` / `RegisterMandatoryFields`) before `CreateGenerator`.
6. Optionally `SetStructuredOutputHooks` for product parse/format behaviour (this app registers PostGenerator phase XML here).
7. Register generators, evaluators, workflows, and (optional) a feedback formatter.
8. Run jobs through `runner.JobRunner` (`Generate` then `EvaluateWorkflow`).

Default validation is `ValidateMandatoryFields` from the signature when no extra validator is registered.

Product-only in this app (do not copy unless you need the same jobs): `internal/dspy/clients` (research, prose polish, language fixer), PostGenerator validators, sayings/YouTube job packs.

## Evaluation packs

`evaluation/criteria.NewCriterionRegistry()` and `DefaultRegistry()` ship portable process/quality rubrics only. Product rubrics register at container startup:

- `internal/pipelines/sayings/evaluation/criteriapack`
- `internal/pipelines/youtube/evaluation/criteriapack`

## Human-review packs

`humanreview` ships types, maths, an injectable job→step map, an approval **Gate** (`Start`, `RecordAlignment`, `ResetAlignment`, `ResetRejected`, `SetStatus`), reject-and-regenerate helpers (`FeedbackNormalizer`, `PassthroughNormalizer`, `RegenOptionsFromComment`), stored-feedback string helpers (`BuildStoredFeedbackForRejection`, `ExtractStructuredFeedbackFromStored`), and **ScoreProposer** (`Propose`). `RecordAlignment` does not change evaluation status. Product Job/Step constants register at container startup:

- `internal/pipelines/sayings/humanreview/jobpack`
- `internal/pipelines/youtube/humanreview/jobpack`

Reject-and-regenerate sequence: record disagree on the Gate, reject the current artifact, normalize the comment, regenerate with `regenerate.RegenerateOptions{Force: true, Message}`, then `Gate.Start` only if a new version exists (resets `rejected` → `in_progress`). Leave `StatusRejected` on regen failure or max versions.

App-owned: Postgres `Store` adapter, DSPy CoT `ScoreProposer` adapter (`NewModuleScoreProposer`), Postgres learning artifacts + Meilisearch demos, and the sayings pterm classroom (implements reviewflow ports). After a successful post-disagree regen, reviewflow calls `Gate.Start` (rejected → in_progress). YouTube dedicated `review <job> --reject` always regenerates via Gate only (no reviewflow engine); it uses the shared `feedback_analysis` formatter when registered, otherwise passthrough. Per-item jobs accept `--chapters` or `--ideas`. `review job <job_name> <video_uuid>` writes a Gate tape for any registered YouTube generation job (tape-only except the dedicated reject-and-regen commands). Maths-based `ProposeAllCriterionScoresFromHistory` does not call ScoreProposer.

## Learning interface

`humanreview.LearningService` (`StoreLearning`, `GetExamplesForGeneration`, merge/objective helpers) and optional `LearningStore` (CRUD + `GetApprovedByJobStep`) use portable `LearningArtifact` (no embedding field). JobRunner still takes the thin `LearningServiceForGeneration` (string job/step) so `dspy/runner` does not depend on extra types.

A second app implements `LearningStore` or skips demos. This app indexes approved generator examples in Meilisearch for near/contrast retrieval; merge identity uses distinctive-move text in Postgres.

## Composition strategies

Both implement `CompositionStrategy` for `RunCompositionLoop` (ordered phases, per-phase retry, lock on pass).

### Field-walk (`NewFieldWalkStrategy`)

One owned field per phase (phase ID = field key). Strop decides pass/fail: `MinPassScore`, non-empty output, empty source auto-pass, version feedback applied once. Prior fields only are locked. Default result: `FieldWalkState` + `evaluation.AggregateLabeledEvals`.

App supplies `FieldPhaseRunner`, phase defs, `MinPassScore`, optional `SourceText`, and seed draft.

**Use when** you need low-level field-walk with a flat `map[string]string` draft and no typed codec. Prefer **`NewSectionWalkStrategy`** when refining ordered sections on a typed document (polish, translation).

### Section-walk (`NewSectionWalkStrategy`)

Typed field-walk over **`DocumentSectionDefinition`**: one section ID per phase, strop pass/fail (`MinPassScore`, non-empty output, empty source auto-pass, version feedback once). Prior sections only are locked. Default result: `SectionWalkState[T]` + aggregated evals.

App supplies `SectionFieldRunner`, `SectionCodec[T]` (`ToMap` / `FromMap`, optional `SourceText`), section recipe, seed draft, and optional `EmptyResultErr`.

**Use when** refining ordered string sections on a typed draft — polish, translation, future locales (e.g. sayings post via `internal/pipelines/sayings/services/postsection` wrapping `SayingsLinkedInPostSections`).

### Phase-walk (`NewPhaseWalkStrategy`)

Multi-field phases: `PhaseWalkOwnedFields` maps each `PhaseID` to the field keys that phase may write. On pass, only owned keys merge into the draft; prior phases’ owned non-empty fields are locked (`LockedOutput`). Failed attempt output is wired to the next retry (`PreviousFailedOutput` + feedback). Set **`MergeOnFail: true`** when the runner merges generator output into the draft before evaluation (failed attempts still update owned fields in the strop draft). The app runner decides pass/fail, score, and feedback per phase; the strategy handles draft lifecycle and retry wiring.

**Finalize** is pluggable (`PhaseWalkFinalize`). `nil` → default `PhaseWalkState` + aggregate passed-phase evals by phase order. Return a custom `CompositionResult` for typed output (e.g. assemble a struct instead of `map[string]string`).

App supplies `PhaseWalkRunner`, phase defs, `OwnedFields`, optional `Seed`/`Version`, and optional `Finalize`.

**Use when** a phase writes several fields together, pass logic is phase-specific, or the final artifact is not a flat string map (e.g. sayings post skim → warmth → depth → teaser in `internal/pipelines/sayings/services/post/composition.go`).

### Field-walk vs phase-walk

| | Field-walk | Phase-walk |
|---|------------|------------|
| Fields per phase | One (ID = key) | Many (`OwnedFields`) |
| Pass/fail | Strop (`MinPassScore` + non-empty) | App runner |
| Empty source | Auto-pass | Runner decides |
| Version feedback | Once, strop-managed | Runner receives `PreviousFeedback` |
| Final result | Always aggregated eval + draft map | `Finalize` or default aggregate |
| Typical shape | Per-field polish on prose sections | Multi-field arc with scoped eval per phase |

**Do not use PhaseWalk for section jobs.** Post polish, post translation, and other per-section walks belong on **`NewSectionWalkStrategy`** (or `NewFieldWalkStrategy` for flat maps) — e.g. `postsection.NewStrategy`. Reserve PhaseWalk for multi-field phases such as the sayings post arc.

### Document section (`DocumentSectionDefinition`)

Portable recipe for section refinement: section order, `MaxAttempts`, and `MinPassScore`. Helpers wire into section-walk:

- `PhaseDefs()` → field-walk phases
- `MinPass()` → field-walk gate score
- `LockedSectionsBefore(sectionID, draft)` → prior-section locks for runners

**Strop does not include:** job clients, prose isolation, or section-specific validators. Apps define a `DocumentSectionDefinition`, implement `SectionFieldRunner`, and optionally a typed `SectionCodec`.

### Document arc (`DocumentArcDefinition`)

Portable recipe for phased multi-field assembly: phase order, IDs, `ActiveOutputFields`, `MaxAttempts`, and `MinPassScore`. Helpers wire into PhaseWalk:

- `OrchestrationPhases()` → `PhaseWalkConfig.Phases`
- `PhaseWalkOwnedFields()` → `PhaseWalkConfig.OwnedFields`
- `LockedOutputFieldsBefore(phaseID)` → prior-phase locks for runners
- `PreviousVersionDisplayFields(phaseID, exclude...)` → regen reference text

**Strop does not include:** generator/evaluator prompt text, DSPy modules, domain alignment checks, or unstructured feedback filters. Apps define a `DocumentArcDefinition` value, attach prompts via a local instruction provider, implement `PhaseWalkRunner`, and optionally a feedback scope plugin.

**Decision tree**

| Shape | Use |
|-------|-----|
| One string field per step, strop pass/fail, flat map | `NewFieldWalkStrategy` |
| One section per step, strop pass/fail, typed draft | `DocumentSectionDefinition` + `NewSectionWalkStrategy` |
| Multi-field phases, app pass/fail | `DocumentArcDefinition` + `NewPhaseWalkStrategy` |
| Single generator call | `JobRunner` only — no arc |

Reference (assembly): sayings LinkedIn post (`internal/pipelines/sayings/clients/post_arc.go` + `services/post/composition.go`). Second document arc: `docs/features/document-arc-second-pipeline.md`.

Reference (refinement): sayings post polish/translation (`internal/pipelines/sayings/clients/post_sections.go` + `services/postsection`). Second pipeline section walk: `docs/features/document-section-second-pipeline.md`.

## Reviewflow engine

`humanreview/reviewflow` is a pterm-free engine: handler table, max-iteration guard, terminals Exit / Rejection / Done (those next-states stop **without** running those handlers). Live states: Init, Generation, Alignment, Regeneration, FinalizeCriteria, Completion. Inject `Prompter`, `Generator`, `Session`, `*Gate`, and `FeedbackNormalizer`. `RegisterDefaultHandlers` wires the live graph.

A second app implements the three ports and optionally uses default handlers. Sayings and YouTube both use a pipeline-specific classroom under `internal/cli/pipelines/<pipeline>/reviewflow/` (alignment, disagree/regenerate, criterion finalize). YouTube also keeps flag-based review (`pipelines youtube review --approve` / `--reject`) for scripts and CI; interactive `pipelines youtube review execute` and analyze generate/regenerate share the same Gate store.

## Job skip

`jobskip` hides a root from one generator job’s pending generate list, lists the skip archive, and restores a selected root. Strop is pterm-free: `Store` (`Skip`, `Unskip`, `IsSkipped`, `List`), `Labeler`, `Selector`, and `Restore`. Job keys are opaque strings.

Pending-list exclusion is **app SQL** (omit skipped roots). This app’s adapter is Postgres `youtube.video_job_skips`; CLI cobra/pterm lives in `internal/cli/flow/jobskip`. YouTube first consumer: `pipelines youtube analyze <job> skip|unskip|skipped`.

## Not included (yet)

Parked until a second consumer needs them without this repo:

- Learning **Postgres + Meilisearch** (artifact store, near/contrast demos, distinctive-move merge)
- pterm classroom UX (alignment menus, browser opener, learning-artifact TUI)
- Extra document-arc recipes beyond the sayings LinkedIn post reference (strop `DocumentArcDefinition` is ready; app prompts/runners are per pipeline)
- Extra document-section recipes beyond sayings post prose (strop `DocumentSectionDefinition` + `NewSectionWalkStrategy` are ready; app runners are per pipeline)
