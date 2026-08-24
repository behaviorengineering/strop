package dspy

import (
	"context"
	"fmt"

	dspymodules 	"github.com/behaviorengineering/strop/dspy/modules"
	"github.com/behaviorengineering/strop/evaluation/criteria"
	"github.com/behaviorengineering/strop/streaming"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
)

// EvaluatorSignatureFormatter applies pipeline-specific formatting to an evaluator signature
// (e.g. LLM instructions, evaluator output format). Pass nil to skip formatting.
type EvaluatorSignatureFormatter func(core.Signature) core.Signature

// DefaultEvaluatorSignatureFormatter returns the shared formatter used by all pipelines
// (evaluator output formatting + LLM writing). XML formatting is always applied in Create* functions, not here.
func DefaultEvaluatorSignatureFormatter() EvaluatorSignatureFormatter {
	return func(s core.Signature) core.Signature {
		return WithLLMInstructions(WithEvaluatorOutputFormatting(s))
	}
}

// DefaultChainedEvaluatorSignature returns the standard signature for chained evaluators (feedback analysis → score generation).
// Inputs: generator_input, generator_output. Outputs: criterion_scores, feedback, directives_ack.
// All evaluators use this chained flow; then consolidation merges feedback from multiple evaluators. No simple one-step evaluators.
func DefaultChainedEvaluatorSignature() core.Signature {
	evalField := func(name, desc string) core.Field {
		return core.NewField(name, core.WithNoPrefix(), core.WithDescription(desc))
	}
	return core.NewSignature(
		[]core.InputField{
			{Field: evalField(FieldGeneratorInput, "What the generator received (context, previous_feedback, version, etc.)")},
			{Field: evalField(FieldGeneratorOutput, "What the generator produced")},
		},
		[]core.OutputField{
			{Field: evalField(FieldCriterionScores, "Map of criterion ID to score (0-10). Use key output_quality for the overall score.")},
			{Field: evalField(FieldFeedback, "Checklist-based instructional feedback for refinement")},
			{Field: evalField(FieldDirectivesAck, "Structured attention from DirectivesCoT (brief evaluation steps)")},
		},
	)
}

