package jev

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestEval_httptest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/systemone" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"answers": map[string]any{
				"probe": map[string]any{"type": "noul", "noul": 0.5},
			},
		})
	}))
	t.Cleanup(srv.Close)
	c := NewClient(srv.URL, srv.Client(), "")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ans, err := c.Eval(ctx, Request{
		Model: "jev",
		State: "state",
		Questions: map[string]Question{
			"probe": {Type: "noul", Instructions: "x"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if ans["probe"].Noul != 0.5 {
		t.Fatalf("noul: %v", ans["probe"].Noul)
	}
}
