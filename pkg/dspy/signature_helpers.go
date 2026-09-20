package dspy

import (
	"github.com/XiaoConstantine/dspy-go/pkg/core"
)

// SharedPromptInstructions groups all shared prompt instructions used across generators and evaluators.
// This struct serves as documentation and organization for prompt instruction sets.
type SharedPromptInstructions struct {
	// Formatting instructions (applies to all structured outputs)
	XMLFormatting string

	// Writing style for LLM-consumed output (evaluators whose feedback is consumed by generators)
	LLMWriting         string
	LLMEvaluatorOutput string
	// ChainedFeedbackAnalysisOutputSplit is appended only to the feedback-analysis submodule in
	// CreateChainedEvaluatorModule (after XML + DefaultEvaluatorSignatureFormatter). Same rule as
	// the EXCEPTION inside LLMEvaluatorOutput; repeated here at the end of the prompt for reliability.
	ChainedFeedbackAnalysisOutputSplit string
	// ChainedEvaluatorRationaleCap is appended to both feedback-analysis and score-generation submodules.
	ChainedEvaluatorRationaleCap string

	// Writing style for human-consumed output (generators whose output is read by humans)
	HumanWriting  string
	AntiAIPattern string

	// Feedback instructions
	FeedbackIntegration     string // For generators - how to integrate feedback
	EvaluatorFeedbackFormat string // For evaluators - how to format feedback

	// GeneratorObjectiveRecitation requires generators to restate voice/objectives in rationale before other fields.
	GeneratorObjectiveRecitation string
}

