package rawresponse

import "testing"

func TestTextFrom_PrefersCanonicalWhenBothPresent(t *testing.T) {
	outputs := map[string]any{
		CanonicalKey: "  canonical  ",
		AlternateKey: "single",
	}
	text, key := TextFrom(outputs)
	if key != CanonicalKey || text != "canonical" {
		t.Fatalf("got text=%q key=%q", text, key)
	}
}

func TestTextFrom_UnderscoreSingleOnly(t *testing.T) {
	outputs := map[string]any{
		AlternateKey: `<response><quotes><item>a</item></quotes><rationale>r</rationale></response>`,
	}
	text, key := TextFrom(outputs)
	if key != AlternateKey || text == "" {
		t.Fatalf("got text empty=%v key=%q", text == "", key)
	}
}

func TestTextFromInterface(t *testing.T) {
	outputs := map[string]interface{}{
		AlternateKey: "hello",
	}
	text, key := TextFromInterface(outputs)
	if key != AlternateKey || text != "hello" {
		t.Fatalf("got %q %q", text, key)
	}
}
