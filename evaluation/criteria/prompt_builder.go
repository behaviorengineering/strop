package criteria

import (
	"fmt"
	"strings"
	"sync"
)

// RoleScopeResolver returns role-specific scope text for an evaluator by display name.
// Pipelines inject their own implementation; criteria never imports pipeline code.
type RoleScopeResolver func(roleName string) string

// PromptBuilder generates evaluator prompts from criterion lists.
type PromptBuilder struct {
	registry          *CriterionRegistry
	RoleScopeResolver RoleScopeResolver // Optional: set by pipeline (e.g. sayings) when building prompts
}

// NewPromptBuilder creates a new prompt builder.
func NewPromptBuilder(registry *CriterionRegistry) (*PromptBuilder, error) {
	if registry == nil {
		return nil, fmt.Errorf("CriterionRegistry cannot be nil")
	}
	return &PromptBuilder{registry: registry}, nil
}

// BuildEvaluatorPrompt generates a complete evaluator prompt from criterion IDs.
func (pb *PromptBuilder) BuildEvaluatorPrompt(
	roleName string,
	focusAreas string,
	criterionIDs []CriterionID,
) (string, error) {
	// Fetch criteria from registry.
	criteria, err := pb.registry.GetMultiple(criterionIDs)
	if err != nil {
		return "", fmt.Errorf("failed to fetch criteria: %w", err)
	}

	// Group criteria by category.
	processCriteria := []CriterionDescription{}
	qualityCriteria := []CriterionDescription{}

	for _, crit := range criteria {
		switch crit.Category {
		case CriterionCategoryProcessEvaluation:
			processCriteria = append(processCriteria, crit)
		case CriterionCategoryOutputQuality:
			qualityCriteria = append(qualityCriteria, crit)
		}
	}

	// Calculate point totals.
	totalPoints := 0.0
	for _, crit := range criteria {
		totalPoints += crit.MaxPoints
	}

	processPoints := 0.0
	for _, crit := range processCriteria {
		processPoints += crit.MaxPoints
	}

	qualityPoints := 0.0
	for _, crit := range qualityCriteria {
		qualityPoints += crit.MaxPoints
	}

	// Build sections.
	rubricSection := pb.buildRubricSection(processCriteria, qualityCriteria, processPoints, qualityPoints)
	processSection := pb.buildProcessSection()
	scoreBreakdown := pb.buildScoreBreakdownFormat(criteria)
	criterionIDMapping := pb.buildCriterionIDMapping(criteria)

	// Build category summary for table of contents.
	categorySummary := pb.buildCategorySummary(processCriteria, qualityCriteria, processPoints, qualityPoints)

	// Assemble full prompt.
	prompt := fmt.Sprintf(`TABLE OF CONTENTS:
1. ROLE AND PURPOSE - Your role as evaluator and instructional coach
2. SCORING RUBRIC - Detailed scoring components (%d components, %.0f points total)
%s
3. PROCESS - Step-by-step evaluation workflow (score first, then feedback)
4. SCORE BREAKDOWN FORMAT - Required feedback structure with component scores
5. FEEDBACK CHECKLIST FORMAT - Checklist structure requirements
6. CONSTRAINTS FORMAT - Constraint specification requirements
7. ACTIONABLE STEPS FORMAT - Action step requirements
8. REMINDERS - Key evaluation principles

---

[Section 1] You are a %s evaluator acting as an instructional coach.

Your role is to review the generator's work and provide actionable guidance for improvement.

[Section 2] SCORING RUBRIC (Calculate score from these discrete components):

%s

[Section 3] CRITERION ID MAPPING (use these exact IDs in criterion_scores output):
%s

%s

[Section 4] SCORE BREAKDOWN (include in feedback):
%s

[Section 5] FEEDBACK CHECKLIST (NESTED STRUCTURE):

**CRITICAL: FEEDBACK IS ORGANIZED AS NESTED STRUCTURE**
Previous feedback (if present) is organized as:
- [ ] Topic: Brief description
  - [ ] [EvaluatorName1] "Their concern"
  - [ ] [%s] "Your concern"
  - [ ] [EvaluatorName2] "Their concern"

**YOUR JOB:**
1. **Check previous_feedback for YOUR sub-items** - Look for sub-items tagged with [%s]
2. **Mark YOUR sub-items as [✓] FIXED if satisfied** - If you're happy with the fix, mark YOUR sub-item as complete
3. **Keep YOUR sub-items as [ ] if still needs work** - If you're not satisfied, keep YOUR sub-item open and update your concern
4. **Create NEW sub-items for new issues you find** - If you find a new issue, create a new sub-item tagged with [%s]

**Format for YOUR feedback:**
FEEDBACK CHECKLIST:
- [ ] Topic: Brief description (use existing topic from previous_feedback if reviewing same issue)
  - [✓] [%s] FIXED in v{version} - quote the improvement (if you're satisfied with a previous concern)
  - [ ] [%s] Still needs work - quote where issue exists (if you're still not satisfied)
  - [ ] [%s] NEW issue found - describe new issue (if you found a new issue)

**CRITICAL RULES:**
1. **ONLY mark YOUR sub-items as complete** - Don't mark other evaluators' sub-items
2. **ALWAYS tag YOUR sub-items with [%s] as prefix** - This identifies your concerns (e.g., "[Cultural Anthropologist] feedback text")
3. **ONE field per feedback item** - If you want to address multiple fields, create SEPARATE feedback items for EACH field (e.g., one item for "Fix idiomatic_translation", another for "Fix literal_translation"). DO NOT combine multiple fields into one feedback item.
4. **Preserve the topic grouping** - Keep sub-items under the same topic if reviewing the same issue
5. **Create new topics for new issues** - If you find a completely new issue, create a new topic with your sub-item

**Example - Multiple fields (create separate items):**
❌ WRONG: "[%%s] NEW issue found - The literal_translation uses 'ignited' and the idiomatic_translation uses 'blaze,' which are semantically close. The idiomatic translation should ideally use a more distinct English idiom..."
✅ CORRECT: Create TWO separate items:
  - [ ] [%%s] NEW issue found - The idiomatic_translation uses 'blaze' which is semantically close to the literal_translation's 'ignited'. The idiomatic translation should ideally use a more distinct English idiom to maximize the difference between the translations.
  - [ ] [%%s] NEW issue found - The literal_translation uses 'ignited' which is semantically close to the idiomatic_translation's 'blaze'. Consider using a different term to maximize distinction.

CONSTRAINTS FOR NEXT VERSION:
(List YOUR specific constraints for the generator - what you want them to change)
- Topic: Brief constraint description
  - [%s] "Your specific constraint with example"
  - [%s] "Your multiline constraint that spans multiple lines should start on a new line"

ACTIONABLE STEPS:
(List YOUR specific actions for the generator - concrete steps to address your concerns)
1. Main action for topic
   - [%s] "Your specific action with concrete example"
   - [%s] "Your multiline action that spans multiple lines should start on a new line"

Remember: Reference specific phrases/choices from generator_output. Be instructional, not just evaluative. Focus on %s.
Remember: ALWAYS tag YOUR sub-items with [%s] as prefix to indicate ownership (e.g., "[Cultural Anthropologist] feedback text").
Remember: ONLY mark YOUR sub-items as complete - let other evaluators mark their own.`,
		len(criteria),
		totalPoints,
		categorySummary,
		roleName,
		rubricSection,
		criterionIDMapping,
		processSection,
		scoreBreakdown,
		roleName,   // [%s] in example (line 105)
		roleName,   // tagged with [%s] (line 109)
		roleName,   // create new sub-item tagged with [%s] (line 112)
		roleName,   // FIXED format [%s] (line 117)
		roleName,   // Still needs work [%s] (line 118)
		roleName,   // NEW issue [%s] (line 119)
		roleName,   // ALWAYS tag with [%s] (line 123)
		roleName,   // constraint [%s] (line 130)
		roleName,   // multiline constraint [%s] (line 131)
		roleName,   // step [%s] (line 136)
		roleName,   // multiline step [%s] (line 137)
		focusAreas, // Focus on %s (line 139)
		roleName,   // tag YOUR sub-items [%s] (line 140)
	)

	return prompt, nil
}