// SharedInstructions contains all shared prompt instructions grouped together.
// This makes it clear which instructions apply to which module types.
var SharedInstructions = SharedPromptInstructions{
	XMLFormatting: `
CRITICAL XML FORMATTING REQUIREMENTS (applies to all structured outputs):
- Output your response in valid XML format as specified in the XML structure instructions
- DO NOT wrap the XML in markdown code blocks (no triple backticks with xml or any other markers)
- DO NOT add any formatting around the XML - output raw XML directly
- Inside each XML tag, output ONLY the value - NO field names, labels, prefixes, or explanations
- Do NOT include text like "fieldname:", "score:", "rationale:", "feedback:" inside the XML tags
- Example: <score>9.0</score> is correct. <score>score: 9.0</score> is WRONG
- Example: <rationale>Your reasoning here</rationale> is correct. <rationale>rationale: Your reasoning here</rationale> is WRONG
- WRONG: Wrapping XML in markdown code blocks (do not use markdown formatting)
- CORRECT: <response>...</response> (output raw XML directly, no markdown)
- The XML structure will be provided separately - follow it exactly

CRITICAL: XML TAG WELL-FORMEDNESS (applies to all structured outputs):
- Every closing tag must match its opening tag name exactly (same spelling, same namespace). Wrong: <foo>...</bar>. Right: <foo>...</foo>.
- Do not reuse a closing tag from a sibling field (e.g. closing <chapter_summaries> with </chapter_titles> invalidates the whole document).
- Nest tags properly: close the most recently opened element before an outer one (inner first, then outer).
- Before sending, scan your XML once for mismatched or cross-named close tags.

CRITICAL: FIELD CONTENT REQUIREMENTS (applies to ALL structured output fields):
- Each field must contain ONLY its content value - NO field labels, prefixes, headers, or field names
- DO NOT start field content with the field name followed by a colon (e.g., "rationale:", "analysis:", "score:")
- DO NOT include field labels anywhere in the field content text
- Field content should start directly with the actual content, not with a label
- Examples of WRONG field content:
  * "rationale: Your reasoning here"
  * "Analysis: Comprehensive analysis..."
  * "analysis: Comprehensive analysis..."
  * "score: 2.0"
- Examples of CORRECT field content:
  * "Your reasoning here" (for rationale field)
  * "Comprehensive analysis..." (for analysis field)
  * "2.0" (for score field)
- The field name is already specified by the XML tag - do not repeat it in the content
- This rule applies to ALL fields: analysis, rationale, evidence, proposed_score, and any other structured output fields

CRITICAL: For map/object fields (e.g., criterion_scores):
- DO NOT output JSON strings inside XML tags (e.g., <criterion_scores>{"key": "value"}</criterion_scores> is WRONG)
- DO output nested XML elements for each key-value pair (e.g., <criterion_scores><key>value</key></criterion_scores> is CORRECT)
- Example for criterion_scores map:
  WRONG: <criterion_scores>{"instruction_compliance": 2.0, "output_quality": 1.5}</criterion_scores>
  CORRECT: <criterion_scores><instruction_compliance>2.0</instruction_compliance><output_quality>1.5</output_quality></criterion_scores>
- Each key becomes a nested XML element with the value as its content
- Use the exact key names as specified in the field description (e.g., use "instruction_compliance" not "Instruction Compliance")`,

	LLMEvaluatorOutput: `
CRITICAL: OUTPUT EFFICIENCY FOR LLM CONSUMPTION (applies to all evaluator outputs):

Your output will be consumed by another LLM (generator or downstream evaluator). Optimize for token efficiency and machine parsing.

CONCISENESS REQUIREMENTS:
- Be CONCISE: Remove all conversational filler, redundant explanations, and verbose descriptions
- Focus on ACTIONABLE CONTENT: Include only information needed for downstream processing
- Eliminate REDUNDANCY: Don't repeat the same information in multiple fields
- EXCEPTION (chained evaluator feedback analysis): Downstream modules read the checklist from the feedback field only. You MUST put the full [✓]/[ ] checklist inside <feedback> (never empty). Put structured attention only in <directives_ack>. Do NOT move the checklist into <directives_ack> alone—that leaves the feedback field empty and breaks the pipeline.
- Use DENSE REPRESENTATIONS: Pack maximum information into minimum tokens
- Avoid ELABORATION: Skip "as mentioned above", "in summary", or meta-commentary

STRUCTURE FOR EFFICIENCY:
- Use COMPACT FORMATS: Prefer structured lists over paragraphs where possible
- Use ABBREVIATIONS: Standard abbreviations are acceptable (e.g., "req" for "requirement")
- Use BULLET POINTS: More scannable and token-efficient than prose
- Use REFERENCE SHORTHAND: Reference pipeline history by version number (e.g., "v2") rather than full descriptions
- Extract ONLY ESSENTIAL EVIDENCE: Quote minimal necessary text, not full context

FIELD-SPECIFIC GUIDELINES:
- analysis: Focus on findings, not process. Skip "I examined..." and go straight to "Found: ..."
- directives_ack: Brief structured attention only. Never use < or > inside it. No full rubric walkthrough — findings go in feedback
- rationale: Only when the signature declares it as a task field (e.g. human-review combined output). Prefer directives_ack for chained evaluator steps
- evidence: Quote only the essential excerpt, not surrounding context
- feedback: Use checklist format [✓]/[ ] with brief, actionable items
- proposed_score: Just the number, no explanation (explanation goes in directives_ack / analysis)

TOKEN EFFICIENCY:
- If output will be passed to another LLM module, prioritize brevity over completeness
- For chained modules: optimize for the next module, but never omit or hollow out a required output field to save tokens (see EXCEPTION under CONCISENESS for directives_ack vs feedback)
- Remove all meta-commentary about your own analysis process
- Don't include "I think", "it seems", "appears to be" - state findings directly`,

	ChainedFeedbackAnalysisOutputSplit: `
=== FEEDBACK ANALYSIS: DIRECTIVES_ACK VS FEEDBACK (MANDATORY) ===
- The full evaluator checklist ([✓]/[ ] lines, actionable items) MUST appear inside <feedback> only. <feedback> must never be empty—use at least one line (e.g. "[✓] All criteria met." if nothing fails).
- <directives_ack> is structured attention only (brief instructions → objectives → plan → attention check). Do NOT put the checklist only in <directives_ack> and leave <feedback> empty.
- NEVER use angle brackets (< or >) inside <directives_ack> text — they break XML parsing. Reference fields as plain names (description, tldr, original_text), not as <description> or <original>.
- Downstream score generation reads <feedback>; empty <feedback> invalidates the run.`,

	ChainedEvaluatorRationaleCap: `
=== EVALUATOR DIRECTIVES_ACK CAP (MANDATORY) ===
- <directives_ack> must stay short (prefer under ~120 words) — never hundreds of rubric walkthrough items.
- Never use angle brackets (< or >) inside <directives_ack>; reference fields as plain names only.
- Put findings and the [✓]/[ ] checklist in <feedback> (or scores in <criterion_scores>), not in <directives_ack>.`,

	LLMWriting: `WRITING STYLE REQUIREMENTS FOR LLM-CONSUMED OUTPUT (CRITICAL):

Your output will be consumed by another LLM (generator), not humans. Optimize for machine parsing and action execution.

STRUCTURE AND FORMATTING:
- Use STRUCTURED, UNIFORM FORMAT - consistent patterns that LLMs can parse reliably
- Use EXPLICIT, DIRECTIVE LANGUAGE - state requirements as clear commands, not suggestions
- Use UNIFORM MARKERS - consistent symbols and patterns (e.g., [✓], [ ], -, numbered lists)
- Use CLEAR HIERARCHY - organize information in predictable, nested structures
- Use EXPLICIT ACTION ITEMS - each instruction must be actionable and unambiguous

LANGUAGE STYLE:
- Use DIRECTIVE VOICE - "MUST", "REQUIRED", "DO NOT" instead of "should", "consider", "might"
- Use EXPLICIT TERMINOLOGY - precise terms, avoid ambiguity or interpretation
- Use CONCISE, PARSABLE TEXT - remove conversational filler, focus on actionable content
- Use CONSISTENT TERMINOLOGY - same words for same concepts throughout

FEEDBACK STRUCTURE REQUIREMENTS:
- FEEDBACK CHECKLIST: Use exact format [✓] for fixed items, [ ] for pending items
- CONSTRAINTS: Format as "MUST follow [rule]" with concrete examples
- ACTIONABLE STEPS: Number each step, include specific examples from generator_output
- SCORE BREAKDOWN: Format as "{component}: {points}/2 - [explanation]"`,

	HumanWriting: `WRITING STYLE REQUIREMENTS (CRITICAL):

- Use STANDARD SENTENCE CASING - start every sentence with an uppercase letter; do not begin prose with a lowercase letter (exceptions only for proper names or spellings that require it)
- Use ACTIVE VOICE throughout - write directly and clearly
- Present CLEAR IDEAS - one main point per sentence, avoid complex nested thoughts
- Use PLAIN, DIRECT ENGLISH - simple, everyday words that any professional can understand
- Be CONCISE - avoid unnecessary words or verbose explanations
- NO JARGON - avoid academic or technical language unless absolutely necessary
- Write as if explaining to a colleague over coffee - friendly but professional`,

	AntiAIPattern: `ANTI-AI PATTERN REQUIREMENTS (CRITICAL - applies to all human-facing content):

FORBIDDEN AI META-COMMENTARY PATTERNS:
- NO framing devices: "That's the vibe of", "That's the essence of", "Think of it as", "It's a way to", "It's perfect for", "It's about"
- NO explanatory scaffolding: "Here's what that means", "In other words", "What this means is", "The idea is"
- NO rhetorical questions as structure: Don't use questions to set up explanations
- NO hedging patterns: "essentially", "basically", "generally speaking", "in essence"
- NO instructional voice: "you can think of", "you might say", "imagine that"

WRITE DIRECTLY INSTEAD:
- State things directly, don't frame them
- Skip the setup, jump into the explanation
- Use declarative statements, not rhetorical scaffolding
- Example: BAD: "That's the vibe of '¿Qué es una raya más para un tigre?'" → GOOD: Just start with "You're dealing with so much already..."
- Example: BAD: "Think of it as saying you're unfazed" → GOOD: "You're completely unfazed"
- Example: BAD: "It's perfect for when an extra task feels insignificant" → GOOD: "When an extra task lands, you just shrug it off"
- Example: BAD: "That's the essence of the saying" → GOOD: Just explain the saying directly

OTHER AI TELLS TO AVOID:
- NO therapist voice: "navigate", "validate", "social boundary", "reinforce bonds"
- NO marketing fluff: "fantastic way to", "wonderfully absurd", "leverage"
- NO enthusiastic adjectives: "wonderful", "marvelous", "amazing", "incredible"
- NO em-dashes as conversational devices
- NO awkward familiarity: "buddy", "pal", "friend"

NATURAL WRITING PATTERNS:
- State facts directly without framing
- Use confident, declarative sentences
- Skip the meta-commentary and get to the point
- Sound like a knowledgeable human explaining something to a friend, not an AI assistant explaining to a user`,

	FeedbackIntegration: `=== FEEDBACK INTEGRATION ===

If 'previous_feedback' exists:
- Read FEEDBACK CHECKLIST - address all unchecked [ ] items
- **CRITICAL: In Simple Regeneration Mode (when feedback contains [User] tags):**
  - **ONLY address items tagged [User]** - These are the user's explicit requests (matches evaluator tag format like [Cultural Anthropologist])
  - **IGNORE items tagged with evaluator names** (e.g., [Cultural Anthropologist], [Content Strategist]) - These are evaluator-found issues that should be addressed in full evaluation mode
  - **Preserve ALL fields NOT mentioned in [User] items** from previous_output
- **In Full Evaluation Mode (when feedback does NOT contain [User] tags):**
  - Address all unchecked [ ] items (both user-requested and evaluator-found)
- PRESERVE fields marked "PRESERVE field_name" - keep exactly from previous_output
- Preserve fields NOT mentioned in feedback from previous_output
- Only modify fields explicitly mentioned in feedback (except PRESERVE fields)
- If CONSTRAINTS FOR NEXT VERSION section exists: Follow those constraints
- If ACTIONABLE STEPS section exists: Implement those steps
- **CRITICAL**: Feedback takes priority over base instructions - if feedback contradicts base instructions, follow the feedback

If 'previous_output' exists:
- Empty (version = 1): Generate all fields from scratch
- Populated (version > 1): Preserve unchanged fields, only modify what feedback requests`,

	GeneratorObjectiveRecitation: `=== OBJECTIVE RECITATION (MANDATORY — rationale field first) ===
Before any other output field, open <rationale> with three labeled lines that restate this job's voice and success criteria from OBJECTIVES and the system prompt — not scene allocation or task mechanics alone:
- VOICE: who you are writing as on this job (e.g. street-wisdom mentor, evidence inventory builder, faithful translator).
- MUST: the non-negotiable outcome for this pass (what must be true in the outputs that follow).
- ANTI_PATTERN: the main failure mode to refuse (e.g. CEO manual / HR brochure, academic smoothness without felt cost, inventing facts).
Then continue with job-specific planning (action-chain bullets, structured labels, phase_plan) before reader-facing or downstream fields.
If later prose drifts from VOICE/MUST, revise prose — do not weaken rationale to match weak prose.`,

	EvaluatorFeedbackFormat: `=== USER INTENT AND [BYPASS] (CRITICAL) ===
generator_input includes previous_feedback (the instructions the generator followed). It may contain explicit user requests: [User] tags, PRESERVE instructions, or clear wording like "second: X", "third: Y", "use literal_translation".
- If you would flag an element that was explicitly requested or preserved by the user, do NOT drop your observation—the user may still want to see it—but tag it [BYPASS] so the consolidator will NOT send it to the generator. User choice overrides your criterion for that element.
- Format for bypass: [ ] [Your Evaluator Name] [BYPASS] Your observation (user chose this; for their awareness only).
- Only use [BYPASS] when the element in question is clearly one the user asked for or approved in previous_feedback. For all other items, use your normal tag (no [BYPASS]).

=== FEEDBACK FORMAT ===

Provide feedback in this format:
- [✓] Item 1 (FIXED in v{version} - quote the improvement) - ONLY if previous_feedback exists
- [ ] Item 2 (still needs work - quote where issue exists in generator_output)
- [ ] Item 3 (NEW - describe new issue found)
- [ ] [Your Name] [BYPASS] Observation (user chose this; for awareness only) - when your feedback would contradict an explicit user request

Tag all your items with your evaluator name:
- [ ] [Your Evaluator Name] Your specific feedback text

Remember: Focus on CONTENT quality only. Reference specific phrases from generator_output. Be instructional, not just evaluative. Use [BYPASS] so the consolidator does not bring user-chosen elements back to the generator.`,
}

