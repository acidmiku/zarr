package httpclient

import (
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"time"
)

// retryTransport wraps an http.RoundTripper with bounded retries on transient
// failures: transport errors (EOF, connection reset, dial timeouts) and 5xx /
// 408 / 429 responses. Successful responses and 4xx errors pass through immediately.
//
// Requests with a body are only retried if req.GetBody is set (the stdlib sets
// it automatically for bytes/strings.Reader bodies — i.e. virtually all our
// POSTs). Multipart uploads etc. fall through with no retry.
type retryTransport struct {
	base     http.RoundTripper
	attempts int
	backoffs []time.Duration
}

func wrapRetry(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &retryTransport{
		base:     base,
		attempts: 3,
		backoffs: []time.Duration{
			500 * time.Millisecond,
			1500 * time.Millisecond,
			3000 * time.Millisecond,
		},
	}
}

func (t *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Retrying a POST can enqueue the same download or charge an AI request twice.
	canReplay := (req.Method == http.MethodGet || req.Method == http.MethodHead || req.Method == http.MethodOptions) && (req.Body == nil || req.GetBody != nil)
	// SABnzbd exposes mutations as GET; only retry its read operations.
	if mode := req.URL.Query().Get("mode"); mode != "" {
		switch mode {
		case "version", "get_config", "queue", "history":
			if req.URL.Query().Get("name") != "" {
				canReplay = false
			}
		default:
			canReplay = false
		}
	}
	attempts := t.attempts
	if !canReplay {
		attempts = 1
	}

	var lastErr error

	for i := 0; i < attempts; i++ {
		if i > 0 {
			base := t.backoffs[i-1]
			jitter := time.Duration(rand.Int63n(int64(base) / 3))
			if rand.Intn(2) == 0 {
				jitter = -jitter
			}
			select {
			case <-req.Context().Done():
				return nil, req.Context().Err()
			case <-time.After(base + jitter):
			}

			if req.GetBody != nil {
				body, err := req.GetBody()
				if err != nil {
					return nil, err
				}
				req.Body = body
			}
		}

		resp, err := t.base.RoundTrip(req)
		if err != nil {
			lastErr = err
			// If the user's context is done, stop — they canceled.
			if req.Context().Err() != nil {
				return nil, err
			}
			if i < attempts-1 {
				slog.Debug("http retry: transport error",
					"host", req.URL.Hostname(), "attempt", i+1)
				continue
			}
			return nil, err
		}

		// Got a response. Decide if we should retry the status.
		if i < attempts-1 && retryableStatus(resp.StatusCode) {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			slog.Debug("http retry: status",
				"host", req.URL.Hostname(), "status", resp.StatusCode, "attempt", i+1)
			continue
		}

		return resp, nil
	}

	return nil, lastErr
}

func retryableStatus(s int) bool {
	switch s {
	case http.StatusRequestTimeout, // 408
		http.StatusTooManyRequests,    // 429
		http.StatusBadGateway,         // 502
		http.StatusServiceUnavailable, // 503
		http.StatusGatewayTimeout:     // 504
		return true
	}
	return false
}