// buildCriterionIDMapping generates the CRITERION ID MAPPING section.
func (pb *PromptBuilder) buildCriterionIDMapping(criteria []CriterionDescription) string {
	var sb strings.Builder
	sb.WriteString("CRITERION ID MAPPING (use these exact IDs in criterion_scores output):\n")
	for _, crit := range criteria {
		sb.WriteString(fmt.Sprintf("- %s → %q\n", crit.Name, string(crit.ID)))
	}
	sb.WriteString("\n") // Add trailing newline for formatting.
	return sb.String()
}

// buildCategorySummary generates the category summary for table of contents.
func (pb *PromptBuilder) buildCategorySummary(processCriteria, qualityCriteria []CriterionDescription, processPoints, qualityPoints float64) string {
	var parts []string

	if len(processCriteria) > 0 {
		names := make([]string, len(processCriteria))
		for i, crit := range processCriteria {
			names[i] = crit.Name
		}
		parts = append(parts, fmt.Sprintf("   - %s (%.0f points): %s", CriterionCategoryProcessEvaluation, processPoints, strings.Join(names, ", ")))
	}

	if len(qualityCriteria) > 0 {
		names := make([]string, len(qualityCriteria))
		for i, crit := range qualityCriteria {
			names[i] = crit.Name
		}
		parts = append(parts, fmt.Sprintf("   - %s (%.0f points): %s", CriterionCategoryOutputQuality, qualityPoints, strings.Join(names, ", ")))
	}

	return strings.Join(parts, "\n")
}

