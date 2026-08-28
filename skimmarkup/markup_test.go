package skimmarkup_test

import (
	"strings"
	"testing"

	"github.com/behaviorengineering/strop/skimmarkup"
)

func TestStripBoldItalic(t *testing.T) {
	t.Parallel()
	src := "**Roger Conroy** asks HR, **with the PIP still framing Hector as the delivery problem**"
	want := "Roger Conroy asks HR, with the PIP still framing Hector as the delivery problem"
	if got := skimmarkup.Strip(src); got != want {
		t.Fatalf("Strip() = %q, want %q", got, want)
	}
}

func TestNormalizePlainCollapsesWhitespace(t *testing.T) {
	t.Parallel()
	src := "  **Alpha**   beta   "
	want := "Alpha beta"
	if got := skimmarkup.NormalizePlain(src); got != want {
		t.Fatalf("NormalizePlain() = %q, want %q", got, want)
	}
}

func TestFidelityOK(t *testing.T) {
	t.Parallel()
	plain := "Roger Conroy asks HR to confirm the PIP scope, with the PIP still framing Hector as the delivery problem"
	marked := "**Roger Conroy** asks HR to confirm the PIP scope, **with the PIP still framing Hector as the delivery problem**"
	if !skimmarkup.FidelityOK(plain, marked) {
		t.Fatal("expected fidelity OK")
	}
}

func TestFidelityFailOnRewrite(t *testing.T) {
	t.Parallel()
	plain := "Roger Conroy asks HR to confirm the PIP scope"
	marked := "**Roger Conroy** asks HR to confirm scope"
	if skimmarkup.FidelityOK(plain, marked) {
		t.Fatal("expected fidelity fail on dropped words")
	}
}

func TestValidateFidelity(t *testing.T) {
	t.Parallel()
	err := skimmarkup.ValidateFidelity("Alpha beta.", "**Alpha** beta.")
	if err != nil {
		t.Fatalf("ValidateFidelity: %v", err)
	}
	err = skimmarkup.ValidateFidelity("Alpha beta.", "**Alpha** gamma.")
	if err == nil || !strings.Contains(err.Error(), "fidelity failed") {
		t.Fatalf("ValidateFidelity rewrite: got %v", err)
	}
}
