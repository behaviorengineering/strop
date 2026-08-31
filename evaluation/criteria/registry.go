package criteria

import (
	"fmt"
	"sync"
)

// CriterionID is a unique identifier for a criterion.
type CriterionID string

// Criterion category constants.
const (
	CriterionCategoryProcessEvaluation = "PROCESS EVALUATION"
	CriterionCategoryOutputQuality     = "OUTPUT QUALITY EVALUATION"
)

// NormalizedMaxScore is the maximum score when normalizing criterion scores to a 0-10 scale.
const NormalizedMaxScore = 10.0

const (
	// Process Evaluation Criteria.
	CriterionIDInstructionCompliance     CriterionID = "instruction_compliance"
	CriterionIDCompleteness              CriterionID = "completeness"
	CriterionIDFeedbackAdherence         CriterionID = "feedback_adherence"
	CriterionIDContextAwareness          CriterionID = "context_awareness"
	CriterionIDIterationEfficiency       CriterionID = "iteration_efficiency"
	CriterionIDDepthOfImprovement        CriterionID = "depth_of_improvement"
	CriterionIDConsistencyAcrossVersions CriterionID = "consistency_across_versions"

	// Output Quality Criteria.
	CriterionIDOutputQuality          CriterionID = "output_quality"
	CriterionIDClarityReadability     CriterionID = "clarity_readability"
	CriterionIDLearningAlignment      CriterionID = "learning_alignment"
	CriterionIDAntiFluffCompliance    CriterionID = "anti_fluff_compliance"
	CriterionIDConversationalTone     CriterionID = "conversational_tone"
	CriterionIDNoTherapistVoice       CriterionID = "no_therapist_voice"
	CriterionIDInsiderPerspective     CriterionID = "insider_perspective"
	CriterionIDAntiAIFeel             CriterionID = "anti_ai_feel"
	CriterionIDNoRedundantExplanation CriterionID = "no_redundant_explanation"

	// Output Quality Criteria (used in human approval/rejection stage).
	CriterionIDRoleSpecificQuality CriterionID = "role_specific_quality"
	CriterionIDHumanAlignment      CriterionID = "human_alignment"

	// Post-Specific Output Quality Criteria.
	CriterionIDCatchinessEngagement CriterionID = "catchiness_engagement"

	// Image Prompt: secondary text (second line) must preserve meaning—semantic or literal only, not idiomatic.
	CriterionIDSecondaryTextMeaning CriterionID = "secondary_text_meaning"

	// Chapter Ideas: sub-ideas must be self-contained for fact-checking.
	CriterionIDFactCheckableIdeas CriterionID = "fact_checkable_ideas"

	// Chapter Quotes: clip-worthy, memorable moments (not engagement).
	CriterionIDWorthyMoments CriterionID = "worthy_moments"

	// Global influence: structured comparability (affect + target level + talk-level mix), not sentiment polarity.
	CriterionIDInfluenceComparabilityProfile CriterionID = "influence_comparability_profile"

	// Topic plausibility insight criteria.
	CriterionIDInsightNovelty           CriterionID = "insight_novelty"
	CriterionIDCausalSpecificity        CriterionID = "causal_specificity"
	CriterionIDFalsifiabilityIndicators CriterionID = "falsifiability_indicators"
	CriterionIDAlternativeDistinctness  CriterionID = "alternative_distinctness"
)

// CriterionDescription contains the rubric definition for a criterion.
type CriterionDescription struct {
	ID          CriterionID
	Name        string
	Description string // What the criterion is about (concise explanation)
	Scoring     string // Scoring rubric (2 points, 1 point, 0 points)
	Examples    string // Detailed examples, forbidden patterns, before/after (for generators)
	MaxPoints   float64
	Category    string // Use CriterionCategoryProcessEvaluation or CriterionCategoryOutputQuality constants.
}

// CriterionRegistry holds all available evaluation criteria.
type CriterionRegistry struct {
	criteria map[CriterionID]CriterionDescription
	byName   map[string]CriterionID
}