// buildRubricSection generates the rubric section with all criteria.
func (pb *PromptBuilder) buildRubricSection(processCriteria, qualityCriteria []CriterionDescription, processPoints, qualityPoints float64) string {
	var sb strings.Builder

	// Process Evaluation section.
	if len(processCriteria) > 0 {
		sb.WriteString(fmt.Sprintf("%s (%.0f points total):\n", CriterionCategoryProcessEvaluation, processPoints))
		for i, crit := range processCriteria {
			sb.WriteString(fmt.Sprintf("%d. %s (0-%.0f points):\n", i+1, crit.Name, crit.MaxPoints))
			sb.WriteString(fmt.Sprintf("   %s\n", crit.Description))
			// Parse scoring levels from the Scoring field.
			scoringLevels := pb.parseScoringLevels(crit.Scoring)
			for _, level := range scoringLevels {
				sb.WriteString(fmt.Sprintf("   - %s\n", level))
			}
			// Add evidence placeholder based on category.
			evidence := pb.getEvidencePlaceholder(crit)
			if evidence != "" {
				sb.WriteString(fmt.Sprintf("   - Evidence: %s\n", evidence))
			}
			sb.WriteString("\n")
		}
	}

	// Output Quality Evaluation section.
	if len(qualityCriteria) > 0 {
		startNum := len(processCriteria) + 1
		sb.WriteString(fmt.Sprintf("%s (%.0f points total):\n", CriterionCategoryOutputQuality, qualityPoints))
		for i, crit := range qualityCriteria {
			sb.WriteString(fmt.Sprintf("%d. %s (0-%.0f points):\n", startNum+i, crit.Name, crit.MaxPoints))
			sb.WriteString(fmt.Sprintf("   %s\n", crit.Description))
			scoringLevels := pb.parseScoringLevels(crit.Scoring)
			for _, level := range scoringLevels {
				sb.WriteString(fmt.Sprintf("   - %s\n", level))
			}
			// Include examples for output quality criteria to help evaluators identify violations.
			if crit.Examples != "" {
				sb.WriteString(fmt.Sprintf("   Examples:\n%s\n", crit.Examples))
			}
			evidence := pb.getEvidencePlaceholder(crit)
			if evidence != "" {
				sb.WriteString(fmt.Sprintf("   - Evidence: %s\n", evidence))
			}
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// parseScoringLevels parses scoring levels from the Scoring field.
// Format: "2 points: ... 1 point: ... 0 points: ...".
func (pb *PromptBuilder) parseScoringLevels(scoring string) []string {
	var levels []string

	// The Scoring field format is: "2 points: ... 1 point: ... 0 points: ...".

	// Try to find each scoring level.
	scorePatterns := []struct {
		pattern string
		score   string
	}{
		{"2 points:", "2"},
		{"1 point:", "1"},
		{"0 points:", "0"},
	}

	for _, sp := range scorePatterns {
		idx := strings.Index(scoring, sp.pattern)
		if idx == -1 {
			continue
		}

		// Extract text after the pattern.
		startIdx := idx + len(sp.pattern)
		levelText := strings.TrimSpace(scoring[startIdx:])

		// Find the next "points:" pattern.
		cutoffIdx := len(levelText)
		for _, nextSp := range scorePatterns {
			if nextSp.score == sp.score {
				continue // Skip same pattern.
			}
			if foundIdx := strings.Index(levelText, nextSp.pattern); foundIdx != -1 && foundIdx < cutoffIdx {
				cutoffIdx = foundIdx
			}
		}

		if cutoffIdx < len(levelText) {
			levelText = levelText[:cutoffIdx]
		}

		// Trim trailing period if present.
		levelText = strings.TrimRight(levelText, ".")

		if levelText != "" {
			levels = append(levels, fmt.Sprintf("%s points: %s", sp.score, strings.TrimSpace(levelText)))
		}
	}

	// Sort by score (2, 1, 0).
	sortedLevels := make([]string, 0, 3)
	for _, score := range []string{"2", "1", "0"} {
		for _, level := range levels {
			if strings.HasPrefix(level, score+" points:") {
				sortedLevels = append(sortedLevels, level)
				break
			}
		}
	}

	return sortedLevels
}

// getEvidencePlaceholder returns an evidence placeholder based on the criterion.
func (pb *PromptBuilder) getEvidencePlaceholder(crit CriterionDescription) string {
	// Map criteria to appropriate evidence placeholders.
	evidenceMap := map[CriterionID]string{
		CriterionIDInstructionCompliance:         "[Quote specific examples from generator_output showing instruction adherence]",
		CriterionIDCompleteness:                  "[Check required generator_output fields listed in the job process prompt; do NOT require a separate rationale field unless that job lists rationale as required]",
		CriterionIDFeedbackAdherence:             "[Check each item in previous_feedback checklist; verify no content in [Section 2] PRESERVE INSTRUCTIONS was changed]",
		CriterionIDContextAwareness:              "[Check generator_output (and directives_ack if present) for context source usage — did it use saying_context / inputs correctly?]",
		CriterionIDOutputQuality:                 "[Evaluate output against role-specific focus areas]",
		CriterionIDClarityReadability:            "[Evaluate clarity, readability, and communication quality]",
		CriterionIDFactCheckableIdeas:            "[Check each sub_idea: is it self-contained? Does it state the claim directly (no meta-framing)? Flag fragments, semantic overlap, or meta-framing (is presented as, is framed as)]",
		CriterionIDWorthyMoments:                 "[Check each quote: clip-worthy, shareable, brief (~12–35 words)? Light editing for shareability is acceptable. Flag long rambling, filler-heavy, or repetitive quotes]",
		CriterionIDInfluenceComparabilityProfile: "[Check tactic_target_levels, tactic_primary_affects, tactic_secondary_affects (five each, allowed tokens); comparability_profile matches per-cue tags and mechanism Effect/Nudge; not sentiment polarity]",
	}

	if evidence, ok := evidenceMap[crit.ID]; ok {
		return evidence
	}

	// Default evidence placeholder.
	return "[Evaluate based on criterion requirements]"
}

// buildProcessSection generates the PROCESS section.
func (pb *PromptBuilder) buildProcessSection() string {
	return `[Section 3] PROCESS (CRITICAL: Follow this order strictly - think critically about feedback, then score, then generate feedback):
1. Review generator_input: What the generator received (original_text, previous_feedback, version)
2. Review generator_output: What the generator produced

3. **CRITICAL ANALYSIS PHASE (in directives_ack)**: For EACH criterion, think critically about what feedback would be given
   - Evaluate the output against the generator's task requirements (from your synchronized evaluator prompt)
   - Then analyze against quality criteria
   - For EACH criterion, critically analyze what specific feedback would be needed (strengths, issues, improvements)
   - Write this critical analysis to directives_ack — this is your working memory for scoring (do NOT invent a generator rationale field)
   - Based on this critical analysis, assign a score (0.0 to max_points) that reflects the feedback needed
   - Example directives_ack structure:
     "Instruction Compliance: Critical analysis - [what feedback would be given, specific issues or strengths found]. Based on this analysis, Score: 2.0/2.0 because [reasoning]...
      Completeness: Critical analysis - [what feedback would be given, specific issues or strengths found]. Based on this analysis, Score: 2.0/2.0 because [reasoning]...
      [Continue for all criteria with critical feedback analysis and score for each]"

4. **FEEDBACK GENERATION PHASE**: Generate feedback output based on the critical analysis in directives_ack
5. Output criterion_scores map with all scores (must match the analysis in directives_ack)
6. Output feedback with score breakdown and recommendations (must align with the critical analysis in directives_ack)`
}

// buildWhatToEvaluateSection generates role-specific "WHAT TO EVALUATE" content.
func (pb *PromptBuilder) buildWhatToEvaluateSection(roleName string, outputQualityCriteriaNames string) string {
	if roleName == "Process Evaluator" {
		return `WHAT TO EVALUATE (Process & Structure Only):
- ✅ Structural completeness: Are all required fields populated with content?
- ✅ Feedback compliance: Did they address each item from previous_feedback checklist and keep [Section 2] PRESERVE (user-approved) content unchanged?
- ✅ Instruction compliance: Did they follow format and structural requirements?
- **See rubric for detailed process evaluation criteria**: Instruction Compliance, Completeness, Feedback Adherence
- How well the generator followed the structural and process requirements (from your synchronized evaluator prompt)`
	}

	// Default for all other evaluators (content quality focus).
	return fmt.Sprintf(`WHAT TO EVALUATE (Content Quality Only):
- The actual CONTENT of the generator output (explanation/translation/image prompt text)
- **See rubric for detailed output quality criteria**: %s
- Cultural authenticity, clarity, engagement, impact, style, tone
- How well the content meets the generator's task requirements (from your synchronized evaluator prompt)
- Content-related improvements needed`, outputQualityCriteriaNames)
}

// buildRoleScopeSection generates role-specific scope sections for evaluators.
// Uses RoleScopeResolver when set by the pipeline; otherwise returns empty.
func (pb *PromptBuilder) buildRoleScopeSection(roleName string) string {
	if pb.RoleScopeResolver != nil {
		return pb.RoleScopeResolver(roleName)
	}
	return ""
}

// buildScoreBreakdownFormat generates the SCORE BREAKDOWN format section.
func (pb *PromptBuilder) buildScoreBreakdownFormat(criteria []CriterionDescription) string {
	var sb strings.Builder

	for _, crit := range criteria {
		optionalNote := ""
		if crit.ID == CriterionIDFeedbackAdherence {
			optionalNote = " (if applicable)"
		}
		sb.WriteString(fmt.Sprintf("- %s: {points}/%.0f - [brief explanation]%s\n", crit.Name, crit.MaxPoints, optionalNote))
	}

	return sb.String()
}

// This module only identifies issues and generates improvement feedback (no scoring).
func (pb *PromptBuilder) BuildFeedbackAnalysisPrompt(
	roleName string,
	focusAreas string,
	criterionIDs []CriterionID,
) (string, error) {
	// Fetch criteria from registry.
	criteria, err := pb.registry.GetMultiple(criterionIDs)
	if err != nil {
		return "", fmt.Errorf("failed to fetch criteria: %w", err)
	}

	// Build rubric section for reference (to understand what to evaluate).
	rubricSection := pb.buildRubricSectionForReference(criteria)

	// Build dynamic list of output quality criteria names.
	outputQualityCriteriaNames := pb.buildOutputQualityCriteriaNames(criteria)

	// Build role-specific "WHAT TO EVALUATE" section.
	whatToEvaluateSection := pb.buildWhatToEvaluateSection(roleName, outputQualityCriteriaNames)

	// Build role scope section.
	roleScopeSection := pb.buildRoleScopeSection(roleName)

	prompt := fmt.Sprintf(`[Section 1] You are a %s evaluator acting as an instructional coach.
Your role is to identify areas for improvement and provide actionable guidance.

[Section 2] CRITICAL - WHAT TO EVALUATE (Content Only, Not XML Structure):
**YOU ARE EVALUATING CONTENT QUALITY, NOT XML STRUCTURE**

%s

WHAT TO IGNORE (Do NOT Evaluate):
- ❌ XML structure (e.g., "wrong XML structure", CDATA markers)
- ❌ Field names or XML tags (e.g., "must produce exactly three fields")
- ❌ Technical XML formatting issues
- ❌ Your own output format (you output: feedback + directives_ack — that is correct; do not require a separate rationale tag)
- ❌ Generator output format structure (focus on content quality, not format)
- ❌ Demanding a generator "rationale" field unless the job process prompt lists rationale as required

CRITICAL RULES:
1. Evaluate ONLY the CONTENT quality of generator_output (the actual text/content)
2. Do NOT evaluate XML structure, field names, or technical formatting
3. Your feedback should be about content improvements, not XML structure fixes
4. Focus on what the generator needs to improve in the CONTENT itself

[Section 3] EVALUATION CRITERIA (for reference - understand what to evaluate):
%s

[Section 4] REVIEW PROCESS:
1. Review generator_input: What the generator received (original_text, previous_feedback, version)
2. Review generator_output: What the generator produced (focus on CONTENT quality, not XML structure)
   - Your evaluator prompt (above) contains the synchronized generator instructions - use these to understand what the generator was supposed to produce

[Section 5] FEEDBACK GENERATION:
- **First, verify the CONTENT meets the generator's task requirements (from your synchronized evaluator prompt above)**
- Then, critically analyze the CONTENT against each criterion
- **CRITICAL: Do not ask for changes that contradict the generator's task requirements from the synchronized prompt**
- Identify specific CONTENT issues, strengths, and areas for improvement
- **CRITICAL: You MUST always provide feedback - it is a required field**
- **CRITICAL: Provide feedback about CONTENT quality only - do NOT mention XML structure**
- If issues are found: Generate feedback items with checklist, constraints, and actionable steps (all about CONTENT)
- If output is perfect with no issues: Provide a positive affirmation in the feedback field

[Section 6] FEEDBACK CHECKLIST FORMAT (NESTED STRUCTURE):
**CRITICAL: The feedback field is ALWAYS required. Never leave it empty.**
**CRITICAL: Provide feedback about CONTENT quality only - do NOT mention XML structure.**

**CRITICAL: FEEDBACK IS ORGANIZED AS NESTED STRUCTURE**
Previous feedback (if present) is organized as:
- [ ] Topic: Brief description
  - [ ] [EvaluatorName1] "Their concern"
  - [ ] [%s] "Your concern"
  - [ ] [EvaluatorName2] "Their concern"

**YOUR JOB:**
1. **Check previous_feedback for YOUR sub-items** - Look for sub-items tagged with [%s]
2. **Mark YOUR sub-items as [✓] FIXED if satisfied** - If you're happy with the fix, mark YOUR sub-item as complete
3. **Keep YOUR sub-items as [ ] if still needs work** - If you're not satisfied, keep YOUR sub-item open and update your concern
4. **Create NEW sub-items for new issues you find** - If you find a new issue, create a new sub-item tagged with [%s]

**If issues are found, use this format (CONTENT issues only):**
FEEDBACK CHECKLIST:
- [ ] Topic: Brief description (use existing topic from previous_feedback if reviewing same issue)
  - [✓] [%s] FIXED in v{version} - quote the CONTENT improvement (if you're satisfied with a previous concern)
  - [ ] [%s] Still needs work - quote where CONTENT issue exists (if you're still not satisfied)
  - [ ] [%s] NEW issue found - describe new CONTENT issue (if you found a new issue)

**CRITICAL RULES:**
1. **ONLY mark YOUR sub-items as complete** - Don't mark other evaluators' sub-items
2. **ALWAYS tag YOUR sub-items with [%s] as prefix** - This identifies your concerns (e.g., "[Cultural Anthropologist] feedback text")
3. **ONE field per feedback item** - If you want to address multiple fields, create SEPARATE feedback items for EACH field (e.g., one item for "Fix idiomatic_translation", another for "Fix literal_translation"). DO NOT combine multiple fields into one feedback item.
4. **Preserve the topic grouping** - Keep sub-items under the same topic if reviewing the same issue
5. **Create new topics for new issues** - If you find a completely new issue, create a new topic with your sub-item

**Example - Multiple fields (create separate items):**
❌ WRONG: "[%%s] NEW issue found - The literal_translation uses 'ignited' and the idiomatic_translation uses 'blaze,' which are semantically close. The idiomatic translation should ideally use a more distinct English idiom..."
✅ CORRECT: Create TWO separate items:
  - [ ] [%%s] NEW issue found - The idiomatic_translation uses 'blaze' which is semantically close to the literal_translation's 'ignited'. The idiomatic translation should ideally use a more distinct English idiom to maximize the difference between the translations.
  - [ ] [%%s] NEW issue found - The literal_translation uses 'ignited' which is semantically close to the idiomatic_translation's 'blaze'. Consider using a different term to maximize distinction.

CONSTRAINTS FOR NEXT VERSION (CONTENT constraints only):
(List YOUR specific constraints for the generator - what you want them to change)
- Topic: Brief constraint description
  - [%s] "Your specific CONTENT constraint with example"
  - [%s] "Your multiline CONTENT constraint that spans multiple lines should start on a new line"

ACTIONABLE STEPS (CONTENT improvement steps only):
(List YOUR specific actions for the generator - concrete steps to address your concerns)
1. Main action for topic
   - [%s] "Your specific action with concrete example from CONTENT"
   - [%s] "Your multiline action that spans multiple lines should start on a new line"

**If NO issues are found (output is perfect), use this format:**
- [✓] No issues found. The output meets all criteria and requirements.

**Remember: The feedback field must NEVER be empty. Always provide feedback, even if it's just a positive affirmation.**
**Remember: Provide feedback about CONTENT quality only - do NOT mention XML structure, field names, or technical formatting.**
**Remember: Reference specific phrases/choices from generator_output CONTENT. Be instructional, not just evaluative. Focus on %s.**
**Remember: ALWAYS tag YOUR sub-items with [%s] as prefix to indicate ownership (e.g., "[Cultural Anthropologist] feedback text").**
**Remember: ONLY mark YOUR sub-items as complete - let other evaluators mark their own.`,
		roleName,
		whatToEvaluateSection,
		rubricSection,
		roleName,   // [%s] in example (line 469)
		roleName,   // tagged with [%s] (line 473)
		roleName,   // create new sub-item tagged with [%s] (line 476)
		roleName,   // FIXED format [%s] (line 481)
		roleName,   // Still needs work [%s] (line 482)
		roleName,   // NEW issue [%s] (line 483)
		roleName,   // ALWAYS tag with [%s] (line 487)
		roleName,   // constraint [%s] (line 494)
		roleName,   // multiline constraint [%s] (line 495)
		roleName,   // step [%s] (line 500)
		roleName,   // multiline step [%s] (line 501)
		focusAreas, // Focus on %s (line 508)
		roleName,   // tag YOUR sub-items [%s] (line 509)
	)

	// Append role scope section if it exists.
	if roleScopeSection != "" {
		prompt += "\n\n" + roleScopeSection
	}

	return prompt, nil
}

// ScoreGenerationPromptBase is the shared base for all evaluator score-generation prompts.
// Pipelines add criteria-derived rubric + mapping (from BuildScoreGenerationPrompt) and optionally
// a pipeline-specific suffix (e.g. role-specific guidance). Same pattern as consolidator: base + criteria middle + optional pipeline enhancement.
const ScoreGenerationPromptBase = `[Section 1] You are a scoring evaluator.
Your role is to assign criterion scores based on the feedback analysis provided.

[Section 4] SCORING PROCESS:
1. Review the feedback provided by the feedback analysis module
2. For EACH criterion, check if the feedback identifies any issues:
   - If feedback identifies issues for a criterion → score MUST be < 2.0/2.0 (use 1.0/2.0 for minor issues, 0.0/2.0 for major issues)
   - If feedback says "no issues" or is positive/affirmative for a criterion → score CAN be 2.0/2.0
   - If feedback is silent about a criterion and output appears perfect → score CAN be 2.0/2.0
3. Assign scores that accurately reflect the severity of issues identified in the feedback

[Section 5] CRITICAL RULE:
- If feedback contains ANY "still needs work" items, "NEW" items, "CONSTRAINTS", or "ACTIONABLE STEPS" → the corresponding criteria MUST have scores < 2.0/2.0
- If feedback is ONLY positive affirmation with no improvement items → all criteria can score 2.0/2.0

[Section 6] OUTPUT FORMAT:
- Output criterion_scores map with all required criterion IDs
- Each score value MUST be a plain decimal number only (examples: 0.0, 1.0, 2.0)
- NEVER use ".", "✓", "[✓]", "[ ]", "./.", checkmarks, slashes, or other checklist punctuation as a score
- NEVER copy feedback checklist marks into criterion_scores; checklist syntax belongs only in the feedback field
- Scores MUST align with the feedback provided
- Document scoring attention in directives_ack (not a separate rationale field)`

// BuildScoreGenerationPrompt builds the full score-generation prompt: base + criteria rubric + criterion mapping + optional pipeline suffix.
// Same pattern as consolidator: shared base, then criteria-derived content, then optional pipeline-specific enhancement (suffix).
// roleName is used for context; pipelineSuffix can be "" or pipeline-specific guidance (e.g. "For Topic and Language Detector: weight topic accuracy and ideas_discussed completeness heavily.").
func (pb *PromptBuilder) BuildScoreGenerationPrompt(
	roleName string,
	criterionIDs []CriterionID,
	pipelineSuffix string,
) (string, error) {
	// Fetch criteria from registry.
	criterionList, err := pb.registry.GetMultiple(criterionIDs)
	if err != nil {
		return "", fmt.Errorf("failed to fetch criteria: %w", err)
	}

	// Group criteria by category.
	processCriteria := []CriterionDescription{}
	qualityCriteria := []CriterionDescription{}

	for _, crit := range criterionList {
		switch crit.Category {
		case CriterionCategoryProcessEvaluation:
			processCriteria = append(processCriteria, crit)
		case CriterionCategoryOutputQuality:
			qualityCriteria = append(qualityCriteria, crit)
		}
	}

	// Calculate point totals.
	processPoints := 0.0
	for _, crit := range processCriteria {
		processPoints += crit.MaxPoints
	}

	qualityPoints := 0.0
	for _, crit := range qualityCriteria {
		qualityPoints += crit.MaxPoints
	}

	// Build criteria-derived sections (rubric + mapping).
	criterionIDMapping := pb.buildCriterionIDMapping(criterionList)
	rubricSection := pb.buildRubricSection(processCriteria, qualityCriteria, processPoints, qualityPoints)

	middle := fmt.Sprintf(`[Section 2] SCORING RUBRIC (use these to assign scores):
%s

[Section 3] CRITERION ID MAPPING (use these exact IDs in criterion_scores output):
%s`,
		rubricSection,
		criterionIDMapping,
	)

	prompt := ScoreGenerationPromptBase + "\n\n" + middle
	if pipelineSuffix != "" {
		prompt = prompt + "\n\n" + pipelineSuffix
	}
	return prompt, nil
}

var defaultPromptBuilder *PromptBuilder
var defaultPromptBuilderOnce sync.Once

// GetDefaultPromptBuilder returns a shared PromptBuilder backed by DefaultRegistry (singleton).
// Use it from any pipeline to build score or feedback prompts from criterion IDs without
// creating a new registry. For RoleScopeResolver-dependent prompts (e.g. sayings feedback),
// the pipeline can set pb.RoleScopeResolver after getting the builder.
func GetDefaultPromptBuilder() (*PromptBuilder, error) {
	var err error
	defaultPromptBuilderOnce.Do(func() {
		defaultPromptBuilder, err = NewPromptBuilder(DefaultRegistry())
	})
	if err != nil {
		return nil, err
	}
	return defaultPromptBuilder, nil
}

// EvaluatorScorePromptBuilder holds all parts needed to render the evaluator score-generation prompt
// (base + criteria rubric/mapping + optional pipeline suffix). Configure it and call Build() once.
// Same pattern as consolidator: one builder with all parts, one render.
type EvaluatorScorePromptBuilder struct {
	RoleName       string
	CriterionIDs   []CriterionID
	PipelineSuffix string
}

// NewEvaluatorScorePromptBuilder returns a builder for the evaluator score prompt. Add optional suffix with WithSuffix, then call Build().
func NewEvaluatorScorePromptBuilder(roleName string, criterionIDs []CriterionID) *EvaluatorScorePromptBuilder {
	return &EvaluatorScorePromptBuilder{
		RoleName:       roleName,
		CriterionIDs:   criterionIDs,
		PipelineSuffix: "",
	}
}

// WithSuffix sets the optional pipeline-specific suffix (e.g. role-specific guidance). Chainable.
func (b *EvaluatorScorePromptBuilder) WithSuffix(suffix string) *EvaluatorScorePromptBuilder {
	b.PipelineSuffix = suffix
	return b
}

// Build renders the full prompt: base + criteria rubric/mapping + suffix. On registry/builder failure returns base only so the app keeps running.
func (b *EvaluatorScorePromptBuilder) Build() string {
	prompt, err := BuildScoreGenerationPromptFromCriteria(b.RoleName, b.CriterionIDs, b.PipelineSuffix)
	if err != nil {
		return ScoreGenerationPromptBase
	}
	return prompt
}

// EvaluatorFeedbackPromptBuilder holds all parts needed to render the evaluator feedback-analysis prompt
// (criteria-derived feedback prompt + optional pipeline suffix). Same pattern as EvaluatorScorePromptBuilder:
// one builder, one render. Pipelines that need RoleScopeResolver can inject a PromptBuilder via WithPromptBuilder.
type EvaluatorFeedbackPromptBuilder struct {
	RoleName       string
	FocusAreas     string
	CriterionIDs   []CriterionID
	PipelineSuffix string
	promptBuilder  *PromptBuilder
}

// NewEvaluatorFeedbackPromptBuilder returns a builder for the feedback-analysis prompt. Add optional suffix with WithSuffix;
// optionally set a custom PromptBuilder (e.g. with RoleScopeResolver) via WithPromptBuilder. Then call Build().
func NewEvaluatorFeedbackPromptBuilder(roleName, focusAreas string, criterionIDs []CriterionID) *EvaluatorFeedbackPromptBuilder {
	return &EvaluatorFeedbackPromptBuilder{
		RoleName:       roleName,
		FocusAreas:     focusAreas,
		CriterionIDs:   criterionIDs,
		PipelineSuffix: "",
		promptBuilder:  nil,
	}
}

// WithSuffix sets the optional pipeline-specific suffix (e.g. base prompt + feedbackRequired). Chainable.
func (b *EvaluatorFeedbackPromptBuilder) WithSuffix(suffix string) *EvaluatorFeedbackPromptBuilder {
	b.PipelineSuffix = suffix
	return b
}

// WithPromptBuilder sets the PromptBuilder to use (e.g. pipeline with RoleScopeResolver). If not set, Build uses GetDefaultPromptBuilder().
func (b *EvaluatorFeedbackPromptBuilder) WithPromptBuilder(pb *PromptBuilder) *EvaluatorFeedbackPromptBuilder {
	b.promptBuilder = pb
	return b
}

// Build renders the full feedback-analysis prompt: criteria-derived prompt + suffix. Returns error if builder or registry fails.
func (b *EvaluatorFeedbackPromptBuilder) Build() (string, error) {
	pb := b.promptBuilder
	if pb == nil {
		var err error
		pb, err = GetDefaultPromptBuilder()
		if err != nil {
			return "", fmt.Errorf("default prompt builder: %w", err)
		}
	}
	prompt, err := pb.BuildFeedbackAnalysisPrompt(b.RoleName, b.FocusAreas, b.CriterionIDs)
	if err != nil {
		return "", err
	}
	if b.PipelineSuffix != "" {
		prompt = prompt + "\n\n" + b.PipelineSuffix
	}
	return prompt, nil
}

// BuildScoreGenerationPromptFromCriteria builds the score-generation prompt using the default
// PromptBuilder: base + criteria rubric/mapping + optional pipeline suffix. Same pattern as
// consolidator (base + pipeline enhancement). Pipelines can use EvaluatorScorePromptBuilder instead for a single configure-and-render flow.
func BuildScoreGenerationPromptFromCriteria(roleName string, criterionIDs []CriterionID, pipelineSuffix string) (string, error) {
	pb, err := GetDefaultPromptBuilder()
	if err != nil {
		return "", fmt.Errorf("default prompt builder: %w", err)
	}
	return pb.BuildScoreGenerationPrompt(roleName, criterionIDs, pipelineSuffix)
}

// buildOutputQualityCriteriaNames builds a comma-separated list of output quality criteria names.
func (pb *PromptBuilder) buildOutputQualityCriteriaNames(criteria []CriterionDescription) string {
	var names []string
	for _, crit := range criteria {
		if crit.Category == CriterionCategoryOutputQuality {
			names = append(names, crit.Name)
		}
	}
	if len(names) == 0 {
		return "None specified"
	}
	return strings.Join(names, ", ")
}

// buildRubricSectionForReference generates a simplified rubric section for reference only.
// Uses a cleaner structure: concise criteria list + grouped forbidden patterns for quick scanning.
func (pb *PromptBuilder) buildRubricSectionForReference(criteria []CriterionDescription) string {
	var sb strings.Builder

	// Group by category.
	processCriteria := []CriterionDescription{}
	qualityCriteria := []CriterionDescription{}

	for _, crit := range criteria {
		switch crit.Category {
		case CriterionCategoryProcessEvaluation:
			processCriteria = append(processCriteria, crit)
		case CriterionCategoryOutputQuality:
			qualityCriteria = append(qualityCriteria, crit)
		}
	}

	// List process criteria concisely (name + description only).
	if len(processCriteria) > 0 {
		sb.WriteString(fmt.Sprintf("%s:\n", CriterionCategoryProcessEvaluation))
		for i, crit := range processCriteria {
			sb.WriteString(fmt.Sprintf("%d. %s: %s\n", i+1, crit.Name, crit.Description))
		}
		sb.WriteString("\n")
	}

	// List output quality criteria concisely (name + description only).
	if len(qualityCriteria) > 0 {
		startNum := len(processCriteria) + 1
		sb.WriteString(fmt.Sprintf("%s:\n", CriterionCategoryOutputQuality))
		for i, crit := range qualityCriteria {
			sb.WriteString(fmt.Sprintf("%d. %s: %s\n", startNum+i, crit.Name, crit.Description))
		}
		sb.WriteString("\n")

		// Add quick reference section for forbidden patterns (only for output quality criteria).
		sb.WriteString("QUICK REFERENCE: FORBIDDEN PATTERNS TO FLAG\n\n")
		sb.WriteString(pb.buildForbiddenPatternsReference(qualityCriteria))
	}

	return sb.String()
}

// buildForbiddenPatternsReference creates a quick reference of common forbidden patterns grouped by type.
func (pb *PromptBuilder) buildForbiddenPatternsReference(criteria []CriterionDescription) string {
	var sb strings.Builder

	// Check which criteria are present to determine which patterns to include.
	hasAntiFluff := false
	hasConversationalTone := false
	hasNoTherapistVoice := false
	hasInsiderPerspective := false
	hasAntiAIFeel := false

	for _, crit := range criteria {
		switch crit.ID {
		case CriterionIDAntiFluffCompliance:
			hasAntiFluff = true
		case CriterionIDConversationalTone:
			hasConversationalTone = true
		case CriterionIDNoTherapistVoice:
			hasNoTherapistVoice = true
		case CriterionIDInsiderPerspective:
			hasInsiderPerspective = true
		case CriterionIDAntiAIFeel:
			hasAntiAIFeel = true
		}
	}

	// Build quick reference grouped by pattern type.
	if hasAntiFluff {
		sb.WriteString("Abstract Verbs (Anti-Fluff):\n")
		sb.WriteString(`❌ "demonstrates", "illustrates", "serves as", "embodies", "highlights", "reflecting"` + "\n")
		sb.WriteString("✅ Use direct statements instead\n\n")

		sb.WriteString("Abstract Nouns (Anti-Fluff):\n")
		sb.WriteString(`❌ "power of", "beauty of", "essence of", "spirit of"` + "\n")
		sb.WriteString("✅ Use concrete descriptions instead\n\n")

		sb.WriteString("Meta-Commentary (Anti-Fluff):\n")
		sb.WriteString(`❌ "What this teaches...", "It's a reminder that...", "The lesson here is..."` + "\n")
		sb.WriteString("✅ State things directly, skip the frame\n\n")
	}

	if hasNoTherapistVoice {
		sb.WriteString("Therapist Voice (No Therapist Voice):\n")
		sb.WriteString(`❌ "navigating", "maintaining", "reinforcing", "social boundary", "validate", "gentle nudge"` + "\n")
		sb.WriteString("✅ Describe actions directly, don't analyze dynamics\n\n")
	}

	if hasConversationalTone {
		sb.WriteString("Formal/Academic Language (Conversational Tone):\n")
		sb.WriteString(`❌ "This Venezuelan saying acknowledges that...", "When one has already navigated..."` + "\n")
		sb.WriteString("✅ Use contractions, casual language, direct address\n\n")
	}

	if hasAntiAIFeel {
		sb.WriteString("Framing Devices (Anti-AI):\n")
		sb.WriteString(`❌ "That's the vibe of...", "Think of it as...", "It's perfect for...", "Here's what that means:", "In other words:"` + "\n")
		sb.WriteString("✅ Start directly, skip the setup\n\n")
	}

	if hasInsiderPerspective {
		sb.WriteString("Outsider Language (Insider Perspective):\n")
		sb.WriteString(`❌ "In Spanish, they have...", "Spanish speakers use...", "In their culture, they use..."` + "\n")
		sb.WriteString(`✅ "We have this saying...", "In Spanish, we say..."` + "\n\n")
	}

	return sb.String()
}

// BuildGeneratorGuidance generates guidance for generators with detailed examples.
// This includes Description + Examples for output quality criteria to help generators
// understand what evaluators will be looking for.
func (pb *PromptBuilder) BuildGeneratorGuidance(criterionIDs []CriterionID) (string, error) {
	criteria, err := pb.registry.GetMultiple(criterionIDs)
	if err != nil {
		return "", fmt.Errorf("failed to fetch criteria: %w", err)
	}

	var sb strings.Builder
	sb.WriteString("🚨 CRITICAL: OUTPUT QUALITY STANDARDS 🚨\n\n")
	sb.WriteString("Your output will be evaluated against these criteria. READ THIS CAREFULLY - evaluators will check for violations:\n\n")

	// Use separate counter for output quality criteria to ensure correct numbering.
	outputQualityNum := 1
	for _, crit := range criteria {
		// Only include output quality criteria for generators.
		if crit.Category != CriterionCategoryOutputQuality {
			continue
		}

		sb.WriteString(fmt.Sprintf("%d. %s: %s\n", outputQualityNum, crit.Name, crit.Description))
		if crit.Examples != "" {
			sb.WriteString(fmt.Sprintf("%s\n\n", crit.Examples))
		}
		outputQualityNum++
	}

	sb.WriteString("⚠️ REMINDER: When writing your TLDR section, actively avoid the forbidden patterns shown above.\n")
	sb.WriteString("⚠️ REMINDER: Use the examples as a checklist - if you see yourself writing phrases like 'demonstrates', 'highlights', 'reflecting', STOP and rewrite directly.\n\n")

	return sb.String(), nil
}
