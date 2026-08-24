package dspy

import (
	"fmt"

	dspymodules "github.com/behaviorengineering/strop/dspy/modules"
	"github.com/behaviorengineering/strop/evaluation/criteria"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
)

// JobPrompts groups generator, evaluator (feedback), and score prompts for a refinement job.
// Shared by sayings and YouTube so generator and evaluator text stay co-located and in sync.
// Evaluator is the task-specific feedback step; ScorePrompt is the chained score step (criterion_scores).
// Jobs without evaluation use empty Evaluator; jobs using a shared score prompt may leave ScorePrompt empty.
type JobPrompts struct {
	Generator   string // Generator system prompt
	Evaluator   string // Chained evaluator feedback step (task-specific); empty if no evaluator
	ScorePrompt string // Chained evaluator score step (criterion_scores 0-10); empty to use pipeline default
}

// CriteriaGuidanceFn builds criterion-based generator guidance text.
// Callers inject their implementation (e.g. default PromptBuilder.BuildGeneratorGuidance or sayings pipeline).
type CriteriaGuidanceFn func(criterionIDs []criteria.CriterionID) (string, error)

// HumanInstructionsFn applies human-optimized writing instructions to a signature.
// Callers inject their own implementation (e.g. sayings uses SharedInstructions, research uses a simpler set).
type HumanInstructionsFn func(core.Signature) core.Signature

// GeneratorConfig is the shared configuration for all pipeline generators (sayings, YouTube, research).
// Render order: Persona (optional, at top) + SystemPrompt + GlobalRules + CriterionCustomisations; then WithXMLFormatting; then HumanInstructionsFn applied to signature.
type GeneratorConfig struct {
	Name                string
	Signature           core.Signature
	Persona             string                 // Optional: how the module should behave (objectives, tone); rendered first when non-empty
	SystemPrompt        string                 // Customised prompt (job-specific instructions)
	GlobalRules         string                 // Optional: appended after customised prompt (e.g. feedback integration)
	CriterionIDs        []criteria.CriterionID // When set, CriteriaGuidanceFn must be set; required for new pattern
	CriteriaGuidanceFn  CriteriaGuidanceFn     // Builds criterion guidance; required when CriterionIDs set
	HumanInstructionsFn HumanInstructionsFn    // Optional: human-focused writing style only (not format); applied to signature last
	// RetainRationaleAsTaskField keeps a declared rationale output as a task field after directives_ack
	// when a job still needs a separate rationale product field. Default false. Post composition uses
	// phase_plan for the 12-line skim plan instead (RetainRationaleAsTaskField remains false).
	RetainRationaleAsTaskField bool
}

// CreateModule creates a DirectivesCoT generator from this config.
// Renders prompt as: Persona (if set) + customised + GlobalRules + criterion guidance; then WithXMLFormatting; then HumanInstructionsFn(signature).
func (c GeneratorConfig) CreateModule() (core.Module, error) {
	return CreateGeneratorModule(
		c.Signature,
		c.Name,
		c.Persona,
		c.SystemPrompt,
		c.GlobalRules,
		c.CriterionIDs,
		c.CriteriaGuidanceFn,
		c.HumanInstructionsFn,
		c.RetainRationaleAsTaskField,
	)
}

// CreateGeneratorModule creates a DirectivesCoT generator (Predict + directives_ack; no stock CoT rationale).
// Shared entry point for all pipelines; lives in strop so apps and product clients can import it.

// Render order: persona (if non-empty, at top) + systemPrompt + globalRules + criterion guidance; then WithXMLFormatting; then humanInstructionsFn when non-nil.
func CreateGeneratorModule(
	signature core.Signature,
	name string,
	persona string,
	systemPrompt string,
	globalRules string,
	criterionIDs []criteria.CriterionID,
	criteriaGuidanceFn CriteriaGuidanceFn,
	humanInstructionsFn HumanInstructionsFn,
	retainRationaleAsTaskField bool,
) (core.Module, error) {
	finalPrompt := systemPrompt
	if persona != "" {
		finalPrompt = persona + "\n\n" + finalPrompt
	}
	if globalRules != "" {
		finalPrompt = finalPrompt + "\n\n" + globalRules
	}
	// Objective recitation lives in DirectivesProtocol (directives_ack); do not append the old rationale-first block.
	if len(criterionIDs) > 0 {
		if criteriaGuidanceFn == nil {
			return nil, fmt.Errorf("CriteriaGuidanceFn is required when criterionIDs is non-empty")
		}
		guidance, err := criteriaGuidanceFn(criterionIDs)
		if err != nil {
			return nil, fmt.Errorf("build generator guidance: %w", err)
		}
		finalPrompt = finalPrompt + "\n\n" + guidance
	}

	sig := signature.WithInstruction(finalPrompt)
	sig = WithXMLFormatting(sig)
	if humanInstructionsFn != nil {
		sig = humanInstructionsFn(sig)
	}

	cot := dspymodules.New(sig, dspymodules.Config{
		Name:                       name,
		RetainRationaleAsTaskField: retainRationaleAsTaskField,
	})
	return cot, nil
}