// NewCriterionRegistry creates a registry with portable/generic criteria only.
// Product-specific rubrics (sayings posts, YouTube chapter/topic jobs) are registered
// by app packs onto DefaultRegistry() at container startup.
func NewCriterionRegistry() *CriterionRegistry {
	registry := &CriterionRegistry{
		criteria: make(map[CriterionID]CriterionDescription),
		byName:   make(map[string]CriterionID),
	}

	// Register portable criteria (process + shared output quality).
	registry.Register(CriterionDescription{
		ID:          CriterionIDInstructionCompliance,
		Name:        "Instruction Compliance",
		Description: `Generator followed all instructions and met all basic requirements.`,
		Scoring: `2 points: All instructions followed, all basic requirements met.
1 point: Most instructions followed but missed some requirements.
0 points: Did not follow instructions or missed critical requirements.`,
		Examples:  `Check: Required fields, format specifications, style guidelines.`,
		MaxPoints: 2.0,
		Category:  CriterionCategoryProcessEvaluation,
	})

	registry.Register(CriterionDescription{
		ID:          CriterionIDCompleteness,
		Name:        "Completeness",
		Description: `Output is complete and covers all required aspects. Encompasses output contract understanding and ensures nothing is missing.`,
		Scoring: `2 points: Output is complete, all required fields present, nothing missing.
1 point: Output is mostly complete but some aspects or fields are missing.
0 points: Output is incomplete, missing critical fields or aspects.`,
		Examples:  `Check: All specified fields populated, all required sections included, no information gaps.`,
		MaxPoints: 2.0,
		Category:  CriterionCategoryProcessEvaluation,
	})

	registry.Register(CriterionDescription{
		ID:          CriterionIDFeedbackAdherence,
		Name:        "Feedback Adherence",
		Description: `Generator addressed all previous feedback items and kept PRESERVE (user-approved) content unchanged. Only applicable if previous_feedback exists.`,
		Scoring: `2 points: Addressed all feedback items and did not change any content in [Section 2] PRESERVE INSTRUCTIONS.
1 point: Addressed most feedback but missed some, or changed one item that was marked PRESERVE.
0 points: Did not address previous feedback, or changed content that was in PRESERVE INSTRUCTIONS (user-approved).`,
		Examples:  `Check: Fixed all issues in checklist, followed all constraints, completed all actionable steps. CRITICAL: Content in [Section 2] PRESERVE INSTRUCTIONS must be kept exactly; if the generator changed it, score 0.`,
		MaxPoints: 2.0,
		Category:  CriterionCategoryProcessEvaluation,
	})

	registry.Register(CriterionDescription{
		ID:          CriterionIDContextAwareness,
		Name:        "Context Awareness",
		Description: `Generator demonstrates proper understanding and usage of context. Output (and directives_ack if present) shows it used appropriate context sources.`,
		Scoring: `2 points: Output clearly shows proper context usage and integration.
1 point: Some context awareness but could be better.
0 points: Poor context awareness, used wrong sources, or failed to use available context.`,
		Examples:  `Look for: Referenced specific details, understood nuances, used relevant sources.`,
		MaxPoints: 2.0,
		Category:  CriterionCategoryProcessEvaluation,
	})

	registry.Register(CriterionDescription{
		ID:          CriterionIDIterationEfficiency,
		Name:        "Iteration Efficiency",
		Description: `How efficiently did the generator reach approval? Fewer iterations with steady improvement = better.`,
		Scoring: `2 points: Approved in 1-2 iterations with efficient process.
1 point: Approved in 3-4 iterations with some inefficiency.
0 points: Took 5+ iterations or showed very inefficient process.`,
		Examples:  `Good: Followed instructions early, addressed feedback quickly, no repeated mistakes.`,
		MaxPoints: 2.0,
		Category:  CriterionCategoryProcessEvaluation,
	})

	registry.Register(CriterionDescription{
		ID:          CriterionIDDepthOfImprovement,
		Name:        "Depth of Improvement",
		Description: `Did the generator make meaningful, root-cause improvements or just surface-level changes?`,
		Scoring: `2 points: Deep improvements that address root causes and prevent recurrence.
1 point: Some meaningful improvements but also some surface-level changes.
0 points: Only superficial changes, doesn't address root causes, same issues recur.`,
		Examples:  `Deep: Fixed fundamental problems, addressed why issues occurred. Surface: Only changed wording.`,
		MaxPoints: 2.0,
		Category:  CriterionCategoryProcessEvaluation,
	})

	registry.Register(CriterionDescription{
		ID:          CriterionIDConsistencyAcrossVersions,
		Name:        "Consistency Across Versions",
		Description: `Did the generator maintain consistent quality and approach across versions, or did quality fluctuate/regress?`,
		Scoring: `2 points: Consistent quality/approach across all versions, no regressions.
1 point: Some variation but overall stable, minor regressions quickly corrected.
0 points: Significant quality drops, regressions, or high fluctuation.`,
		Examples:  `Good: Quality maintained or improved each version, no backsliding. Bad: Introduced new problems.`,
		MaxPoints: 2.0,
		Category:  CriterionCategoryProcessEvaluation,
	})

	registry.Register(CriterionDescription{
		ID:          CriterionIDOutputQuality,
		Name:        "Output Quality",
		Description: `Output is excellent, meets all quality standards for this role's focus areas.`,
		Scoring: `2 points: Output is excellent - accurate, appropriate, grammatically correct.
1 point: Output is good but has minor quality issues.
0 points: Output has significant quality issues.`,
		Examples:  `Check: Accuracy, appropriateness, grammar, adherence to role-specific standards.`,
		MaxPoints: 2.0,
		Category:  CriterionCategoryOutputQuality,
	})

	registry.Register(CriterionDescription{
		ID:          CriterionIDClarityReadability,
		Name:        "Clarity and Readability",
		Description: `Output is clear, readable, and well-communicated. Ideas are easy to understand.`,
		Scoring: `2 points: Exceptionally clear and readable - ideas are easy to understand, natural flow.
1 point: Mostly clear but has issues (awkward structure, unclear phrasing, confusing ideas).
0 points: Unclear, hard to read, or poorly communicated.`,
		Examples: `Good: Clear ideas, natural flow, easy to comprehend.
Note: This evaluates clarity of communication, not grammatical correctness (that's Instruction Compliance).`,
		MaxPoints: 2.0,
		Category:  CriterionCategoryOutputQuality,
	})

	registry.Register(CriterionDescription{
		ID:          CriterionIDLearningAlignment,
		Name:        "Learning Alignment",
		Description: `Output aligns with human learning needs and educational value. Would humans learn effectively from this?`,
		Scoring: `2 points: Strongly aligned with learning, high educational value.
1 point: Somewhat aligned but could be better for learning.
0 points: Poor alignment with learning needs, low educational value.`,
		Examples:  `Good: Clear explanations, helpful examples, teaches effectively. Bad: Confusing, unhelpful.`,
		MaxPoints: 2.0,
		Category:  CriterionCategoryOutputQuality,
	})

	registry.Register(CriterionDescription{
		ID:          CriterionIDAntiFluffCompliance,
		Name:        "Anti-Fluff Compliance",
		Description: `Content avoids abstract phrases, preachy language, and flowery prose.`,
		Scoring: `2 points: No abstract phrases or preachy language, direct and concrete.
1 point: Some abstract phrases or preachy language present (1-2 instances).
0 points: Heavy use of abstract phrases or preachy language (3+ instances).`,
		Examples: `FORBIDDEN PATTERNS:
• Abstract verbs: "demonstrates", "illustrates", "serves as", "embodies"
• Abstract nouns: "power of", "beauty of", "essence of", "spirit of"
• Meta-commentary: "What this teaches...", "It's a reminder that...", "The lesson here is..."

FIXES (minimal change needed):
❌ "This text demonstrates resilience"
✅ "This text is about resilience"

❌ "What this teaches us is that we're stronger than we think"
✅ "We're stronger than we think"

EDGE CASE:
⚠️ OK: "This is about..." (descriptive, not abstract)
❌ BAD: "This serves as a reminder..." (abstract framing)`,
		MaxPoints: 2.0,
		Category:  CriterionCategoryOutputQuality,
	})

	registry.Register(CriterionDescription{
		ID:          CriterionIDConversationalTone,
		Name:        "Conversational Tone (Bar Test)",
		Description: `Content is direct and conversational: what you'd actually say to a friend at a bar. No teaching or explanatory frames; say it straight, not like a lesson or textbook.`,
		Scoring: `2 points: Direct and conversational; no teaching frames; passes bar test.
1 point: Mostly conversational but 1-2 phrases are formal or teaching-style (e.g. "In English, you'd say...").
0 points: Fails bar test; sounds like a lesson, textbook, or academic.`,
		Examples: `THE BAR TEST: Would you actually say this to a friend at a bar? Direct, not explanatory.

PASSES (direct, no teaching frame):
✅ "Ever had so much on your plate that one more thing barely moves the needle?"
✅ "You're already dealing with a ton of stuff, so one more thing? Whatever."
✅ "Same idea as 'so hungry you could eat a horse.'" or "You're so hungry you could eat a horse, that kind of hungry."

FAILS (teaching frame, not direct):
❌ "In English, you'd say you're so hungry you could eat a horse."
❌ "This expression acknowledges that accustomed resilience means..."
❌ "When one has already navigated significant challenges..."

KEY INDICATORS:
• PASSES: Direct address, contractions, casual language, no "In English you'd say..." or similar teaching frames
• FAILS: Teaching/explanatory frames, formal language, third-person, academic phrasing

EDGE CASE:
⚠️ OK: "This expression means..." (informative introduction)
❌ BAD: "This proverb demonstrates..." (academic tone)
❌ BAD: "In English, you'd say..." (teaching frame; weave the equivalent in directly instead)`,
		MaxPoints: 2.0,
		Category:  CriterionCategoryOutputQuality,
	})

	registry.Register(CriterionDescription{
		ID:          CriterionIDNoTherapistVoice,
		Name:        "No Therapist Voice",
		Description: `Content avoids therapist/self-help language that analyzes social dynamics.`,
		Scoring: `2 points: No therapist voice phrases, no meta-commentary about social dynamics.
1 point: Some therapist voice phrases present (1-2 instances).
0 points: Heavy use of therapist/self-help language or meta-commentary (3+ instances).`,
		Examples: `FORBIDDEN PATTERNS:
• Process verbs: "navigating", "maintaining", "reinforcing"
• Therapy jargon: "social boundary", "validate", "admonishment", "gentle nudge"
• Social analysis: "social dynamics", "keep grounded", "maintain harmony"

FIXES (describe, don't analyze):
❌ "This helps us navigate social boundaries by delivering a gentle nudge"
✅ "You're telling someone to chill out, but in a way that doesn't start a fight"

❌ "It reinforces bonds through shared humor while maintaining social harmony"
✅ "You're making fun of them, but everyone laughs"

EDGE CASE:
⚠️ OK: "You're making fun of them" (describes action)
❌ BAD: "You're playfully nudging them back into line" (analyzes dynamic)`,
		MaxPoints: 2.0,
		Category:  CriterionCategoryOutputQuality,
	})

	registry.Register(CriterionDescription{
		ID:          CriterionIDInsiderPerspective,
		Name:        "Insider Perspective",
		Description: `Content maintains consistent first-person insider voice when speaking about the culture - uses "we/our" language, not "they/their" observational language.`,
		Scoring: `2 points: Consistently uses insider perspective ("we/our").
1 point: Mostly insider but occasionally slips to third-person (1-2 instances).
0 points: Uses third-person observational language ("they/their").`,
		Examples: `INSIDER (speak as a member of the culture):
✅ "We have this expression..."
✅ "In our language, we say..."
✅ "Our culture has this expression..."

OUTSIDER (speak as an observer):
❌ "In that language, they have this saying..."
❌ "In their culture, they use..."
❌ "Speakers of that language have this expression..."

EDGE CASE:
⚠️ OK: "This is an expression from that culture..." (neutral description)
❌ BAD: "In their culture, they use..." (outsider perspective)`,
		MaxPoints: 2.0,
		Category:  CriterionCategoryOutputQuality,
	})

	registry.Register(CriterionDescription{
		ID:          CriterionIDAntiAIFeel,
		Name:        "Anti-AI Feel / Human Voice",
		Description: `Content sounds authentically human, not like polished marketing copy or textbook prose. Uses natural, colloquial language that real people actually use.`,
		Scoring: `2 points: Sounds like a real person talking - casual language, contractions, natural flow, uses colloquial words naturally.
1 point: Mostly human-sounding but has 1-2 polished/overly formal phrases.
0 points: Sounds like marketing copy or over-polished prose (3+ violations).`,
		Examples: `FORBIDDEN FRAMING DEVICES (most common AI tell):
• "That's the vibe of...", "Think of it as...", "It's perfect for..."
• "Here's what that means:", "In other words:"

OVERLY FORMAL LANGUAGE (AI avoids natural colloquialisms):
❌ "More intensely burning" (overly formal when "more lit" is natural)
❌ "Extremely energized" (formal when "hyped up" is more natural)
❌ "Intoxicated but active" (formal when "turnt up" is more natural)
✅ "More lit" (natural, colloquial, grammatically correct)
✅ "Hyped up" (natural, colloquial, grammatically correct)
✅ "Turnt up" (natural, colloquial, grammatically correct)

NATURAL LANGUAGE PRINCIPLE (CRITICAL):
• Use the words real people actually use ("lit", "turnt", "hyped", "stoked")
• Don't avoid colloquialisms just because they're informal
• Grammatically correct ≠ formal
• If a colloquial word is grammatically valid, it's PREFERRED over a formal equivalent
• Avoiding colloquialisms is an AI tell - humans use informal language naturally

FIXES (start directly, skip the frame):
❌ "That's the vibe of this expression..."
✅ "You're dealing with so much already..."

❌ "It's perfect for when an extra task feels insignificant"
✅ "When an extra task lands, you just shrug it off"

MARKETING PHRASES (force enthusiasm):
❌ "fantastic way to", "wonderfully absurd", "inject humor and warmth"
✅ "silly little way", "ridiculous", "lighten things up"

EDGE CASE:
⚠️ OK: "This is about..." (informative, not framing)
❌ BAD: "That's the essence of this expression" (meta-framing)`,
		MaxPoints: 2.0,
		Category:  CriterionCategoryOutputQuality,
	})

	registry.Register(CriterionDescription{
		ID:          CriterionIDNoRedundantExplanation,
		Name:        "No Redundant Explanation / Information Density",
		Description: `Content avoids restating the same idea, explaining self-evident things, or low information density. Each sentence adds new information.`,
		Scoring: `2 points: High information density - each sentence adds new information.
1 point: Mostly efficient but occasionally restates ideas (1-2 instances).
0 points: Low information density - multiple redundancies (3+ instances).`,
		Examples: `REDUNDANCY PATTERNS:
• Restating the meaning after already explaining it
• Explaining that something is a joke when it's obviously absurd
• Repeating the same concept with different words

FIXES (trust the reader):
❌ "You're telling someone to calm down. It's a way to tell them to ease up. You're basically saying they should relax."
✅ "You're telling someone to calm down when they're being too dramatic."

❌ "The image is ridiculous - a tiger with stripes. This is clearly a joke. We're being playful here."
✅ "The image is ridiculous - a tiger with stripes."

EDGE CASE:
⚠️ OK: Expanding on an idea with new information
❌ BAD: Restating the same idea with synonyms`,
		MaxPoints: 2.0,
		Category:  CriterionCategoryOutputQuality,
	})

	registry.Register(CriterionDescription{
		ID:          CriterionIDRoleSpecificQuality,
		Name:        "Role-Specific Quality",
		Description: `Output excels in role-specific focus areas.`,
		Scoring: `2 points: Output excels in role-specific focus areas.
1 point: Output is good in role-specific areas but has room for improvement.
0 points: Output does not meet role-specific standards.`,
		Examples:  `Good: Demonstrates expertise, deep understanding, meets/exceeds expectations.`,
		MaxPoints: 2.0,
		Category:  CriterionCategoryOutputQuality,
	})

	registry.Register(CriterionDescription{
		ID:          CriterionIDHumanAlignment,
		Name:        "Human Alignment",
		Description: `How well the output resonates with human judgment. Would a human approve this?`,
		Scoring: `2 points: Strongly resonates with human judgment.
1 point: Somewhat resonates but could be better.
0 points: Does not resonate with human judgment.`,
		Examples:  `Good: Feels natural, appropriate, matches human preferences. Bad: Feels unnatural, inappropriate.`,
		MaxPoints: 2.0,
		Category:  CriterionCategoryOutputQuality,
	})

	return registry
}

