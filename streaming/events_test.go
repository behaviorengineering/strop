package streaming

import "testing"

func TestActor_roles(t *testing.T) {
	t.Parallel()
	ev := EvaluatorActor("Prose Style Evaluator")
	if !ev.IsEvaluator() || ev.IsConsolidator() || ev.IsExpert() || ev.IsGenerator() {
		t.Fatal("evaluator actor mismatch")
	}
	cons := ConsolidatorActor("Prose Style Consolidator")
	if !cons.IsConsolidator() || cons.IsEvaluator() {
		t.Fatal("consolidator actor mismatch")
	}
	if !ExpertActor("x").IsExpert() || ExpertActor("x").IsEvaluator() {
		t.Fatal("expert actor mismatch")
	}
	if GeneratorActor("g").IsEvaluator() || (Actor{}).IsEvaluator() {
		t.Fatal("generator/zero actor must not look like evaluators")
	}
}

func TestEvaluatorStart_setsActor(t *testing.T) {
	t.Parallel()
	ev := EvaluatorStart("Prose Style Evaluator")
	if ev.Type != EventTypeModuleStart {
		t.Fatalf("type %s", ev.Type)
	}
	if !ev.Actor.IsEvaluator() {
		t.Fatal("expected evaluator actor")
	}
	if ev.ModuleName != "Prose Style Evaluator" || ev.Actor.Label() != "Prose Style Evaluator" {
		t.Fatalf("label %s / %s", ev.ModuleName, ev.Actor.Label())
	}
}

func TestActor_identityDistinguishesSameLabel(t *testing.T) {
	t.Parallel()
	a := EvaluatorActor("Prose Style Evaluator").Identity()
	b := ConsolidatorActor("Prose Style Evaluator").Identity()
	if a == b {
		t.Fatal("same label with different roles must have distinct identity")
	}
}