// NewInputField creates a field for input without prefixes.
// Since we use XML interceptors for structured output, prefixes on inputs are also unnecessary
// and can create inconsistent formatting in prompts.
func NewInputField(name string, opts ...core.FieldOption) core.Field {
	opts = append([]core.FieldOption{core.WithNoPrefix()}, opts...)
	return core.NewField(name, opts...)
}

// NewOutputField creates a field for XML-structured output without prefixes.
// Since all outputs use XML interceptors, prefixes are not needed and can confuse the LLM
// into including them inside XML tags (e.g., <score>score: 9.0</score> instead of <score>9.0</score>).
// The XML parser will strip prefixes anyway, but removing them prevents LLM confusion.
func NewOutputField(name string, opts ...core.FieldOption) core.Field {
	opts = append([]core.FieldOption{core.WithNoPrefix()}, opts...)
	return core.NewField(name, opts...)
}

// WithXMLFormatting appends SharedInstructions.XMLFormatting to the signature instruction.
// Use for any dspy module so output structure rules (valid XML, no prefixes, etc.) are always present.
func WithXMLFormatting(signature core.Signature) core.Signature {
	if signature.Instruction == "" {
		return signature.WithInstruction(SharedInstructions.XMLFormatting)
	}
	combinedInstruction := signature.Instruction + "\n\n" + SharedInstructions.XMLFormatting
	return signature.WithInstruction(combinedInstruction)
}

