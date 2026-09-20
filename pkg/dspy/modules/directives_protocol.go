package modules

// DirectivesAckField is the leading structured-attention output (not reader-facing content).
const DirectivesAckField = "directives_ack"

// DirectivesProtocol is prepended to generator instructions when using DirectivesCoT.
// It forces commitment before task fields — replacement for stock ChainOfThought rationale.
const DirectivesProtocol = `=== STRUCTURED ATTENTION (MANDATORY — directives_ack field first) ===

Before writing any task output field, think step by step inside <directives_ack> only.

In <directives_ack>, cover these steps in order:
1. Instructions: restate the mandatory rules from the persona and task instruction below (what you must and must not do).
2. Objectives: state VOICE (who you are on this job), MUST (non-negotiable outcome for this pass), and ANTI_PATTERN (main failure mode to refuse) — drawn from OBJECTIVES and the system prompt.
3. Plan: list the ordered steps you will take to produce the task outputs from the supplied inputs only.
4. Attention check: name the highest-risk failure for this task and how you will avoid it.

Then fill every remaining required XML task field from the signature.

Rules:
- directives_ack is a discipline field only; it does not replace task outputs.
- Do not skip directives_ack even when the answer is short.
- Do not put task answers inside directives_ack; put them only in the signature task fields.
- Do not invent a separate rationale or reasoning field unless the signature explicitly requires a task field named rationale (e.g. structured composition plan).
- Prefer short, numbered lines in directives_ack; keep it under ~120 words.
- If later task fields drift from VOICE/MUST, revise those fields — do not weaken directives_ack to match weak outputs.`
