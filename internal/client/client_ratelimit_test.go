package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// newRecordingClient returns a client whose retry wait is captured (not slept)
// so retry timing is asserted deterministically without real delays.
func newRecordingClient(baseURL string, hc *http.Client, waits *[]time.Duration) *Client {
	return &Client{
		BaseURL:    baseURL,
		APIKey:     "test",
		HTTPClient: hc,
		sleep: func(_ context.Context, d time.Duration) error {
			*waits = append(*waits, d)
			return nil
		},
	}
}

// TestDoRequestHonorsRetryAfter proves the client waits exactly the Retry-After
// value the server returns (100 req/min → the backend sends a Retry-After on 429).
func TestDoRequestHonorsRetryAfter(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"success":false,"message":"Rate limit exceeded: 100/minute"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true,"message":"ok","data":{"id":1}}`))
	}))
	defer srv.Close()

	var waits []time.Duration
	c := newRecordingClient(srv.URL, srv.Client(), &waits)

	data, err := c.doRequest(context.Background(), http.MethodGet, "/x", nil)
	if err != nil {
		t.Fatalf("expected success after retry, got %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("expected 2 calls (429 then 200), got %d", got)
	}
	if len(waits) != 1 || waits[0] != 2*time.Second {
		t.Fatalf("expected to honor Retry-After 2s, recorded waits %v", waits)
	}
	if string(data) != `{"id":1}` {
		t.Fatalf("unexpected data: %s", data)
	}
}

// TestDoRequestBackoffFallbackAndExhaustion covers the no-Retry-After path: the
// client falls back to capped exponential backoff and gives up after exactly
// retryMaxAttempts, surfacing the final 429 as an error.
func TestDoRequestBackoffFallbackAndExhaustion(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		// No Retry-After header → exercises the backoff fallback.
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"success":false,"message":"Rate limit exceeded"}`))
	}))
	defer srv.Close()

	var waits []time.Duration
	c := newRecordingClient(srv.URL, srv.Client(), &waits)

	_, err := c.doRequest(context.Background(), http.MethodGet, "/x", nil)
	if err == nil {
		t.Fatal("expected error after retries exhausted")
	}
	if got := atomic.LoadInt32(&calls); got != retryMaxAttempts {
		t.Fatalf("expected exactly %d attempts, got %d", retryMaxAttempts, got)
	}
	// retryMaxAttempts requests → retryMaxAttempts-1 waits, backing off 1,2,4,8,16s.
	want := []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second}
	if len(waits) != len(want) {
		t.Fatalf("expected %d backoff waits, got %v", len(want), waits)
	}
	for i, w := range want {
		if waits[i] != w {
			t.Fatalf("backoff wait %d = %v, want %v (full: %v)", i, waits[i], w, waits)
		}
	}
}

// TestDoRequestRetryContextCancelled ensures a cancellation during the retry
// wait aborts immediately and propagates the context error.
func TestDoRequestRetryContextCancelled(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"success":false,"message":"Rate limit exceeded"}`))
	}))
	defer srv.Close()

	c := &Client{
		BaseURL:    srv.URL,
		APIKey:     "test",
		HTTPClient: srv.Client(),
		sleep: func(_ context.Context, _ time.Duration) error {
			return context.Canceled
		},
	}

	_, err := c.doRequest(context.Background(), http.MethodGet, "/x", nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected to stop after 1 attempt on cancellation, got %d", got)
	}
}

// TestWaitRetryRealTimer exercises the default (nil sleep) timer path directly:
// it returns after the duration, and returns ctx.Err() when the context is
// already cancelled (no timer leak — the deferred Stop covers that).
func TestWaitRetryRealTimer(t *testing.T) {
	c := &Client{}
	if err := c.waitRetry(context.Background(), 5*time.Millisecond); err != nil {
		t.Fatalf("expected nil after wait, got %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.waitRetry(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled on cancelled ctx, got %v", err)
	}
}

// TestRetryAfterDelay pins the Retry-After parsing and backoff fallback math.
func TestRetryAfterDelay(t *testing.T) {
	cases := []struct {
		name    string
		header  string
		attempt int
		want    time.Duration
	}{
		{"honors header seconds", "5", 1, 5 * time.Second},
		{"honors zero", "0", 3, 0},
		{"trims whitespace", "  3 ", 1, 3 * time.Second},
		{"negative header falls back to backoff", "-1", 2, 2 * time.Second},
		{"malformed header falls back to backoff", "soon", 1, 1 * time.Second},
		{"empty header backoff attempt 1", "", 1, 1 * time.Second},
		{"empty header backoff attempt 4", "", 4, 8 * time.Second},
		{"backoff caps at 30s", "", 10, 30 * time.Second},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := retryAfterDelay(tc.header, tc.attempt); got != tc.want {
				t.Fatalf("retryAfterDelay(%q, %d) = %v, want %v", tc.header, tc.attempt, got, tc.want)
			}
		})
	}
}
