package xml

import (
	"strings"
	"testing"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
)

func TestGenerateInstructions_FillInSkeletonForPlainFields(t *testing.T) {
	p := NewXMLParser()
	sig := core.NewSignature(
		[]core.InputField{{Field: core.NewField("input")}},
		[]core.OutputField{
			{Field: core.NewField("score", core.WithDescription("numeric score 0-10"))},
			{Field: core.NewField("feedback", core.WithDescription("actionable checklist"))},
		},
	)

	got, err := p.GenerateInstructions(sig, nil)
	if err != nil {
		t.Fatalf("GenerateInstructions: %v", err)
	}

	wantSkeleton := `<response>
  <score>
  <![CDATA[
  {{SCORE}}
  ]]>
  </score>
  <feedback>
  <![CDATA[
  {{FEEDBACK}}
  ]]>
  </feedback>
</response>`
	if !strings.Contains(got, wantSkeleton) {
		t.Fatalf("expected CDATA fill-in skeleton in instructions:\n%s\n\ngot:\n%s", wantSkeleton, got)
	}
	if !strings.Contains(got, "CDATA wraps every leaf value") {
		t.Fatalf("expected CDATA-for-all-leaves rule; got:\n%s", got)
	}
}

func TestGenerateInstructions_MapLeavesUseCDATA(t *testing.T) {
	p := NewXMLParser()
	sig := core.NewSignature(
		[]core.InputField{{Field: core.NewField("input")}},
		[]core.OutputField{
			{Field: core.NewField("directives_ack", core.WithDescription("structured attention"))},
			{Field: core.NewField("criterion_scores", core.WithDescription("map of criterion to score. Exact map keys: instruction_compliance, completeness."))},
		},
	)

	got, err := p.GenerateInstructions(sig, nil)
	if err != nil {
		t.Fatalf("GenerateInstructions: %v", err)
	}

	if !strings.Contains(got, `<directives_ack>
  <![CDATA[
  {{DIRECTIVES_ACK}}
  ]]>
  </directives_ack>`) {
		t.Fatalf("expected CDATA on directives_ack; got:\n%s", got)
	}
	wantScores := `<criterion_scores>
    <instruction_compliance>
    <![CDATA[
    {{INSTRUCTION_COMPLIANCE}}
    ]]>
    </instruction_compliance>
    <completeness>
    <![CDATA[
    {{COMPLETENESS}}
    ]]>
    </completeness>
  </criterion_scores>`
	if !strings.Contains(got, wantScores) {
		t.Fatalf("expected CDATA on map score leaves; got:\n%s", got)
	}
}

func TestGenerateInstructions_ArrayUsesCDATA(t *testing.T) {
	p := NewXMLParser()
	sig := core.NewSignature(
		[]core.InputField{{Field: core.NewField("input")}},
		[]core.OutputField{
			{Field: core.NewField("indicators", core.WithDescription("XML array (list of items)"))},
		},
	)

	got, err := p.GenerateInstructions(sig, nil)
	if err != nil {
		t.Fatalf("GenerateInstructions: %v", err)
	}
	want := `<indicators>
    <item>
    <![CDATA[
    {{ITEM}}
    ]]>
    </item>
  </indicators>`
	if !strings.Contains(got, want) {
		t.Fatalf("expected CDATA array items; got:\n%s", got)
	}
}

func TestParseXML_SanitizeBareLessThanInDirectivesAck(t *testing.T) {
	p := NewXMLParser()
	sig := core.NewSignature(nil, []core.OutputField{
		{Field: core.NewField("directives_ack", core.WithDescription("structured attention"))},
		{Field: core.NewField("criterion_scores", core.WithDescription("map of criterion to score. Exact map keys: instruction_compliance."))},
	})
	cfg := testXMLConfig()

	// Logged failure shape: almost-valid XML broken only by "< 2.0" / "<2.0" in prose.
	raw := `<response>
<directives_ack>
1. Instructions: Assign scores.
3. Plan: Assign scores (2.0 if perfect, <2.0 if issues).
4. Attention check: Ensure score is < 2.0 when feedback has NEW issues.
</directives_ack>
<criterion_scores>
<instruction_compliance>
2.0
</instruction_compliance>
</criterion_scores>
</response>`

	fields, err := p.parseXML(raw, sig, cfg)
	if err != nil {
		t.Fatalf("parseXML: %v", err)
	}
	ack, ok := fields["directives_ack"].(string)
	if !ok {
		t.Fatalf("expected directives_ack string, got %#v", fields["directives_ack"])
	}
	if !strings.Contains(ack, "<2.0") && !strings.Contains(ack, "< 2.0") {
		t.Fatalf("expected sanitized less-than to decode back into ack text, got %q", ack)
	}
	scores, ok := fields["criterion_scores"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected criterion_scores map, got %#v", fields["criterion_scores"])
	}
	switch v := scores["instruction_compliance"].(type) {
	case float64:
		if v != 2.0 {
			t.Fatalf("expected score 2.0, got %v", v)
		}
	case string:
		if strings.TrimSpace(v) != "2.0" && strings.TrimSpace(v) != "2" {
			t.Fatalf("expected score 2.0, got %q", v)
		}
	default:
		t.Fatalf("unexpected score type %#v", scores["instruction_compliance"])
	}
}

