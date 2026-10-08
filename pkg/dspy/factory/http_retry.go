package factory

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/XiaoConstantine/dspy-go/pkg/interceptors"
	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/retrypolicy"
)

type retryTransport struct {
	base     http.RoundTripper
	attempts int
	delay    time.Duration
}

func wrapHTTPClientRetry(client *http.Client, cfg *interceptors.RetryConfig) {
	if client == nil {
		return
	}
	attempts := 2
	delay := 2 * time.Second
	if cfg != nil {
		if cfg.MaxAttempts > 0 {
			attempts = cfg.MaxAttempts
		}
		if cfg.Delay > 0 {
			delay = cfg.Delay
		}
	}
	base := client.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	client.Transport = &retryTransport{base: base, attempts: attempts, delay: delay}
}

func (t *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t == nil || t.base == nil {
		return nil, errors.New("retry transport: nil base")
	}
	body, err := readAndCloneBody(req)
	if err != nil {
		return nil, err
	}
	policy := retrypolicy.NewBuilder[*http.Response]().
		HandleIf(func(_ *http.Response, err error) bool {
			if err != nil {
				return true
			}
			return false
		}).
		WithMaxAttempts(t.attempts).
		WithDelay(t.delay).
		Build()
	return failsafe.With(policy).
		WithContext(req.Context()).
		Get(func() (*http.Response, error) {
			cloned := cloneRequest(req, body)
			resp, err := t.base.RoundTrip(cloned)
			if err != nil {
				return nil, err
			}
			if resp.StatusCode == 429 || resp.StatusCode >= 500 {
				_ = resp.Body.Close()
				return nil, &retryableHTTPStatus{code: resp.StatusCode}
			}
			return resp, nil
		})
}

type retryableHTTPStatus struct {
	code int
}

func (e *retryableHTTPStatus) Error() string {
	return "retryable http status: " + http.StatusText(e.code)
}

func readAndCloneBody(req *http.Request) ([]byte, error) {
	if req == nil || req.Body == nil {
		return nil, nil
	}
	data, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	req.Body = io.NopCloser(bytes.NewReader(data))
	return data, nil
}

func cloneRequest(req *http.Request, body []byte) *http.Request {
	cloned := req.Clone(req.Context())
	if len(body) > 0 {
		cloned.Body = io.NopCloser(bytes.NewReader(body))
		cloned.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(body)), nil
		}
	}
	return cloned
}

var retryWrapMu sync.Mutex
var wrappedClients = make(map[*http.Client]bool)

func instrumentClientRetryOnce(client *http.Client, cfg *interceptors.RetryConfig) {
	if client == nil {
		return
	}
	retryWrapMu.Lock()
	defer retryWrapMu.Unlock()
	if wrappedClients[client] {
		return
	}
	wrapHTTPClientRetry(client, cfg)
	wrappedClients[client] = true
}