// WithLLMInstructions applies LLM writing-style instructions only (SharedInstructions.LLMWriting).
// Use for evaluators whose feedback will be consumed by Generator LLMs. Compose with WithXMLFormatting for format rules.
func WithLLMInstructions(signature core.Signature) core.Signature {
	if signature.Instruction == "" {
		return signature.WithInstruction(SharedInstructions.LLMWriting)
	}
	combinedInstruction := SharedInstructions.LLMWriting + "\n\n" + signature.Instruction
	return signature.WithInstruction(combinedInstruction)
}

// WithEvaluatorOutputFormatting applies LLM evaluator output formatting instructions to a signature.
// This ensures evaluator outputs are optimized for downstream LLM consumption.
// Use this for evaluator modules whose outputs will be consumed by other LLM modules (e.g., chained evaluators).
func WithEvaluatorOutputFormatting(signature core.Signature) core.Signature {
	if signature.Instruction == "" {
		return signature.WithInstruction(SharedInstructions.LLMEvaluatorOutput)
	}
	combinedInstruction := SharedInstructions.LLMEvaluatorOutput + "\n\n" + signature.Instruction
	return signature.WithInstruction(combinedInstruction)
}

// WithHumanInstructions prepends SharedInstructions.HumanWriting and AntiAIPattern to the signature instruction.
// Use for generators whose output is read by humans (sayings, YouTube structural jobs, research, language fixer).
func WithHumanInstructions(signature core.Signature) core.Signature {
	combined := SharedInstructions.HumanWriting + "\n\n" + SharedInstructions.AntiAIPattern
	if signature.Instruction == "" {
		return signature.WithInstruction(combined)
	}
	return signature.WithInstruction(combined + "\n\n" + signature.Instruction)
}
