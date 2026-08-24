package xml

import (
	"context"
	"testing"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
)

func scoreGenerationSignature() core.Signature {
	return core.NewSignature(
		nil,
		[]core.OutputField{
			{Field: core.NewField("directives_ack", core.WithDescription("structured attention"))},
			{Field: core.NewField("criterion_scores", core.WithDescription("Map of individual criterion scores. Each key is a criterion ID and each value is the score (0.0 to max_points)."))},
		},
	)
}

func TestParseOutputs_CriterionScores_MapTypes(t *testing.T) {
	p := NewXMLParser()
	sig := scoreGenerationSignature()
	cfg := testXMLConfig()

	cases := map[string]struct {
		raw      string
		wantMap  bool
		wantKeys []string
	}{
		"nested_tags": {
			raw: `<response><directives_ack>ok</directives_ack><criterion_scores><instruction_compliance>2.0</instruction_compliance><completeness>1.5</completeness></criterion_scores></response>`,
			wantMap: true, wantKeys: []string{"instruction_compliance", "completeness"},
		},
		"cdata_scalar_in_parent": {
			raw:     `<response><directives_ack>ok</directives_ack><criterion_scores><![CDATA[2.0]]></criterion_scores></response>`,
			wantMap: true, // empty map today
		},
		"json_text_in_parent": {
			raw:      `<response><directives_ack>ok</directives_ack><criterion_scores>{"instruction_compliance": 2.0, "completeness": 1.5}</criterion_scores></response>`,
			wantMap:  true,
			wantKeys: []string{"instruction_compliance", "completeness"},
		},
		"nested_cdata_leaf": {
			raw:      `<response><directives_ack>ok</directives_ack><criterion_scores><instruction_compliance><![CDATA[2.0]]></instruction_compliance></criterion_scores></response>`,
			wantMap:  true,
			wantKeys: []string{"instruction_compliance"},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			fields, err := p.ParseOutputs(context.Background(), map[string]any{"__raw_response": tc.raw}, sig, cfg)
			if err != nil {
				t.Fatalf("ParseOutputs: %v", err)
			}
			v, ok := fields["criterion_scores"]
			if !ok {
				t.Fatal("criterion_scores missing")
			}
			scores, isMap := v.(map[string]interface{})
			if tc.wantMap && !isMap {
				t.Fatalf("expected map[string]interface{}, got %T %#v", v, v)
			}
			for _, key := range tc.wantKeys {
				if _, exists := scores[key]; !exists {
					t.Fatalf("missing key %q in %#v", key, scores)
				}
			}
		})
	}
}