// ConsolidatorPromptBase is the full shared prompt for any evaluation-chain consolidator.
// It covers: role, group-by-topic, [BYPASS] exclusion, tag preservation, nested checklist format,
// analysis process, and output. Pipelines append only domain-specific CONSOLIDATE/IGNORE content
// and optional domain-specific examples.
const ConsolidatorPromptBase = `You are a Feedback Consolidator. Your job is to GROUP and ORGANIZE evaluator feedbacks into a unified checklist.

CRITICAL: GROUP evaluator feedbacks by topic - DO NOT synthesize, DO NOT add your own suggestions, just organize what evaluators said.

=== ITEMS TO EXCLUDE FROM GENERATOR (e.g. [BYPASS]) ===
- Evaluators may tag items (e.g. [BYPASS]) when feedback would contradict an explicit user choice or is for user awareness only. Those items must not reach the generator.
- You MUST EXCLUDE all such tagged items from consolidated_feedback. Do not list them in the checklist that goes to the generator. Only include items that are NOT tagged for exclusion.

=== TRACKING EVALUATOR SOURCES (MANDATORY) ===
- **Evaluators already tag their own items** - each evaluator tags their feedback items with their own name (e.g. [EvaluatorName1], [EvaluatorName2])
- **Your job is to PRESERVE these tags** - don't change them, don't infer new ones
- **For FIXED items**: Preserve the tags that evaluators already added. **For NEW items**: Preserve the tags that evaluators already added
- **If multiple evaluators have the same issue**: Merge into ONE item with multiple tags (e.g. [EvaluatorName1, EvaluatorName2])
- **If you infer something not mentioned by any evaluator**: Tag with [Inferred]

**CRITICAL: DO NOT INFER TAGS FROM SCORES**
- High scores do NOT mean all evaluators agree on everything - an evaluator can give a high score but still have specific feedback items
- Tags must be READ from actual feedback items - look for tags in the feedback text itself
- If an evaluator didn't tag an item, don't add their tag - even if they gave a high score
- Only tag with evaluators who EXPLICITLY tagged that specific item - don't assume all evaluators agree just because scores are high

**How to consolidate tags:**
1. READ each evaluator's individual feedback text carefully - look for their tags in the actual feedback items
2. For items marked [✓] FIXED: Preserve the evaluator's tag only if they explicitly tagged it
3. For items marked [ ] (needs work): Preserve the evaluator's tag only if they explicitly tagged it
4. If multiple evaluators flagged the same issue: Merge into one item with all their tags (only if they all explicitly tagged it)
5. If you infer something: Tag with [Inferred]
6. NEVER tag with evaluators who didn't explicitly tag that item - even if they gave high scores

**CRITICAL RULES:**
1. PRESERVE existing tags - evaluators already tagged their items, don't change them
2. READ tags from feedback text - don't infer tags from scores
3. Merge similar items - if 3 evaluators say the same thing AND all tagged it, create ONE merged item with all 3 tags
4. Don't add tags - only preserve tags from feedback text, or use [Inferred] for your own inferences
5. Don't remove tags - if an evaluator tagged an item, preserve that tag
6. Don't infer tags from scores - high scores don't mean all evaluators agree on everything

=== WHAT TO CONSOLIDATE / IGNORE ===
CONSOLIDATE: Content and quality issues identified by evaluators (pipeline-specific list follows in the next section).
IGNORE: Technical formatting (XML, field names, output format); your output format (directives_ack, consolidated_feedback); generator output structure. Focus on content/quality only.

=== ANALYSIS PROCESS ===
1. Read ALL evaluator feedbacks (each has checklist with evaluator name in header)
2. If low scores but no actionable feedback: Infer fixes from score + requirements; create feedback items tagged [Inferred]. (Pipeline may add domain-specific checks.)
3. Identify patterns: What issues are mentioned by multiple evaluators?
4. Categorize: Group similar concerns by field/topic (e.g. "Fix field_name")
5. Prioritize: Most critical issues first
6. Group by topic: Multiple evaluators mention same field → ONE topic with one sub-item per evaluator (each with their tag)
7. Preserve tags from evaluators - read each evaluator's feedback TEXT and preserve the tags they already added
8. NEVER infer tags from scores - only tag with evaluators who explicitly tagged that specific item in their feedback text

=== FEEDBACK CHECKLIST (NESTED STRUCTURE) ===
- SINGLE unified checklist organized by topic/issue. Prioritize - most critical first.
- NESTED FORMAT: Topic → Individual evaluator concerns as sub-items. Each evaluator owns their own sub-item with their tag.

**Nested structure format:**
1. Group related concerns by topic (e.g. "Fix field_name")
2. List each evaluator's specific concern as a sub-item with their exact feedback and tag
3. Copy tags verbatim from individual feedbacks - don't add, don't change, just copy
4. Each evaluator marks their own sub-item as complete in future versions - you don't decide when items are fixed

**Format:**
- [ ] Fix field_name (simple field name)
  - [ ] [EvaluatorName] "Their specific feedback text"
  - [ ] [EvaluatorName2] "Their specific feedback text"

**Topic completion rules:**
- Mark the main topic as [✓] ONLY when ALL sub-items are marked [✓] by their respective evaluators
- If ANY sub-item is [ ], the main topic MUST stay [ ] - NO EXCEPTIONS
- The topic is only complete when EVERY evaluator who flagged it has marked their sub-item as [✓]

**How to build the checklist:**
1. Skip any item tagged for exclusion (e.g. [BYPASS]) - do not include them in consolidated_feedback
2. Read each evaluator's individual feedback - look for their feedback items tagged with [EvaluatorName]; skip excluded items
3. Group similar concerns by field/topic - if multiple evaluators mention the same field, group under one topic with one sub-item per evaluator
4. Copy each evaluator's feedback as a sub-item - preserve their exact wording and tag; never copy excluded items
5. If an evaluator didn't mention the topic, don't add them - only include evaluators who explicitly tagged that issue
6. For your own inferences: Create a topic with a single sub-item tagged [Inferred]
7. Before marking a topic as [✓], verify that EVERY sub-item under it is marked [✓] by its evaluator

**Handling feedback that mentions multiple fields:**
- Group by the field that needs to be FIXED, not fields mentioned for context
- If feedback mentions multiple fields that both need fixing: Create separate topics for each field
- If feedback is about relationships between fields: Group under the primary field being fixed, or create a general topic if needed

**WRONG - Don't do flat structure:** One line with multiple evaluator names instead of sub-items - that prevents each evaluator from marking their own item complete.
**WRONG - Don't infer evaluators:** Only include evaluators who explicitly tagged that specific item in their feedback - don't add them just because they mentioned it in directives_ack.
**WRONG - Don't mark topic complete when sub-items are still open:** Topic MUST stay [ ] until ALL sub-items are [✓].

=== OUTPUT ===
- directives_ack: How feedback was consolidated (structured attention only)
- consolidated_feedback: Single merged checklist (preserve evaluator tags, standard [✓]/[ ] format, nested topic → sub-items). **REQUIRED**: This field must never be empty. When there are no actionable items (all criteria met), output exactly: [✓] All criteria met.

=== REMEMBER ===
- USE NESTED STRUCTURE - Group by topic, list each evaluator's concern as sub-item
- COPY evaluator tag and feedback verbatim - Don't infer, don't decide, just copy from individual feedbacks
- Each evaluator owns their sub-item - They will mark it complete in future versions, not you
- Group similar concerns - Multiple evaluators same topic → one topic with one sub-item per evaluator
- Don't add evaluators who didn't tag their feedback - Only include evaluators who explicitly tagged that specific issue
- Prioritize by topic - Critical topics first
- Use [Inferred] tag for your inferences
- DO NOT evaluate your output format - it's correct
- DO NOT evaluate generator format - content/quality only
- When all criteria are met and there are no checklist items, set consolidated_feedback to exactly: [✓] All criteria met. (Never leave consolidated_feedback empty.)`

