package factory

import (
	"testing"
	"time"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
)

type stubLLM struct{ core.LLM }

func TestLLMFactorySetWrapLLM(t *testing.T) {
	f := NewLLMFactory(nil, time.Minute)
	var calls int
	f.SetWrapLLM(func(llm core.LLM) core.LLM {
		calls++
		return llm
	})
	if f.wrapLLM == nil {
		t.Fatal("expected wrap hook")
	}
	stub := &stubLLM{}
	if out := f.wrapLLM(stub); out != stub {
		t.Fatal("expected identity wrap")
	}
	if calls != 1 {
		t.Fatalf("calls=%d", calls)
	}
	f.SetWrapLLM(nil)
	if f.wrapLLM != nil {
		t.Fatal("expected cleared hook")
	}
}
