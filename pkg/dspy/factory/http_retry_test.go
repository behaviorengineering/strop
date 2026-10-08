package factory

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	stropdspy "github.com/behaviorengineering/strop/pkg/dspy"

	"github.com/XiaoConstantine/dspy-go/pkg/interceptors"
)

func TestInstrumentHTTP_retries429InPlace(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := hits.Add(1)
		if n == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":[{"embedding":[1,0],"index":0}],"model":"m","usage":{"total_tokens":1}}`))
	}))
	t.Cleanup(srv.Close)

	fac := NewLLMFactory(nil, 30*time.Second)
	fac.SetRetryConfig(&interceptors.RetryConfig{MaxAttempts: 2, Delay: 10 * time.Millisecond, Backoff: 1})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	emb, err := fac.CreateEmbedder(ctx, stropdspy.ProviderConfig{
		APISchema:  "openai",
		BaseURL:    srv.URL,
		EmbedModel: "m",
	})
	if err != nil {
		t.Fatal(err)
	}
	vecs, err := emb.Embed(ctx, []string{"a"})
	if err != nil {
		t.Fatal(err)
	}
	if hits.Load() < 2 {
		t.Fatalf("hits %d", hits.Load())
	}
	if len(vecs) != 1 || vecs[0][0] != 1 {
		t.Fatalf("%v", vecs)
	}
}

func TestInstrumentHTTP_doesNotRetry400(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	t.Cleanup(srv.Close)

	fac := NewLLMFactory(nil, 30*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	emb, err := fac.CreateEmbedder(ctx, stropdspy.ProviderConfig{
		APISchema:  "openai",
		BaseURL:    srv.URL,
		EmbedModel: "m",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = emb.Embed(ctx, []string{"a"})
	if err == nil {
		t.Fatal("expected error")
	}
	if hits.Load() != 1 {
		t.Fatalf("hits %d", hits.Load())
	}
}
