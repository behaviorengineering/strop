package dspy

import (
	"testing"

	"github.com/behaviorengineering/strop/evaluation/criteria"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
)

func TestCreateGeneratorModule_WithoutCriteria(t *testing.T) {
	signature := core.NewSignature(
		[]core.InputField{
			{Field: core.NewField("input", core.WithDescription("Test input"))},
		},
		[]core.OutputField{
			{Field: core.NewField("output", core.WithDescription("Test output"))},
		},
	)

	module, err := CreateGeneratorModule(
		signature,
		"Test Generator",
		"",
		"Test system prompt.",
		"",
		nil,
		nil,
		nil,
		false,
	)

	require.NoError(t, err)
	assert.NotNil(t, module)
	assert.Equal(t, "DirectivesCoT", module.GetModuleType())
}

func TestCreateGeneratorModule_IncludesDirectivesProtocol(t *testing.T) {
	signature := core.NewSignature(
		[]core.InputField{{Field: core.NewField("input")}},
		[]core.OutputField{{Field: core.NewField("output")}},
	)

	module, err := CreateGeneratorModule(
		signature,
		"Objective Generator",
		"",
		"Test system prompt.",
		"",
		nil,
		nil,
		nil,
		false,
	)
	require.NoError(t, err)
	require.NotNil(t, module)
	instruction := module.GetSignature().Instruction
	assert.Contains(t, instruction, "STRUCTURED ATTENTION")
	assert.Contains(t, instruction, "VOICE")
	assert.Contains(t, instruction, "ANTI_PATTERN")
	outputs := module.GetSignature().Outputs
	require.NotEmpty(t, outputs)
	assert.Equal(t, "directives_ack", outputs[0].Field.Name)
}

func TestCreateGeneratorModule_WithHumanInstructions(t *testing.T) {
	signature := core.NewSignature(
		[]core.InputField{
			{Field: core.NewField("input")},
		},
		[]core.OutputField{
			{Field: core.NewField("output")},
		},
	)

	humanFn := func(s core.Signature) core.Signature {
		return s.WithInstruction("Extra instructions.\n\n" + s.Instruction)
	}

	module, err := CreateGeneratorModule(
		signature,
		"Human Generator",
		"",
		"Write for humans.",
		"",
		nil,
		nil,
		humanFn,
		false,
	)

	require.NoError(t, err)
	assert.NotNil(t, module)
}

func TestCreateGeneratorModule_WithCriteria(t *testing.T) {
	signature := core.NewSignature(
		[]core.InputField{
			{Field: core.NewField("input")},
		},
		[]core.OutputField{
			{Field: core.NewField("output")},
		},
	)

	criteriaGuidanceFn := func(ids []criteria.CriterionID) (string, error) {
		return "Generated guidance for " + string(ids[0]), nil
	}

	module, err := CreateGeneratorModule(
		signature,
		"Criteria Generator",
		"",
		"Generate content.",
		"",
		[]criteria.CriterionID{criteria.CriterionIDOutputQuality},
		criteriaGuidanceFn,
		nil,
		false,
	)

	require.NoError(t, err)
	assert.NotNil(t, module)
}

func TestCreateGeneratorModule_WithCriteria_NoCriteriaGuidanceFn_ReturnsError(t *testing.T) {
	signature := core.NewSignature(
		[]core.InputField{{Field: core.NewField("input")}},
		[]core.OutputField{{Field: core.NewField("output")}},
	)

	module, err := CreateGeneratorModule(
		signature,
		"Bad",
		"",
		"Prompt",
		"",
		[]criteria.CriterionID{criteria.CriterionIDOutputQuality},
		nil, // required when criterionIDs non-empty
		nil,
		false,
	)

	require.Error(t, err)
	assert.Nil(t, module)
	assert.Contains(t, err.Error(), "CriteriaGuidanceFn is required")
}
