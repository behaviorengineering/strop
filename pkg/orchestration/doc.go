// Package orchestration provides generic loops for multi-step workflows.
//
// Layout: under strop — orchestration is a peer of refinement, streaming, and runreport.
// Orchestration depends on refinement (stopping policy) and strategies; it does not own or nest other packages.
//
// Current loops:
//   - RunRefinementLoop: generate → evaluate → check stop → save or recurse (with optional self-healing).
//   - RunPerItemRefinementLoop*: per-item generate → evaluate → refine, then one save.
//   - RunCompositionLoop: ordered phases with per-phase generate → gate → lock (vertical document assembly).
//   - RunStepPlan: ordered stepplan.Plan with filesystem checkpoints, per-step budgets, and resume.
//
// Strategies:
//   - RefinementStrategy — entity-level version refine.
//   - PerItemRefinementStrategy — N items (chapters, fields) with one save at the end.
//   - CompositionStrategy — phased document build; nest under RefinementStrategy.GenerateAndEvaluate.
//     Phases() may be a fixed recipe or built at runtime (any-length essay = more phases).
//   - FieldWalkStrategy — single-field phases over a string draft (one owned field per phase).
//   - SectionWalkStrategy — typed field-walk over DocumentSectionDefinition (polish / translation).
//   - PhaseWalkStrategy — multi-field phases with owned-field sets, pluggable Finalize, and retry wiring.
//   - StepRunner — host callback for one stepplan.Step (RLM / CoT / other); used by RunStepPlan.
//
// Run execution traces (when run reports are enabled): loops call runreport.StartSession;
// steps are recorded via context-scoped collectors and DSPy middleware (module calls, evaluators,
// warnings, alignment). JSON files land under the configured run-report dir with automatic pruning.
//
// Ambient ACE (optional): when dspy/ace.Manager is on ctx (WithManager), each refinement version
// (and each per-item round) is one ACE trajectory. The loop StartTrajectory / RecordStep /
// EndTrajectory; JobRunner injects LearningsContext into retrieved_guides. CatchInterceptor
// records module Process errors. Host owns NewManager/Close for the session. ACE is not
// runreport and not humanreview.CompositionLearning (few-shot demos). RLM Complete is a later edge.
//
// Pipelines implement RefinementStrategy and call RunRefinementLoop instead of duplicating the recursive loop.
// CompositionStrategy may nest inside each refinement version.
//
// Additional loops for other purposes (e.g. different flow patterns) can be added in this package.
package orchestration
