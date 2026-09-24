package workflow

import (
	"context"
	"encoding/json"
	"strings"

	stropdspy "github.com/behaviorengineering/strop/pkg/dspy"
	"github.com/behaviorengineering/strop/pkg/evaluation"
	"github.com/behaviorengineering/strop/pkg/runreport"
)

func (w *ParallelEvaluationWorkflow) logEvaluatorPayloadSizes(ctx context.Context, roleKey evaluation.EvaluatorKey, inputs map[string]interface{}, parseOK bool) {
	if w.logger == nil {
		return
	}
	genInBytes, genOutBytes := evaluationPayloadByteSizes(inputs)
	fields := map[string]interface{}{
		"evaluator":              roleKey.String(),
		"generator_input_bytes":  genInBytes,
		"generator_output_bytes": genOutBytes,
		"parse_ok":               parseOK,
	}
	phase := compositionPhaseFromEvalInputs(inputs)
	if phase != "" {
		fields["composition_phase"] = phase
	}
	w.logger.WithFields(fields).Info("Evaluation agent payload")
	if c := runreport.CollectorFromContext(ctx); c != nil {
		c.RecordEvaluator(roleKey.String(), phase, parseOK, fields)
	}
}

func evaluationPayloadByteSizes(inputs map[string]interface{}) (generatorInputBytes, generatorOutputBytes int) {
	if inputs == nil {
		return 0, 0
	}
	if genIn, ok := inputs[stropdspy.FieldGeneratorInput]; ok {
		generatorInputBytes = jsonMapByteSize(genIn)
	}
	if genOut, ok := inputs[stropdspy.FieldGeneratorOutput]; ok {
		generatorOutputBytes = jsonMapByteSize(genOut)
	}
	return generatorInputBytes, generatorOutputBytes
}

func compositionPhaseFromEvalInputs(inputs map[string]interface{}) string {
	genIn, ok := inputs[stropdspy.FieldGeneratorInput].(map[string]interface{})
	if !ok {
		return ""
	}
	phase, ok := genIn["composition_phase"].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(phase)
}

func jsonMapByteSize(value interface{}) int {
	b, err := json.Marshal(value)
	if err != nil {
		return 0
	}
	return len(b)
}