// Register adds a criterion to the registry.
func (r *CriterionRegistry) Register(criterion CriterionDescription) {
	if r.byName == nil {
		r.byName = make(map[string]CriterionID)
	}
	r.criteria[criterion.ID] = criterion
	r.byName[criterion.Name] = criterion.ID
}

// GetByName retrieves a criterion by its display name (exact match).
func (r *CriterionRegistry) GetByName(name string) (CriterionDescription, error) {
	if r.byName == nil {
		return CriterionDescription{}, fmt.Errorf("criterion not found by name: %q", name)
	}
	id, ok := r.byName[name]
	if !ok {
		return CriterionDescription{}, fmt.Errorf("criterion not found by name: %q", name)
	}
	return r.Get(id)
}

// Get retrieves a criterion by ID.
func (r *CriterionRegistry) Get(id CriterionID) (CriterionDescription, error) {
	criterion, ok := r.criteria[id]
	if !ok {
		return CriterionDescription{}, fmt.Errorf("criterion not found: %s", id)
	}
	return criterion, nil
}

// GetMultiple retrieves multiple criteria by ID (in order).
func (r *CriterionRegistry) GetMultiple(ids []CriterionID) ([]CriterionDescription, error) {
	criteria := make([]CriterionDescription, 0, len(ids))
	for _, id := range ids {
		criterion, err := r.Get(id)
		if err != nil {
			return nil, err
		}
		criteria = append(criteria, criterion)
	}
	return criteria, nil
}

var defaultRegistry *CriterionRegistry
var defaultRegistryOnce sync.Once

// DefaultRegistry returns a shared criterion registry (singleton). Use it when building
// score or feedback prompts so all pipelines share the same criterion definitions.
func DefaultRegistry() *CriterionRegistry {
	defaultRegistryOnce.Do(func() {
		defaultRegistry = NewCriterionRegistry()
	})
	return defaultRegistry
}
