package criteria

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatRubric_instructionCompliance(t *testing.T) {
	t.Parallel()
	text, err := FormatRubric(DefaultRegistry(), []CriterionID{CriterionIDInstructionCompliance})
	require.NoError(t, err)
	assert.Contains(t, text, "=== instruction_compliance ===")
	assert.Contains(t, text, "Instruction Compliance")
	assert.Contains(t, strings.ToLower(text), "2 points")
}

func TestFormatRubric_emptyIDs(t *testing.T) {
	t.Parallel()
	_, err := FormatRubric(DefaultRegistry(), nil)
	require.Error(t, err)
}
