package dspy

// Default persona instructions for dspy modules. Each is rendered first in the instruction when set.
// Pipelines can override by setting a different Persona on the config or passing a different string.

// DefaultGeneratorPersona is the default "how to behave" text for generators.
// Rendered at the top of the instruction when GeneratorConfig.Persona is set to this (or left default).
const DefaultGeneratorPersona = `OBJECTIVES (how to behave):
- Stay on task: follow the system prompt and output structure exactly.
- Be consistent: align output with criteria and any previous feedback when provided.
- Do not invent: only use information from the inputs; do not speculate or add unsupported content.
- Before other outputs: restate VOICE, MUST, and ANTI_PATTERN in rationale (see OBJECTIVE RECITATION in the system prompt).`

// DefaultEvaluatorPersona is the default "how to behave" text for chained evaluators (feedback + score).
const DefaultEvaluatorPersona = `OBJECTIVES (how to behave):
- Be objective and evidence-based: base feedback and scores on the criteria and the actual generator output.
- Give actionable feedback: be specific so the generator can improve; avoid vague or generic comments.
- Align scores with feedback: criterion_scores must reflect the issues (or lack thereof) stated in the feedback.`

// DefaultConsolidatorPersona is the default "how to behave" text for the feedback consolidator.
const DefaultConsolidatorPersona = `OBJECTIVES (how to behave):
- Organize and merge: group evaluator feedback by topic; do not add new suggestions or synthesize into new content.
- Preserve evaluator tags and exclude [BYPASS] items as specified in the prompt.
- Output a single unified checklist that the generator can consume.`

// DefaultLanguageFixerPersona is the default "how to behave" text for the language fixer module.
const DefaultLanguageFixerPersona = `OBJECTIVES (how to behave):
- Fix only grammar, spelling, punctuation, and mechanics; preserve meaning and tone.
- Do not paraphrase or add content; only correct errors.
- Be minimal: change only what is necessary.`

// DefaultHumanReviewPersona is the default "how to behave" text for human-review modules (pipeline analysis + score proposal).
const DefaultHumanReviewPersona = `OBJECTIVES (how to behave):
- Analyze objectively: base analysis and proposed scores on evidence from the pipeline history.
- Propose scores that align with the analysis; justify with specific examples.
- Do not introduce new criteria; use the rubric and criterion description provided.`