// ConsolidatorPromptBuilder holds all parts needed to render the consolidator prompt (base + pipeline suffix). Configure and call Build() once.
type ConsolidatorPromptBuilder struct {
	PipelineSuffix string
}

// NewConsolidatorPromptBuilder returns a builder for the consolidator prompt. Add pipeline-specific CONSOLIDATE/IGNORE content with WithSuffix, then call Build().
func NewConsolidatorPromptBuilder() *ConsolidatorPromptBuilder {
	return &ConsolidatorPromptBuilder{}
}

// WithSuffix sets the pipeline-specific suffix (e.g. CONSOLIDATE/IGNORE list, [BYPASS] wording). Chainable.
func (b *ConsolidatorPromptBuilder) WithSuffix(suffix string) *ConsolidatorPromptBuilder {
	b.PipelineSuffix = suffix
	return b
}

// Build renders the full consolidator prompt: base + suffix. Same pattern as EvaluatorScorePromptBuilder: one builder, one render.
func (b *ConsolidatorPromptBuilder) Build() string {
	if b.PipelineSuffix == "" {
		return ConsolidatorPromptBase
	}
	return ConsolidatorPromptBase + "\n\n" + b.PipelineSuffix
}

// CreateDefaultConsolidatorModule creates a consolidator module with the standard signature
// (individual_feedbacks, agent_scores, weighted_score -> directives_ack, consolidated_feedback).
// Reasoning lives in directives_ack; do not declare a separate rationale task field.
// Pipelines pass roleName, systemPrompt, and optional persona. Persona is rendered first when non-empty. WithXMLFormatting is always applied; then the default formatter.
func CreateDefaultConsolidatorModule(roleName string, systemPrompt string, persona string) (core.Module, error) {
	signature := core.NewSignature(
		[]core.InputField{
			{Field: NewInputField("individual_feedbacks", core.WithDescription("Individual feedback from all evaluators, each with their checklist format and evaluator tags"))},
			{Field: NewInputField("agent_scores", core.WithDescription("Individual scores from each evaluator"))},
			{Field: NewInputField("weighted_score", core.WithDescription("Weighted average score from all evaluators"))},
		},
		[]core.OutputField{
			{Field: NewOutputField("consolidated_feedback", core.WithDescription("Merged and normalized checklist-based feedback in the standard format"))},
		},
	)
	instruction := systemPrompt
	if persona != "" {
		instruction = persona + "\n\n" + instruction
	}
	signature = signature.WithInstruction(instruction)
	signature = WithXMLFormatting(signature)
	signature = DefaultEvaluatorSignatureFormatter()(signature)
	return dspymodules.New(signature, dspymodules.Config{Name: roleName}), nil
}