func TestParseXML_DirectivesAckCDATAAllowsLessThanComparison(t *testing.T) {
	p := NewXMLParser()
	sig := core.NewSignature(nil, []core.OutputField{
		{Field: core.NewField("directives_ack", core.WithDescription("structured attention"))},
		{Field: core.NewField("criterion_scores", core.WithDescription("map of criterion to score. Exact map keys: instruction_compliance."))},
	})
	cfg := testXMLConfig()

	raw := `<response>
<directives_ack>
<![CDATA[
Assign scores <2.0 if feedback identifies issues.
]]>
</directives_ack>
<criterion_scores>
<instruction_compliance>1.0</instruction_compliance>
</criterion_scores>
</response>`

	fields, err := p.parseXML(raw, sig, cfg)
	if err != nil {
		t.Fatalf("parseXML: %v", err)
	}
	ack, ok := fields["directives_ack"].(string)
	if !ok || !strings.Contains(ack, "<2.0") {
		t.Fatalf("expected directives_ack to keep <2.0 from CDATA, got %#v", fields["directives_ack"])
	}
}

func TestParseXML_RepairsUnclosedCDATAOnScoreLeaves(t *testing.T) {
	p := NewXMLParser()
	sig := core.NewSignature(nil, []core.OutputField{
		{Field: core.NewField("criterion_scores", core.WithDescription("map of criterion to score. Exact map keys: instruction_compliance, completeness."))},
	})
	cfg := testXMLConfig()

	// Model opened CDATA but omitted ]]> before the closing tags.
	raw := `<response>
<criterion_scores>
<instruction_compliance>
<![CDATA[
2.0
</instruction_compliance>
<completeness>
<![CDATA[
1.5
</completeness>
</criterion_scores>
</response>`

	fields, err := p.parseXML(raw, sig, cfg)
	if err != nil {
		t.Fatalf("parseXML: %v", err)
	}
	scores, ok := fields["criterion_scores"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected criterion_scores map, got %#v", fields["criterion_scores"])
	}
	switch v := scores["instruction_compliance"].(type) {
	case string:
		if strings.TrimSpace(v) != "2.0" && strings.TrimSpace(v) != "2" {
			t.Fatalf("expected instruction_compliance 2.0, got %q", v)
		}
	case float64:
		if v != 2.0 {
			t.Fatalf("expected instruction_compliance 2.0, got %v", v)
		}
	default:
		t.Fatalf("unexpected instruction_compliance type %#v", scores["instruction_compliance"])
	}
	switch v := scores["completeness"].(type) {
	case string:
		if strings.TrimSpace(v) != "1.5" {
			t.Fatalf("expected completeness 1.5, got %q", v)
		}
	case float64:
		if v != 1.5 {
			t.Fatalf("expected completeness 1.5, got %v", v)
		}
	default:
		t.Fatalf("unexpected completeness type %#v", scores["completeness"])
	}
}

func TestRepairUnclosedCDATA(t *testing.T) {
	in := `<x><![CDATA[
2.0
</x>`
	got := repairUnclosedCDATA(in)
	want := `<x><![CDATA[
2.0
]]></x>`
	if got != want {
		t.Fatalf("repairUnclosedCDATA:\nwant %#v\ngot  %#v", want, got)
	}
	already := `<x><![CDATA[2.0]]></x>`
	if repairUnclosedCDATA(already) != already {
		t.Fatalf("must leave well-formed CDATA unchanged")
	}
}
