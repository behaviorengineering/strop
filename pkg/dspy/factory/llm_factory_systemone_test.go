package factory

import (
	"context"
	"testing"

	stropdspy "github.com/behaviorengineering/strop/pkg/dspy"
)

func TestCreateLLM_rejectsSystemOneSchema(t *testing.T) {
	t.Parallel()
	f := NewLLMFactory(nil, 0)
	_, err := f.CreateLLM(context.Background(), stropdspy.ProviderConfig{
		Model:     "system-one-model",
		APISchema: apiSchemaSystemOne,
		APIKey:    "key",
		BaseURL:   "http://127.0.0.1:1320/v1/apis/systemone",
	})
	if err == nil {
		t.Fatal("expected error for systemone api_schema")
	}
}