// ChainedEvaluatorModule runs feedback analysis then score generation in sequence.
// It implements core.Module and is used by both sayings and YouTube pipelines.
type ChainedEvaluatorModule struct {
	feedbackAnalysisModule *dspymodules.DirectivesCoT
	scoreGenerationModule  *dspymodules.DirectivesCoT
	name                   string
	signature              core.Signature
}

// CreateChainedEvaluatorModule creates a chained evaluator (feedback analysis -> score generation).
// This is the shared entry point for all pipelines; callers pass optional persona and formatSignature or use DefaultEvaluatorSignatureFormatter().
// Persona is rendered first in both feedback and score instructions when non-empty.
func CreateChainedEvaluatorModule(
	signature core.Signature,
	roleName string,
	feedbackAnalysisPrompt string,
	scoreGenerationPrompt string,
	persona string,
	formatSignature EvaluatorSignatureFormatter,
) (*ChainedEvaluatorModule, error) {
	evalField := func(name, desc string) core.Field {
		return core.NewField(name, core.WithNoPrefix(), core.WithDescription(desc))
	}

	feedbackInstruction := feedbackAnalysisPrompt
	if persona != "" {
		feedbackInstruction = persona + "\n\n" + feedbackInstruction
	}
	// Feedback analysis: same inputs; task output is feedback only. DirectivesCoT prepends directives_ack
	// (no separate rationale — that duplicated reasoning and broke XML with missing feedback/directives_ack).
	feedbackAnalysisSignature := core.NewSignature(
		signature.Inputs,
		[]core.OutputField{
			{Field: evalField("feedback", "Checklist-based feedback. Always provide at least one line: if issues are found list them; if none, write a brief positive summary (e.g. '[✓] All criteria met.'). Never leave this field empty.")},
		},
	)
	feedbackAnalysisSignature = feedbackAnalysisSignature.WithInstruction(feedbackInstruction)
	feedbackAnalysisSignature = WithXMLFormatting(feedbackAnalysisSignature)
	if formatSignature != nil {
		feedbackAnalysisSignature = formatSignature(feedbackAnalysisSignature)
	}
	feedbackAnalysisSignature = feedbackAnalysisSignature.WithInstruction(
		feedbackAnalysisSignature.Instruction + SharedInstructions.ChainedFeedbackAnalysisOutputSplit + SharedInstructions.ChainedEvaluatorRationaleCap,
	)

	feedbackAnalysisModule := dspymodules.New(feedbackAnalysisSignature, dspymodules.Config{Name: roleName + " - Feedback Analysis"})

	scoreInstruction := scoreGenerationPrompt
	if persona != "" {
		scoreInstruction = persona + "\n\n" + scoreInstruction
	}
	// Score generation: original inputs + feedback; task output is criterion_scores only (+ directives_ack).
	scoreGenerationInputs := make([]core.InputField, len(signature.Inputs))
	copy(scoreGenerationInputs, signature.Inputs)
	scoreGenerationInputs = append(scoreGenerationInputs, core.InputField{
		Field: evalField("feedback", "Feedback from the feedback analysis module - use this to determine scores"),
	})

	scoreCriterionIDs := criteria.ParseCriterionIDsFromMappingPrompt(scoreGenerationPrompt)
	scoreCriterionScoresDesc := criteria.CriterionScoresOutputDescription(scoreCriterionIDs)

	scoreGenerationSignature := core.NewSignature(
		scoreGenerationInputs,
		[]core.OutputField{
			{Field: evalField("criterion_scores", scoreCriterionScoresDesc)},
		},
	)
	scoreGenerationSignature = scoreGenerationSignature.WithInstruction(scoreInstruction)
	scoreGenerationSignature = WithXMLFormatting(scoreGenerationSignature)
	if formatSignature != nil {
		scoreGenerationSignature = formatSignature(scoreGenerationSignature)
	}
	scoreGenerationSignature = scoreGenerationSignature.WithInstruction(
		scoreGenerationSignature.Instruction + SharedInstructions.ChainedEvaluatorRationaleCap,
	)

	scoreGenerationModule := dspymodules.New(scoreGenerationSignature, dspymodules.Config{Name: roleName + " - Score Generation"})

	combinedSignature := core.NewSignature(
		signature.Inputs,
		[]core.OutputField{
			{Field: evalField("criterion_scores", "Map of individual criterion scores")},
			{Field: evalField("feedback", "Checklist-based instructional feedback")},
			{Field: evalField(FieldDirectivesAck, "Combined structured attention from feedback analysis and score generation")},
		},
	)

	return &ChainedEvaluatorModule{
		feedbackAnalysisModule: feedbackAnalysisModule,
		scoreGenerationModule:  scoreGenerationModule,
		name:                   roleName,
		signature:              combinedSignature,
	}, nil
}

