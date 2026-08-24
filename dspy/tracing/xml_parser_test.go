package tracing

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRemoveRawResponse(t *testing.T) {
	tests := []struct {
		name     string
		outputs  map[string]interface{}
		expected map[string]interface{}
	}{
		{
			name: "outputs with __raw_response and parsed fields",
			outputs: map[string]interface{}{
				"__raw_response": `<response><field1>value1</field1><field2>value2</field2></response>`,
				"field1":         "value1",
				"field2":         "value2",
				"other":          "other_value",
			},
			expected: map[string]interface{}{
				"field1": "value1",
				"field2": "value2",
				"other":  "other_value",
				// __raw_response should be removed.
			},
		},
		{
			name: "outputs without __raw_response",
			outputs: map[string]interface{}{
				"field1": "value1",
				"field2": "value2",
			},
			expected: map[string]interface{}{
				"field1": "value1",
				"field2": "value2",
			},
		},
		{
			name: "outputs with only __raw_response",
			outputs: map[string]interface{}{
				"__raw_response": `<response><field1>value1</field1></response>`,
			},
			expected: map[string]interface{}{
				// __raw_response should be removed, but no other fields exist.
			},
		},
		{
			name: "outputs with only _raw_response alternate key",
			outputs: map[string]interface{}{
				"_raw_response": `<response><field1>value1</field1></response>`,
			},
			expected: map[string]interface{}{},
		},
		{
			name: "outputs strip both raw keys when both present",
			outputs: map[string]interface{}{
				"__raw_response": "a",
				"_raw_response":  "b",
				"field1":         "value1",
			},
			expected: map[string]interface{}{
				"field1": "value1",
			},
		},
		{
			name:     "empty outputs",
			outputs:  map[string]interface{}{},
			expected: map[string]interface{}{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RemoveRawResponseForTest(tt.outputs)

			assert.Equal(t, tt.expected, result)
		})
	}
}
