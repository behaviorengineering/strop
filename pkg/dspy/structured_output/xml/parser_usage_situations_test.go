package xml

import (
	"strings"
	"testing"
	"time"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
)

// usageSituationsSignature matches ExplanationGenerator output for this field only (minimal parse test).
func usageSituationsSignature() core.Signature {
	return core.NewSignature(nil, []core.OutputField{
		{Field: core.Field{Name: "usage_situations", Type: core.FieldTypeText, Description: "bullet points"}},
	})
}

func testXMLConfig() XMLConfig {
	return XMLConfig{
		StrictParsing:      false,
		MaxDepth:           15,
		MaxSize:            1024 * 1024,
		ParseTimeout:       30 * time.Second,
		CustomTags:         map[string]string{},
		PreserveWhitespace: false,
	}
}

func TestParseXML_usageSituations_nestedLiBecomesJoinedString(t *testing.T) {
	p := NewXMLParser()
	sig := usageSituationsSignature()
	xmlConfig := testXMLConfig()

	raw := `<response><usage_situations><li> First scenario </li><li>Second</li></usage_situations></response>`
	fields, err := p.parseXML(raw, sig, xmlConfig)
	if err != nil {
		t.Fatalf("parseXML: %v", err)
	}
	got, ok := fields["usage_situations"].(string)
	if !ok {
		t.Fatalf("expected string, got %T", fields["usage_situations"])
	}
	if !strings.Contains(got, "First scenario") || !strings.Contains(got, "Second") {
		t.Fatalf("unexpected usage_situations: %q", got)
	}
	if strings.TrimSpace(got) == "" {
		t.Fatal("expected non-empty usage_situations")
	}
}

func TestParseXML_usageSituations_plainLinesWithoutLi(t *testing.T) {
	p := NewXMLParser()
	sig := usageSituationsSignature()
	xmlConfig := testXMLConfig()

	raw := "<response><usage_situations>\n- one\n- two\n</usage_situations></response>"
	fields, err := p.parseXML(raw, sig, xmlConfig)
	if err != nil {
		t.Fatalf("parseXML: %v", err)
	}
	got := strings.TrimSpace(fields["usage_situations"].(string))
	if got == "" || !strings.Contains(got, "one") {
		t.Fatalf("unexpected usage_situations: %q", got)
	}
}

func TestParseXML_extraNewlineJoinedArrayField(t *testing.T) {
	p := NewXMLParser()
	sig := core.NewSignature(nil, []core.OutputField{
		{Field: core.Field{Name: "my_bullets", Type: core.FieldTypeText, Description: "bullets"}},
	})
	xmlConfig := testXMLConfig()
	xmlConfig.ExtraArrayFieldsAsNewlineString = []string{"my_bullets"}

	raw := `<response><my_bullets><li>alpha</li><li>beta</li></my_bullets></response>`
	fields, err := p.parseXML(raw, sig, xmlConfig)
	if err != nil {
		t.Fatalf("parseXML: %v", err)
	}
	got, ok := fields["my_bullets"].(string)
	if !ok {
		t.Fatalf("expected string, got %T", fields["my_bullets"])
	}
	if !strings.Contains(got, "alpha") || !strings.Contains(got, "beta") {
		t.Fatalf("unexpected my_bullets: %q", got)
	}
}
