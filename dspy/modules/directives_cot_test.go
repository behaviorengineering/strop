package modules_test

import (
	"strings"
	"testing"

	dspymodules "github.com/behaviorengineering/strop/dspy/modules"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
)

func TestDirectivesCoT_AckFirstNoRationale(t *testing.T) {
	t.Parallel()
	sig := core.NewSignature(
		[]core.InputField{{Field: core.NewField("q")}},
		[]core.OutputField{
			{Field: core.NewField("rationale", core.WithDescription("should be stripped"))},
			{Field: core.NewField("summary")},
		},
	).WithInstruction("Task: fill summary only.")

	mod := dspymodules.New(sig, dspymodules.Config{Name: "TestCoT"})
	if mod.GetModuleType() != "DirectivesCoT" {
		t.Fatalf("type=%q", mod.GetModuleType())
	}
	got := mod.GetSignature()
	if len(got.Outputs) < 2 || got.Outputs[0].Field.Name != dspymodules.DirectivesAckField {
		t.Fatalf("outputs=%+v", got.Outputs)
	}
	if got.Outputs[1].Field.Name != "summary" {
		t.Fatalf("second=%q", got.Outputs[1].Field.Name)
	}
	for _, f := range got.Outputs {
		if f.Field.Name == "rationale" {
			t.Fatal("rationale must not appear when RetainRationaleAsTaskField is false")
		}
	}
	if !strings.Contains(got.Instruction, "STRUCTURED ATTENTION") {
		t.Fatal("missing protocol")
	}
	if !strings.Contains(got.Instruction, "Task: fill summary only.") {
		t.Fatalf("missing task rules: %q", got.Instruction)
	}
}

func TestDirectivesCoT_RetainRationaleAsTaskField(t *testing.T) {
	t.Parallel()
	sig := core.NewSignature(
		nil,
		[]core.OutputField{
			{Field: core.NewField("rationale", core.WithDescription("12-line plan"))},
			{Field: core.NewField("description")},
		},
	).WithInstruction("Write post.")

	mod := dspymodules.New(sig, dspymodules.Config{
		Name:                       "Post",
		RetainRationaleAsTaskField: true,
	})
	got := mod.GetSignature()
	if len(got.Outputs) != 3 {
		t.Fatalf("want 3 outputs, got %+v", got.Outputs)
	}
	if got.Outputs[0].Field.Name != dspymodules.DirectivesAckField {
		t.Fatalf("first=%q", got.Outputs[0].Field.Name)
	}
	if got.Outputs[1].Field.Name != "rationale" {
		t.Fatalf("second=%q want rationale", got.Outputs[1].Field.Name)
	}
	if got.Outputs[2].Field.Name != "description" {
		t.Fatalf("third=%q", got.Outputs[2].Field.Name)
	}
}

func TestPredictOf_DirectivesCoT(t *testing.T) {
	t.Parallel()
	sig := core.NewSignature(nil, []core.OutputField{{Field: core.NewField("summary")}})
	mod := dspymodules.New(sig, dspymodules.Config{Name: "X"})
	predict, err := dspymodules.PredictOf(mod)
	if err != nil {
		t.Fatal(err)
	}
	if predict == nil {
		t.Fatal("nil predict")
	}
}
