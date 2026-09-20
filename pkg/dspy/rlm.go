package dspy

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
	dspyrlm "github.com/XiaoConstantine/dspy-go/pkg/modules/rlm"
	"github.com/google/uuid"
)

// RLMInputsDumpFile is the append-only JSONL sidecar written beside TraceDir
// sessions. dspy-go metadata truncates context to ~500 chars for display;
// this file keeps the full context and query for local AI replay.
const RLMInputsDumpFile = "rlm_inputs.jsonl"

// RLMConfig holds portable Recursive Language Model construction options.
// Hosts supply task queries and context; strop does not encode product vocabularies.
// Recommended construction (config create): set LLM and budgets on the config, then call CreateModule.
type RLMConfig struct {
	// LLM is required for CreateModule. Resolve via factory.LLMFactory (or factory.CreateRLM), then set.
	LLM                      core.LLM
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

// CreateModule builds a dspy-go RLM from this config (same rhythm as GeneratorConfig.CreateModule).
// LLM must be set on the config. Hosts then call RLMComplete with context and query.
func (c RLMConfig) CreateModule() (*dspyrlm.RLM, error) {
	return CreateRLMModule(c.LLM, c)
}

// CreateRLMModule builds a dspy-go RLM module from an already-resolved LLM.
// Prefer RLMConfig.CreateModule after setting cfg.LLM, or factory.CreateRLM from ProviderConfig.
func CreateRLMModule(llm core.LLM, cfg RLMConfig) (*dspyrlm.RLM, error) {
	if llm == nil {
		return nil, fmt.Errorf("CreateRLMModule: llm is required")
	}
	return dspyrlm.NewFromLLM(llm, rlmOptions(cfg)...), nil
}

// RLMComplete runs one RLM completion and returns the final answer text.
// When the module has TraceDir set, it also appends a full-fidelity inputs
// (and result) record to TraceDir/rlm_inputs.jsonl. That sidecar is the
// fixture source for env-gated RLM replay; the dspy-go session JSONL still
// truncates context in its metadata entry.
func RLMComplete(ctx context.Context, module *dspyrlm.RLM, contextPayload any, query string) (string, *dspyrlm.CompletionResult, error) {
	if module == nil {
		return "", nil, fmt.Errorf("RLMComplete: module is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	callID := uuid.NewString()
	traceDir := strings.TrimSpace(module.Config().TraceDir)
	if traceDir != "" {
		_ = appendRLMInputsDump(traceDir, rlmInputsDumpEntry{
			Type:      "inputs",
			Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
			CallID:    callID,
			Context:   contextPayload,
			Query:     query,
		})
	}
	result, err := module.Complete(ctx, contextPayload, query)
	if err != nil {
		if traceDir != "" {
			_ = appendRLMInputsDump(traceDir, rlmInputsDumpEntry{
				Type:      "error",
				Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
				CallID:    callID,
				Query:     query,
				Error:     err.Error(),
			})
		}
		return "", nil, err
	}
	if result == nil {
		return "", nil, fmt.Errorf("RLMComplete: empty result")
	}
	if traceDir != "" {
		_ = appendRLMInputsDump(traceDir, rlmInputsDumpEntry{
			Type:        "result",
			Timestamp:   time.Now().UTC().Format(time.RFC3339Nano),
			CallID:      callID,
			Query:       query,
			FinalAnswer: result.Response,
			Iterations:  result.Iterations,
		})
	}
	return result.Response, result, nil
}

type rlmInputsDumpEntry struct {
	Type        string `json:"type"` // inputs | result | error
	Timestamp   string `json:"timestamp"`
	CallID      string `json:"call_id"`
	Context     any    `json:"context,omitempty"`
	Query       string `json:"query,omitempty"`
	FinalAnswer string `json:"final_answer,omitempty"`
	Iterations  int    `json:"iterations,omitempty"`
	Error       string `json:"error,omitempty"`
}

func appendRLMInputsDump(traceDir string, entry rlmInputsDumpEntry) error {
	if err := os.MkdirAll(traceDir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(traceDir, RLMInputsDumpFile)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	return enc.Encode(entry)
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
