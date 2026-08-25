package dspy

import (
	"strings"

	"github.com/behaviorengineering/strop/evaluation"
)

// DSPy framework contract field names - used across multiple pipelines.
// Pipeline-specific generator fields (e.g., translation fields, explanation fields)
// belong in their respective pipeline packages.
const (
	// Generator contract fields (used by both sayings and YouTube pipelines).
	FieldGeneratorInput     = "generator_input"
	FieldGeneratorOutput    = "generator_output"
	FieldGeneratorRationale = "generator_rationale"
	FieldIterationVersion   = "iterationVersion"
	FieldOriginalText       = "original_text" // Common artifact/generator input key
	FieldRetrievedGuides    = "retrieved_guides" // Per-call transferable principles from learning store

	// Evaluator output fields.
	FieldScore           = "score"
	FieldCriterionScores = "criterion_scores"
	FieldFeedback        = "feedback"
	FieldRationale       = "rationale"
	FieldDirectivesAck   = "directives_ack"

	// Consolidator input/output fields.
	FieldIndividualFeedbacks  = "individual_feedbacks"
	FieldAgentScores          = "agent_scores"
	FieldWeightedScore        = "weighted_score"
	FieldConsolidatedFeedback = "consolidated_feedback"

	// RoleContextConsolidator is the registry key for the shared context consolidator.
	RoleContextConsolidator evaluation.ConsolidatorKey = "context_consolidator"
)

// generatorRationaleActionChainRules is the contract for generator rationale fields:
// objective recitation first, then short action-chain bullets (not introspective essays).
const generatorRationaleActionChainRules = "Start with exactly three labeled lines — VOICE: (who you are on this job), MUST: (non-negotiable outcome for this pass), ANTI_PATTERN: (main failure mode to refuse) — drawn from OBJECTIVES and the system prompt. Then add concise action-chain bullets or short numbered steps (what you did for this task), not introspective narration (avoid \"I read...\", \"I considered...\", and long reasoning essays). Unless the system prompt defines a longer structured rationale plan, cap post-objective lines at 5. Prefer imperative or past-tense actions."

// evaluatorRationaleActionChainRules is the contract for evaluator and human-review rationale fields.
// Evaluators must keep rationale brief; full checklists belong in feedback, not rationale.
const evaluatorRationaleActionChainRules = "Use a compact numbered list (3–5 lines max) of evaluation steps only — not a walkthrough of every rubric rule. Do NOT use VOICE/MUST/ANTI_PATTERN (generators only). Do NOT put the [✓]/[ ] checklist in rationale; put it in feedback. Never use angle brackets (< or >) inside rationale — reference fields as plain names (description, tldr, original_text). Avoid introspective narration."

// RationaleDescription is a short documentation fragment; use RationaleDescriptionWithContext on generator output fields.
const RationaleDescription = "AI-to-AI rationale (VOICE/MUST/ANTI_PATTERN objective recitation, then action-chain; plain text, no tags or code fences in rationale text)."

// EvaluatorRationaleDescription is a short documentation fragment for evaluator rationale fields.
const EvaluatorRationaleDescription = "AI-to-AI evaluator rationale (brief action-chain only; plain text, no angle brackets or code fences)."

// RationaleDescriptionWithContext returns the standard rationale output-field description for generators.
// taskFocus names what the bullets should cover (e.g. \"chapter boundary choices\", \"quote selection\").
func RationaleDescriptionWithContext(taskFocus string) string {
	focus := strings.TrimSpace(taskFocus)
	if focus == "" {
		focus = "the structured output you produced"
	}
	return "AI-to-AI rationale for generators and audit logs. " + generatorRationaleActionChainRules + " Focus: " + focus + ". Plain text only. Do not put XML/HTML tags, JSON objects, or code fences inside the rationale text."
}

// EvaluatorRationaleDescriptionWithContext returns the rationale field description for chained evaluators and review modules.
func EvaluatorRationaleDescriptionWithContext(taskFocus string) string {
	focus := strings.TrimSpace(taskFocus)
	if focus == "" {
		focus = "how you evaluated the generator output"
	}
	return EvaluatorRationaleDescription + " " + evaluatorRationaleActionChainRules + " Focus: " + focus + "."
}

// RationaleDescriptionWithExtra appends module-specific constraints (e.g. XML emit order, tighter length caps).
// Use for the rare cases where the standard rationale rules need a sibling (structured XML must close first).
func RationaleDescriptionWithExtra(taskFocus string, extraConstraints string) string {
	base := RationaleDescriptionWithContext(taskFocus)
	ex := strings.TrimSpace(extraConstraints)
	if ex == "" {
		return base
	}
	return base + " " + ex
}
