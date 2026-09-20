package xml

import (
	"context"
	"strings"
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
			raw:     `<response><directives_ack>ok</directives_ack><criterion_scores><instruction_compliance>2.0</instruction_compliance><completeness>1.5</completeness></criterion_scores></response>`,
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

// Dump shape from youtube_chapter_ideas score generation: directives_ack CDATA
// restates the XML contract and therefore contains a literal <criterion_scores>
// tag plus a <2.0 comparison. Nested score children must still parse as a map.
func TestParseOutputs_CriterionScores_CDATAMentioningFieldTag(t *testing.T) {
	p := NewXMLParser()
	sig := scoreGenerationSignature()
	cfg := testXMLConfig()
	raw := dumpScoreXMLWithFieldMentionInCDATA()

	fields, err := p.ParseOutputs(context.Background(), map[string]any{"__raw_response": raw}, sig, cfg)
	if err != nil {
		t.Fatalf("ParseOutputs: %v", err)
	}
	v, ok := fields["criterion_scores"]
	if !ok {
		t.Fatal("criterion_scores missing")
	}
	scores, isMap := v.(map[string]interface{})
	if !isMap {
		t.Fatalf("expected map[string]interface{}, got %T %#v", v, v)
	}
	for _, key := range []string{"output_quality", "instruction_compliance", "fact_checkable_ideas", "anti_fluff_compliance"} {
		if _, exists := scores[key]; !exists {
			t.Fatalf("missing key %q in %#v", key, scores)
		}
	}
}

func dumpScoreXMLWithFieldMentionInCDATA() string {
	return `<response>
  <directives_ack>
  <![CDATA[
  1. Instructions: Assign criterion scores based on provided feedback.
  3. Plan:
     4. Format as nested XML elements within <criterion_scores>.
  4. Attention check: The highest risk is providing a 2.0 score for a criterion that the feedback identifies as needing work. I will cross-reference every score against the feedback checklist.
  ]]>
  </directives_ack>
  <criterion_scores>
    <output_quality>
      <![CDATA[2.0]]>
    </output_quality>
    <instruction_compliance>
      <![CDATA[2.0]]>
    </instruction_compliance>
    <fact_checkable_ideas>
      <![CDATA[2.0]]>
    </fact_checkable_ideas>
    <anti_fluff_compliance>
      <![CDATA[2.0]]>
    </anti_fluff_compliance>
  </criterion_scores>
</response>`
}

func TestSanitize_CDATAFieldMentionDoesNotStealMapChildren(t *testing.T) {
	raw := dumpScoreXMLWithFieldMentionInCDATA()
	got := sanitizePlainTextFieldAngleBrackets(raw, []string{"directives_ack"})
	if !strings.Contains(got, "<output_quality>") {
		t.Fatalf("sanitizer dropped nested score tags; got:\n%s", got)
	}
	if !strings.Contains(got, "within <criterion_scores>.") {
		t.Fatalf("expected CDATA mention of criterion_scores to remain text; got:\n%s", got)
	}
}

func TestPlainTextFieldNames_OmitsCriterionScores(t *testing.T) {
	sig := scoreGenerationSignature()
	names := plainTextFieldNames(sig, testXMLConfig())
	for _, n := range names {
		if strings.EqualFold(n, "criterion_scores") {
			t.Fatalf("criterion_scores must not be angle-sanitized, got %v", names)
		}
	}
}

func TestParseOutputs_AfterDirectivesAckSanitize(t *testing.T) {
	p := NewXMLParser()
	sig := scoreGenerationSignature()
	cfg := testXMLConfig()
	raw := dumpScoreXMLWithFieldMentionInCDATA()
	sanitized := sanitizePlainTextFieldAngleBrackets(raw, []string{"directives_ack"})
	fields, err := p.parseXML(sanitized, sig, cfg)
	if err != nil {
		t.Fatalf("parseXML after sanitize: %v", err)
	}
	scores, isMap := fields["criterion_scores"].(map[string]interface{})
	if !isMap {
		t.Fatalf("expected map after CDATA-safe sanitize, got %T %#v", fields["criterion_scores"], fields["criterion_scores"])
	}
	if _, ok := scores["output_quality"]; !ok {
		t.Fatalf("missing output_quality in %#v", scores)
	}
}

func TestIndexOutsideCDATA_SkipsNeedleInsideCDATA(t *testing.T) {
	s := `before <![CDATA[ within <criterion_scores>. ]]><criterion_scores>after`
	got := indexOutsideCDATA(s, "<criterion_scores>")
	want := strings.LastIndex(s, "<criterion_scores>")
	if got != want {
		t.Fatalf("indexOutsideCDATA = %d, want %d (real tag after CDATA)", got, want)
	}
}
