package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestDoRequestRetriesOn429(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"success":false,"message":"Rate limit exceeded: 100/minute"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true,"message":"ok","data":{"id":1}}`))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, APIKey: "test", HTTPClient: srv.Client()}
	start := time.Now()
	data, err := c.doRequest(context.Background(), http.MethodGet, "/x", nil)
	if err != nil {
		t.Fatalf("expected success after retry, got %v", err)
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("expected 2 calls (429 then 200), got %d", calls)
	}
	if time.Since(start) < time.Second {
		t.Fatalf("expected to honor Retry-After: 1s, waited %v", time.Since(start))
	}
	if string(data) != `{"id":1}` {
		t.Fatalf("unexpected data: %s", data)
	}
}

func TestDoRequestRetryBudgetExhausted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"success":false,"message":"Rate limit exceeded"}`))
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, APIKey: "test", HTTPClient: srv.Client()}
	_, err := c.doRequest(context.Background(), http.MethodGet, "/x", nil)
	if err == nil {
		t.Fatal("expected error after retries exhausted")
	}
}