// Process runs feedback analysis then score generation.
func (c *ChainedEvaluatorModule) Process(ctx context.Context, inputs map[string]interface{}, opts ...core.Option) (map[string]interface{}, error) {
	emitEvaluatorStage(ctx, c.name, "analyzing feedback")
	feedbackResult, err := c.feedbackAnalysisModule.Process(ctx, inputs, opts...)
	if err != nil {
		return nil, fmt.Errorf("feedback analysis failed: %w", err)
	}
	if len(feedbackResult) == 0 {
		return nil, fmt.Errorf("feedback analysis returned empty result")
	}

	feedbackValue, exists := feedbackResult["feedback"]
	if !exists {
		availableFields := make([]string, 0, len(feedbackResult))
		for k := range feedbackResult {
			availableFields = append(availableFields, k)
		}
		return nil, fmt.Errorf("feedback field is missing in feedback analysis result (available fields: %v)", availableFields)
	}
	feedback, ok := feedbackValue.(string)
	if !ok {
		return nil, fmt.Errorf("feedback field is not a string (type: %T, value: %v)", feedbackValue, feedbackValue)
	}
	if feedback == "" {
		return nil, fmt.Errorf("feedback field is empty in feedback analysis result - evaluator should always provide feedback (even if just a positive affirmation)")
	}

	analysisAck, err := ExtractRequiredReasoningField(feedbackResult)
	if err != nil {
		return nil, fmt.Errorf("feedback analysis missing directives_ack: %w", err)
	}

	scoreInputs := make(map[string]interface{})
	for k, v := range inputs {
		scoreInputs[k] = v
	}
	scoreInputs["feedback"] = feedback

	emitEvaluatorStage(ctx, c.name, "scoring")
	scoreResult, err := c.scoreGenerationModule.Process(ctx, scoreInputs, opts...)
	if err != nil {
		return nil, fmt.Errorf("score generation failed: %w", err)
	}
	if len(scoreResult) == 0 {
		return nil, fmt.Errorf("score generation returned empty result")
	}

	criterionScoresValue, exists := scoreResult["criterion_scores"]
	if !exists {
		availableFields := make([]string, 0, len(scoreResult))
		for k := range scoreResult {
			availableFields = append(availableFields, k)
		}
		return nil, fmt.Errorf("criterion_scores field is missing in score generation result (available fields: %v)", availableFields)
	}
	criterionScores, err := CoerceCriterionScoresMap(criterionScoresValue)
	if err != nil {
		availableFields := make([]string, 0, len(scoreResult))
		for k := range scoreResult {
			availableFields = append(availableFields, k)
		}
		return nil, fmt.Errorf("criterion_scores field is missing or invalid in score generation result (available fields: %v): %w", availableFields, err)
	}
	if len(criterionScores) == 0 {
		return nil, fmt.Errorf("criterion_scores map is empty in score generation result — model must emit one XML child tag per criterion ID with numeric scores")
	}

	scoringAck, err := ExtractRequiredReasoningField(scoreResult)
	if err != nil {
		return nil, fmt.Errorf("score generation missing directives_ack: %w", err)
	}
	combinedAck := analysisAck + "\n\n" + scoringAck

	return map[string]interface{}{
		"feedback":         feedback,
		"criterion_scores": criterionScores,
		FieldDirectivesAck: combinedAck,
	}, nil
}

