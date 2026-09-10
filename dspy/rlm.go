package dspy

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
	dspyrlm "github.com/XiaoConstantine/dspy-go/pkg/modules/rlm"
)

// RLMConfig holds portable Recursive Language Model construction options.
// Hosts supply task queries and context; strop does not encode product vocabularies.
type RLMConfig struct {
	MaxIterations            int
	MaxTokens                int
	Timeout                  time.Duration
	TraceDir                 string
	Verbose                  bool
	MaxFullContextQueryChars int
	SubRLMMaxDepth           int
	SubRLMMaxIterations      int
	SubRLMMaxDirectCalls     int
	SubRLMMaxTotalCalls      int
	REPLSetup                func(repl *dspyrlm.YaegiREPL) error
}

// RLMDefaults returns conservative starter budgets for host RLM jobs.
func RLMDefaults() RLMConfig {
	return RLMConfig{
		MaxIterations:            12,
		Timeout:                  3 * time.Minute,
		MaxFullContextQueryChars: 32_000,
		SubRLMMaxDepth:           2,
		SubRLMMaxIterations:      8,
		SubRLMMaxDirectCalls:     4,
		SubRLMMaxTotalCalls:      8,
	}
}

// CreateRLMModule builds a dspy-go RLM module from an already-resolved LLM.
// Prefer factory.CreateRLM when constructing from strop ProviderConfig.
func CreateRLMModule(llm core.LLM, cfg RLMConfig) (*dspyrlm.RLM, error) {
	if llm == nil {
		return nil, fmt.Errorf("CreateRLMModule: llm is required")
	}
	return dspyrlm.NewFromLLM(llm, rlmOptions(cfg)...), nil
}

// RLMComplete runs one RLM completion and returns the final answer text.
func RLMComplete(ctx context.Context, module *dspyrlm.RLM, contextPayload any, query string) (string, *dspyrlm.CompletionResult, error) {
	if module == nil {
		return "", nil, fmt.Errorf("RLMComplete: module is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result, err := module.Complete(ctx, contextPayload, query)
	if err != nil {
		return "", nil, err
	}
	if result == nil {
		return "", nil, fmt.Errorf("RLMComplete: empty result")
	}
	return result.Response, result, nil
}

func rlmOptions(cfg RLMConfig) []dspyrlm.Option {
	defaults := RLMDefaults()
	if cfg.MaxIterations <= 0 {
		cfg.MaxIterations = defaults.MaxIterations
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaults.Timeout
	}
	if cfg.MaxFullContextQueryChars == 0 {
		cfg.MaxFullContextQueryChars = defaults.MaxFullContextQueryChars
	}
	if cfg.SubRLMMaxDepth <= 0 {
		cfg.SubRLMMaxDepth = defaults.SubRLMMaxDepth
	}
	if cfg.SubRLMMaxIterations <= 0 {
		cfg.SubRLMMaxIterations = defaults.SubRLMMaxIterations
	}
	if cfg.SubRLMMaxDirectCalls <= 0 {
		cfg.SubRLMMaxDirectCalls = defaults.SubRLMMaxDirectCalls
	}
	if cfg.SubRLMMaxTotalCalls <= 0 {
		cfg.SubRLMMaxTotalCalls = defaults.SubRLMMaxTotalCalls
	}

	opts := []dspyrlm.Option{
		dspyrlm.WithMaxIterations(cfg.MaxIterations),
		dspyrlm.WithTimeout(cfg.Timeout),
		dspyrlm.WithMaxFullContextQueryChars(cfg.MaxFullContextQueryChars),
		dspyrlm.WithSubRLMConfig(dspyrlm.SubRLMConfig{
			MaxDepth:               cfg.SubRLMMaxDepth,
			MaxIterationsPerSubRLM: cfg.SubRLMMaxIterations,
			MaxDirectSubRLMCalls:   cfg.SubRLMMaxDirectCalls,
			MaxTotalSubRLMCalls:    cfg.SubRLMMaxTotalCalls,
		}),
	}
	if cfg.MaxTokens > 0 {
		opts = append(opts, dspyrlm.WithMaxTokens(cfg.MaxTokens))
	}
	if cfg.Verbose {
		opts = append(opts, dspyrlm.WithVerbose(true))
	}
	if strings.TrimSpace(cfg.TraceDir) != "" {
		opts = append(opts, dspyrlm.WithTraceDir(cfg.TraceDir))
	}
	if cfg.REPLSetup != nil {
		opts = append(opts, dspyrlm.WithREPLSetup(cfg.REPLSetup))
	}
	return opts
}
