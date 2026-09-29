package openaibatch

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunChatBatchLifecycle(t *testing.T) {
	var batchGets atomic.Int32
	var sawAuth atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h := r.Header.Get("Authorization"); h != "Bearer secret" {
			if r.URL.Path != "/v1/batches/missing" {
				// only lifecycle paths require auth in this test
			}
		} else {
			sawAuth.Store(true)
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/files":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"file_1"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/batches":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"batch_1","status":"in_progress"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/batches/batch_1":
			n := batchGets.Add(1)
			if n < 2 {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"id":"batch_1","status":"in_progress"}`))
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"batch_1","status":"completed","output_file_id":"out_1"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/files/out_1/content":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"custom_id":"id1","response":{"status_code":200,"body":{"choices":[{"message":{"content":"answer"}}]}}}` + "\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := &Client{
		BaseURL:    srv.URL,
		APIKey:     "secret",
		HTTPClient: srv.Client(),
		PollEvery:  5 * time.Millisecond,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	got, err := client.RunChatBatch(ctx, []ChatLine{
		{CustomID: "id1", Model: "model-a", Messages: []map[string]string{{"role": "user", "content": "hi"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got["id1"].Content != "answer" {
		t.Fatalf("result=%+v", got["id1"])
	}
	if !sawAuth.Load() {
		t.Fatal("expected Authorization on requests")
	}
}

func TestRunChatBatchUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/files" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"bad key"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	client := &Client{BaseURL: srv.URL, APIKey: "nope", HTTPClient: srv.Client()}
	_, err := client.RunChatBatch(context.Background(), []ChatLine{
		{CustomID: "a", Model: "m", Messages: nil},
	})
	if err == nil || !IsUnauthorized(err) {
		t.Fatalf("expected unauthorized, got %v", err)
	}
}

func TestRunChatBatchNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/files" {
			http.NotFound(w, r)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	client := &Client{BaseURL: srv.URL, HTTPClient: srv.Client()}
	_, err := client.RunChatBatch(context.Background(), []ChatLine{
		{CustomID: "a", Model: "m"},
	})
	if err == nil || !IsNotFound(err) {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestRunChatBatchSplitTwoCycles(t *testing.T) {
	var creates atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/files":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"file_1"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/batches":
			id := creates.Add(1)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(fmt.Sprintf(`{"id":"batch_%d","status":"in_progress"}`, id)))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/batches/batch_1":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"batch_1","status":"completed","output_file_id":"out_1"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/batches/batch_2":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"batch_2","status":"completed","output_file_id":"out_2"}`))
		case r.URL.Path == "/v1/files/out_1/content":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"custom_id":"a","response":{"status_code":200,"body":{"choices":[{"message":{"content":"A"}}]}}}` + "\n"))
		case r.URL.Path == "/v1/files/out_2/content":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"custom_id":"b","response":{"status_code":200,"body":{"choices":[{"message":{"content":"B"}}]}}}` + "\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	// Force two upload/create cycles via a small cap.
	client := &Client{
		BaseURL:       srv.URL,
		HTTPClient:    srv.Client(),
		MaxJSONLBytes: 280,
		PollEvery:     time.Millisecond,
	}
	pad := strings.Repeat("p", 120)
	got, err := client.RunChatBatch(context.Background(), []ChatLine{
		{CustomID: "a", Model: "m", Messages: []map[string]string{{"role": "user", "content": pad}}},
		{CustomID: "b", Model: "m", Messages: []map[string]string{{"role": "user", "content": pad}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if creates.Load() < 2 {
		t.Fatalf("expected at least 2 batch creates, got %d", creates.Load())
	}
	if got["a"].Content != "A" || got["b"].Content != "B" {
		t.Fatalf("results=%v", got)
	}
}

func TestPollCancelPostsCancel(t *testing.T) {
	var cancelPosts atomic.Int32
	var batchGets atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/files":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"file_1"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/batches":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"batch_1","status":"in_progress"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/batches/batch_1/cancel":
			cancelPosts.Add(1)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"batch_1","status":"cancelling"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/batches/batch_1":
			batchGets.Add(1)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"batch_1","status":"in_progress"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := &Client{
		BaseURL:    srv.URL,
		HTTPClient: srv.Client(),
		PollEvery:  20 * time.Millisecond,
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(30 * time.Millisecond)
		cancel()
	}()
	_, err := client.RunChatBatch(ctx, []ChatLine{
		{CustomID: "a", Model: "m", Messages: []map[string]string{{"role": "user", "content": "x"}}},
	})
	if err == nil {
		t.Fatal("expected cancel error")
	}
	if cancelPosts.Load() != 1 {
		t.Fatalf("expected one cancel POST, got %d", cancelPosts.Load())
	}
}

func TestCancelBatchNotImplemented(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/cancel") {
			w.WriteHeader(http.StatusNotImplemented)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	client := &Client{BaseURL: srv.URL, HTTPClient: srv.Client()}
	if err := client.CancelBatch(context.Background(), "batch_1"); err != nil {
		t.Fatal(err)
	}
}

func TestHTTPErrorHelpers(t *testing.T) {
	err := httpErr("op", 503, "slow down")
	if !IsThrottle(err) {
		t.Fatal("expected throttle")
	}
	err = httpErr("op", 429, "")
	if !IsThrottle(err) {
		t.Fatal("expected throttle 429")
	}
}
