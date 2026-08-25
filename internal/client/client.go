package client

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
)

// Client is an HTTP client for the MisterShell API.
type Client struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
	// sleep waits for d or until ctx is cancelled, returning ctx.Err() if it
	// is. It is overridable in tests to make retry timing deterministic; nil
	// selects the default real-time implementation (see waitRetry).
	sleep        func(ctx context.Context, d time.Duration) error
	pollInterval time.Duration
}

// retryMaxAttempts bounds how many times doRequest re-issues a request that is
// answered with HTTP 429. The backend rate-limits at 100 requests/minute and
// returns a Retry-After header, which the client honors between attempts.
const retryMaxAttempts = 6

// waitRetry blocks for d (or until ctx is cancelled) before the next retry.
// Tests inject c.sleep to observe the delay without real waiting; production
// uses a real timer that is always stopped to avoid leaking it on cancellation.
func (c *Client) waitRetry(ctx context.Context, d time.Duration) error {
	if c.sleep != nil {
		return c.sleep(ctx, d)
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// NewClient creates a MisterShell API client.
func NewClient(baseURL, apiKey string, httpClient *http.Client) *Client {
	return &Client{
		BaseURL:      strings.TrimRight(baseURL, "/"),
		APIKey:       apiKey,
		HTTPClient:   httpClient,
		pollInterval: time.Second,
	}
}

// NotFoundError indicates the requested resource does not exist.
type NotFoundError struct {
	Path string
}

func (e *NotFoundError) Error() string { return "not found: " + e.Path }

// IsNotFound checks whether an error is a 404.
func IsNotFound(err error) bool {
	var nfe *NotFoundError
	return errors.As(err, &nfe)
}
