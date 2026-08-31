package xml

import (
	"testing"

	"github.com/behaviorengineering/strop/dspy/rawresponse"
)

func TestExtractContent_ResponseBoundedIgnoresInnerGreater(t *testing.T) {
	p := NewXMLParser()
	in := `<response><chapters><item>Title ||| 00:00:00 ||| Compare a > b in the model</item></chapters><rationale>short</rationale></response>`
	got := p.ExtractContent(in)
	if got != in {
		t.Fatalf("expected full response document, got %q", got)
	}
}

func TestExtractContent_TruncatedResponseUsesSuffix(t *testing.T) {
	p := NewXMLParser()
	in := `<response><chapters><item>x</item></chapters><rationale>cut off`
	got := p.ExtractContent(in)
	if got != in {
		t.Fatalf("expected from <response> through EOF, got %q", got)
	}
}

func TestFindResponseText_AcceptsAlternateRawKey(t *testing.T) {
	p := NewXMLParser()
	out := map[string]any{
		rawresponse.AlternateKey: `<response><quotes><item>one</item></quotes><rationale>r</rationale></response>`,
	}
	got := p.FindResponseText(out)
	if got == "" {
		t.Fatal("expected non-empty text from alternate raw key")
	}
}

func TestExtractContent_StoriesWrapper(t *testing.T) {
	p := NewXMLParser()
	in := `noise <stories><story>a</story></stories> tail`
	want := `<stories><story>a</story></stories>`
	got := p.ExtractContent(in)
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestFindResponseText_PrefersResponseWrapperOverTagMention(t *testing.T) {
	p := NewXMLParser()
	ack := `4. Format as nested XML elements within <criterion_scores>.`
	doc := `<response><directives_ack>ok</directives_ack><criterion_scores><output_quality>2.0</output_quality></criterion_scores></response>`
	got := p.FindResponseText(map[string]any{
		"directives_ack": ack,
		"other":          doc,
	})
	if got != doc {
		t.Fatalf("expected full <response> document, got %q", got)
	}
}