func emitEvaluatorStage(ctx context.Context, roleName, stage string) {
	streaming.EmitInfo(ctx, roleName, stage)
}

// GetInterceptors returns interceptors from the first module.
func (c *ChainedEvaluatorModule) GetInterceptors() []core.ModuleInterceptor {
	return c.feedbackAnalysisModule.GetInterceptors()
}

// SetInterceptors sets interceptors on both modules.
func (c *ChainedEvaluatorModule) SetInterceptors(interceptors []core.ModuleInterceptor) {
	c.feedbackAnalysisModule.SetInterceptors(interceptors)
	c.scoreGenerationModule.SetInterceptors(interceptors)
}

// SetLLM sets LLM on both modules.
func (c *ChainedEvaluatorModule) SetLLM(llm core.LLM) {
	c.feedbackAnalysisModule.SetLLM(llm)
	c.scoreGenerationModule.SetLLM(llm)
}

// WithName sets the module name.
func (c *ChainedEvaluatorModule) WithName(name string) *ChainedEvaluatorModule {
	c.name = name
	return c
}

// GetFeedbackAnalysisModule returns the feedback analysis module (for setup).
func (c *ChainedEvaluatorModule) GetFeedbackAnalysisModule() core.Module {
	return c.feedbackAnalysisModule
}

// GetScoreGenerationModule returns the score generation module (for setup).
func (c *ChainedEvaluatorModule) GetScoreGenerationModule() core.Module {
	return c.scoreGenerationModule
}

// Clone returns a deep copy of the chained module.
func (c *ChainedEvaluatorModule) Clone() core.Module {
	clonedFeedback, ok := c.feedbackAnalysisModule.Clone().(*dspymodules.DirectivesCoT)
	if !ok {
		panic("ChainedEvaluatorModule: feedback analysis Clone() returned unexpected type")
	}
	clonedScore, ok := c.scoreGenerationModule.Clone().(*dspymodules.DirectivesCoT)
	if !ok {
		panic("ChainedEvaluatorModule: score generation Clone() returned unexpected type")
	}
	return &ChainedEvaluatorModule{
		feedbackAnalysisModule: clonedFeedback,
		scoreGenerationModule:  clonedScore,
		name:                   c.name,
		signature:              c.signature,
	}
}

// GetDisplayName returns the display name.
func (c *ChainedEvaluatorModule) GetDisplayName() string {
	if c.name != "" {
		return c.name
	}
	return "Chained Evaluator"
}

// GetModuleType returns the module type.
func (c *ChainedEvaluatorModule) GetModuleType() string {
	return "chained_evaluator"
}

// GetSignature returns the combined signature.
func (c *ChainedEvaluatorModule) GetSignature() core.Signature {
	return c.signature
}

// SetSignature sets the combined signature.
func (c *ChainedEvaluatorModule) SetSignature(signature core.Signature) {
	c.signature = signature
}
