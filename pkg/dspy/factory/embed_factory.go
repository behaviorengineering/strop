package factory

import (
	"context"
	"fmt"
	"strings"

	stropdspy "github.com/behaviorengineering/strop/pkg/dspy"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
)

// Embedder returns embedding vectors as float64 slices in input order.
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float64, error)
}

type fixedModelEmbedder struct {
	inner core.BatchEmbedder
	model string
}

// CreateEmbedder builds an Embedder that calls dspy-go CreateEmbeddings with a fixed model id.
func (f *LLMFactory) CreateEmbedder(ctx context.Context, cfg stropdspy.ProviderConfig) (Embedder, error) {
	const op = "factory.LLMFactory.CreateEmbedder"
	if f == nil {
		return nil, fmt.Errorf("%s: nil factory", op)
	}
	if ctx == nil {
		return nil, fmt.Errorf("%s: nil context", op)
	}
	if _, ok := ctx.Deadline(); !ok {
		return nil, fmt.Errorf("%s: context must have a deadline", op)
	}
	schema := strings.TrimSpace(cfg.APISchema)
	base := strings.TrimSpace(cfg.BaseURL)
	embedModel := strings.TrimSpace(cfg.EmbedModel)
	if schema != apiSchemaOpenAI {
		return nil, fmt.Errorf("%s: embed requires api_schema openai", op)
	}
	if base == "" {
		return nil, fmt.Errorf("%s: base_url is required", op)
	}
	if embedModel == "" {
		return nil, fmt.Errorf("%s: embed_model is required", op)
	}
	inner := cfg
	if strings.TrimSpace(inner.Model) == "" {
		inner.Model = embedModel
	}
	savedWrap := f.wrapLLM
	f.wrapLLM = nil
	llm, err := f.CreateLLM(ctx, inner)
	f.wrapLLM = savedWrap
	if err != nil {
		return nil, fmt.Errorf("%s: create llm: %w", op, err)
	}
	if !hasCapabilityEmbedding(llm) {
		return nil, fmt.Errorf("%s: llm does not support embeddings", op)
	}
	batch, ok := llm.(core.BatchEmbedder)
	if !ok {
		return nil, fmt.Errorf("%s: llm is not a batch embedder", op)
	}
	return &fixedModelEmbedder{inner: batch, model: embedModel}, nil
}

func hasCapabilityEmbedding(llm core.LLM) bool {
	for _, c := range llm.Capabilities() {
		if c == core.CapabilityEmbedding {
			return true
		}
	}
	return false
}

func (e *fixedModelEmbedder) Embed(ctx context.Context, texts []string) ([][]float64, error) {
	const op = "factory.fixedModelEmbedder.Embed"
	if e == nil || e.inner == nil {
		return nil, fmt.Errorf("%s: nil embedder", op)
	}
	if ctx == nil {
		return nil, fmt.Errorf("%s: nil context", op)
	}
	if _, ok := ctx.Deadline(); !ok {
		return nil, fmt.Errorf("%s: context must have a deadline", op)
	}
	if len(texts) == 0 {
		return nil, fmt.Errorf("%s: texts required", op)
	}
	batch, err := e.inner.CreateEmbeddings(ctx, texts, core.WithModel(e.model))
	if err != nil {
		return nil, fmt.Errorf("%s: create embeddings: %w", op, err)
	}
	if batch == nil {
		return nil, fmt.Errorf("%s: nil batch result", op)
	}
	if batch.Error != nil {
		return nil, fmt.Errorf("%s: %w", op, batch.Error)
	}
	if len(batch.Embeddings) != len(texts) {
		return nil, fmt.Errorf("%s: expected %d embeddings got %d", op, len(texts), len(batch.Embeddings))
	}
	out := make([][]float64, len(texts))
	for i, emb := range batch.Embeddings {
		vec := make([]float64, len(emb.Vector))
		for j, v := range emb.Vector {
			vec[j] = float64(v)
		}
		out[i] = vec
	}
	return out, nil
}
