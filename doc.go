// Package strop is a toolkit for tempering LLM outputs: evaluate, refine, and
// gate until they pass. It is not a prompt-authoring SDK.
//
// Boundary rules:
//  1. No imports of product app internal packages.
//  2. No product-specific strings (job names, criterion copy).
//  3. Dependencies use small interfaces and strop-owned options structs.
//  4. App adapters (logger, config mapping) live in the consuming app.
//
// Included: regenerate, imageread, log, refinement, streaming, runreport,
// agentsession (one directory per short-lived agent conversation under an
// app-injected Root), orchestration (including NewFieldWalkStrategy, NewSectionWalkStrategy,
// DocumentArcDefinition, DocumentSectionDefinition), dspy (ProviderConfig, XML
// structured output, factories, registry, JobRunner, workflow, modules,
// generic validation, tracing), evaluation (criteria engine + generic
// rubrics; product packs register in the app; RoleInfo uses EvaluatorKey and
// ConsolidatorKey; ExpertKey identifies feedback-analysis experts), humanreview
// (types, maths, builder interfaces, job→step registry, learning pack registry,
// approval Gate, FeedbackNormalizer, stored-feedback helpers, ScoreProposer,
// LearningService/LearningStore, ItemObjectiveStore; product jobpacks register
// in the app), humanreview/reviewflow (engine + Prompter/Generator/Session/Learner
// ports), jobskip (Store + Restore) with Labeler/Selector ports; pending-list
// exclusion is app SQL.
//
// Not included: learning Postgres/Meilisearch adapters, pterm classroom UX,
// product document-arc / document-section recipes (strop types are ready; app
// runners per pipeline).
//
// Second-app bootstrap: map YAML to ProviderConfig and the app logger to
// strop/log, create an empty registry, then LLM factory → interceptor
// setup → generator/evaluator/feedback factories, register validators (and
// optional structured-output hooks), register modules, run JobRunner.
package strop
