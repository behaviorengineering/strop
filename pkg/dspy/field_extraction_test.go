package dspy

import (
	"reflect"
	"testing"
)

func TestCoerceCriterionScoresMap(t *testing.T) {
	t.Parallel()

	got, err := CoerceCriterionScoresMap(map[string]string{"instruction_compliance": "2.0"})
	if err != nil {
		t.Fatalf("map[string]string: %v", err)
	}
	if got["instruction_compliance"] != "2.0" {
		t.Fatalf("unexpected value: %#v", got)
	}

	got, err = CoerceCriterionScoresMap(`{"instruction_compliance": 2.0, "completeness": 1.5}`)
	if err != nil {
		t.Fatalf("json string: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 keys, got %#v", got)
	}

	_, err = CoerceCriterionScoresMap("not-json")
	if err == nil {
		t.Fatal("expected error for plain string")
	}
}

func TestToSnakeCase(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple PascalCase",
			input:    "LiteralTranslation",
			expected: "literal_translation",
		},
		{
			name:     "single word",
			input:    "Translation",
			expected: "translation",
		},
		{
			name:     "three words",
			input:    "CoreMeaning",
			expected: "core_meaning",
		},
		{
			name:     "consecutive uppercase",
			input:    "XMLParser",
			expected: "x_m_l_parser",
		},
		{
			name:     "ID in middle",
			input:    "UserID",
			expected: "user_i_d",
		},
		{
			name:     "all lowercase",
			input:    "translation",
			expected: "translation",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "single uppercase",
			input:    "A",
			expected: "a",
		},
		{
			name:     "mixed case with numbers",
			input:    "Field1Name",
			expected: "field1_name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toSnakeCase(tt.input)
			if result != tt.expected {
				t.Errorf("toSnakeCase(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestMapStructFromMap(t *testing.T) {
	type TestStruct struct {
		LiteralTranslation   string
		SemanticTranslation  string
		IdiomaticTranslation string
		OptionalField        string
	}

	tests := []struct {
		name    string
		input   map[string]interface{}
		wantErr bool
		verify  func(*TestStruct) bool
	}{
		{
			name: "all fields present",
			input: map[string]interface{}{
				"literal_translation":   "literal",
				"semantic_translation":  "semantic",
				"idiomatic_translation": "idiomatic",
				"optional_field":        "optional",
			},
			wantErr: false,
			verify: func(s *TestStruct) bool {
				return s.LiteralTranslation == "literal" &&
					s.SemanticTranslation == "semantic" &&
					s.IdiomaticTranslation == "idiomatic" &&
					s.OptionalField == "optional"
			},
		},
		{
			name: "missing optional field",
			input: map[string]interface{}{
				"literal_translation":   "literal",
				"semantic_translation":  "semantic",
				"idiomatic_translation": "idiomatic",
			},
			wantErr: false,
			verify: func(s *TestStruct) bool {
				return s.LiteralTranslation == "literal" &&
					s.SemanticTranslation == "semantic" &&
					s.IdiomaticTranslation == "idiomatic" &&
					s.OptionalField == ""
			},
		},
		{
			name:    "nil map",
			input:   nil,
			wantErr: true,
		},
		{
			name: "wrong type",
			input: map[string]interface{}{
				"literal_translation": 123, // Should be string
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := MapStructFromMap[TestStruct](tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("MapStructFromMap() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && result != nil {
				if !tt.verify(result) {
					t.Errorf("MapStructFromMap() result verification failed")
				}
			}
		})
	}
}

func TestMapStructFromMapRequired(t *testing.T) {
	type TestStruct struct {
		RequiredField1 string
		RequiredField2 string
	}

	tests := []struct {
		name    string
		input   map[string]interface{}
		wantErr bool
	}{
		{
			name: "all required fields present",
			input: map[string]interface{}{
				"required_field1": "value1",
				"required_field2": "value2",
			},
			wantErr: false,
		},
		{
			name: "missing required field",
			input: map[string]interface{}{
				"required_field1": "value1",
				// required_field2 missing
			},
			wantErr: true,
		},
		{
			name:    "nil map",
			input:   nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := MapStructFromMapRequired[TestStruct](tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("MapStructFromMapRequired() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && result == nil {
				t.Errorf("MapStructFromMapRequired() returned nil result when no error expected")
			}
		})
	}
}

func TestMapStructFromMap_OnlyStringFields(t *testing.T) {
	type TestStruct struct {
		StringField string
		IntField    int
		BoolField   bool
	}

	input := map[string]interface{}{
		"string_field": "value",
		"int_field":    123,
		"bool_field":   true,
	}

	result, err := MapStructFromMap[TestStruct](input)
	if err != nil {
		t.Fatalf("MapStructFromMap() error = %v", err)
	}

	if result.StringField != "value" {
		t.Errorf("StringField = %q, want %q", result.StringField, "value")
	}

	// Non-string fields should remain zero values
	if result.IntField != 0 {
		t.Errorf("IntField = %d, want 0", result.IntField)
	}
	if result.BoolField != false {
		t.Errorf("BoolField = %v, want false", result.BoolField)
	}
}

func TestMapStructFromMap_UnexportedFields(t *testing.T) {
	type TestStruct struct {
		ExportedField   string
		unexportedField string // intentionally unexported; must remain zero when map has unexported_field
	}

	input := map[string]interface{}{
		"exported_field":   "value1",
		"unexported_field": "value2",
	}

	result, err := MapStructFromMap[TestStruct](input)
	if err != nil {
		t.Fatalf("MapStructFromMap() error = %v", err)
	}

	if result.ExportedField != "value1" {
		t.Errorf("ExportedField = %q, want %q", result.ExportedField, "value1")
	}
	// Unexported field should remain zero value (map has unexported_field key but field is not set).
	if result.unexportedField != "" {
		t.Errorf("unexportedField should be zero value, got %q", result.unexportedField)
	}

	// Should not panic.
	v := reflect.ValueOf(result).Elem()
	unexportedField := v.FieldByName("unexportedField")
	if unexportedField.String() != "" {
		t.Errorf("unexportedField should remain zero value")
	}
}
