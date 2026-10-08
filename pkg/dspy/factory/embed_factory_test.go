package factory

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	stropdspy "github.com/behaviorengineering/strop/pkg/dspy"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
)

func embeddingServer(t *testing.T, status int, body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/embeddings") {
			http.NotFound(w, r)
			return
		}
		var req struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Model != "embed-model" {
			t.Errorf("model %q", req.Model)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

func TestCreateEmbedder_postsEmbeddings(t *testing.T) {
	srv := embeddingServer(t, http.StatusOK, `{"data":[{"embedding":[1,0],"index":0},{"embedding":[0,1],"index":1}],"model":"embed-model","usage":{"total_tokens":2}}`)
	t.Cleanup(srv.Close)

	fac := NewLLMFactory(nil, 30*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	emb, err := fac.CreateEmbedder(ctx, stropdspy.ProviderConfig{
		APISchema:  "openai",
		BaseURL:    srv.URL,
		EmbedModel: "embed-model",
	})
	if err != nil {
		t.Fatal(err)
	}
	vecs, err := emb.Embed(ctx, []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	if len(vecs) != 2 || vecs[0][0] != 1 || vecs[1][1] != 1 {
		t.Fatalf("%v", vecs)
	}
}

func TestCreateEmbedder_emptyEmbedModel(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)
	fac := NewLLMFactory(nil, 0)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := fac.CreateEmbedder(ctx, stropdspy.ProviderConfig{
		APISchema: "openai",
		BaseURL:   srv.URL,
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCreateEmbedder_noDeadline(t *testing.T) {
	fac := NewLLMFactory(nil, 0)
	_, err := fac.CreateEmbedder(context.Background(), stropdspy.ProviderConfig{
		APISchema:  "openai",
		BaseURL:    "http://127.0.0.1:1",
		EmbedModel: "m",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCreateEmbedder_emptyTexts(t *testing.T) {
	srv := embeddingServer(t, http.StatusOK, `{"data":[],"model":"embed-model","usage":{"total_tokens":0}}`)
	t.Cleanup(srv.Close)
	fac := NewLLMFactory(nil, 30*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	emb, err := fac.CreateEmbedder(ctx, stropdspy.ProviderConfig{
		APISchema:  "openai",
		BaseURL:    srv.URL,
		EmbedModel: "embed-model",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = emb.Embed(ctx, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCreateEmbedder_batchResultError(t *testing.T) {
	srv := embeddingServer(t, http.StatusInternalServerError, `{"error":"fail"}`)
	t.Cleanup(srv.Close)
	fac := NewLLMFactory(nil, 30*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	emb, err := fac.CreateEmbedder(ctx, stropdspy.ProviderConfig{
		APISchema:  "openai",
		BaseURL:    srv.URL,
		EmbedModel: "embed-model",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = emb.Embed(ctx, []string{"x"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCreateEmbedder_skipsWrapLLM(t *testing.T) {
	srv := embeddingServer(t, http.StatusOK, `{"data":[{"embedding":[1],"index":0}],"model":"embed-model","usage":{"total_tokens":1}}`)
	t.Cleanup(srv.Close)
	fac := NewLLMFactory(nil, 30*time.Second)
	fac.SetWrapLLM(func(llm core.LLM) core.LLM {
		return panicOnEmbedLLM{LLM: llm}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	emb, err := fac.CreateEmbedder(ctx, stropdspy.ProviderConfig{
		APISchema:  "openai",
		BaseURL:    srv.URL,
		EmbedModel: "embed-model",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := emb.Embed(ctx, []string{"a"}); err != nil {
		t.Fatal(err)
	}
}

type panicOnEmbedLLM struct {
	core.LLM
}

func (p panicOnEmbedLLM) CreateEmbeddings(ctx context.Context, inputs []string, options ...core.EmbeddingOption) (*core.BatchEmbeddingResult, error) {
	panic("wrapLLM should not apply to CreateEmbedder")
}

func TestCreateEmbedder_rejectsGoogleSchema(t *testing.T) {
	fac := NewLLMFactory(nil, 0)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := fac.CreateEmbedder(ctx, stropdspy.ProviderConfig{
		APISchema:  "google",
		BaseURL:    "https://example.com",
		EmbedModel: "m",
		Model:      "m",
	})
	if err == nil || !strings.Contains(err.Error(), "openai") {
		t.Fatalf("got %v", err)
	}
}
