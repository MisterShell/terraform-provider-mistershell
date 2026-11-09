package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// responseEnvelope is the standard MisterShell API response wrapper.
type responseEnvelope struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
	Meta    *PaginationMeta `json:"meta"`
}

// apiEnvelope keeps existing package-level test helpers source-compatible.
type apiEnvelope = responseEnvelope

// NewHTTPClient returns an HTTP client based on Go's default transport.
func NewHTTPClient(insecure bool) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if insecure {
		tlsConfig := transport.TLSClientConfig
		if tlsConfig == nil {
			tlsConfig = &tls.Config{}
		} else {
			tlsConfig = tlsConfig.Clone()
		}
		tlsConfig.InsecureSkipVerify = true //nolint:gosec
		transport.TLSClientConfig = tlsConfig
	}
	return &http.Client{Transport: transport, Timeout: 60 * time.Second}
}

// doRequest executes an HTTP request and returns the unwrapped data payload.
func (c *Client) doRequest(ctx context.Context, method, path string, body interface{}) (json.RawMessage, error) {
	data, _, err := c.doRequestWithMeta(ctx, method, path, body)
	return data, err
}

// doRequestWithMeta executes an HTTP request and returns data plus pagination metadata.
func (c *Client) doRequestWithMeta(ctx context.Context, method, path string, body interface{}) (json.RawMessage, *PaginationMeta, error) {
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return nil, nil, fmt.Errorf("marshaling request body: %w", err)
		}
	}

	return c.doRequestBody(ctx, method, path, "application/json", bytes.NewReader(bodyBytes))
}

// ConfigurationUpload sends binary catalog content through the shared authenticated
// transport. A seekable body lets rate-limit retries replay the complete upload.
func (c *Client) ConfigurationUpload(ctx context.Context, path string, body io.ReadSeeker) (json.RawMessage, error) {
	data, _, err := c.doRequestBody(ctx, http.MethodPost, path, "application/octet-stream", body)
	return data, err
}

func (c *Client) doRequestBody(ctx context.Context, method, path, contentType string, body io.ReadSeeker) (json.RawMessage, *PaginationMeta, error) {
	for attempt := 1; ; attempt++ {
		if body != nil {
			if _, err := body.Seek(0, io.SeekStart); err != nil {
				return nil, nil, fmt.Errorf("rewinding request body: %w", err)
			}
		}
		req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, body)
		if err != nil {
			return nil, nil, fmt.Errorf("creating request: %w", err)
		}
		// The caller owns seekable uploads; HTTP must not close a file needed
		// by a rate-limit retry (or by the caller after this request).
		if body != nil && req.Body != http.NoBody {
			req.Body = io.NopCloser(body)
		}
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
		req.Header.Set("Content-Type", contentType)
		req.Header.Set("Accept", "application/json")

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			return nil, nil, fmt.Errorf("executing request: %w", err)
		}
		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, nil, fmt.Errorf("reading response: %w", err)
		}

		if resp.StatusCode == http.StatusTooManyRequests && attempt < retryMaxAttempts {
			if err := c.waitRetry(ctx, retryAfterDelay(resp.Header.Get("Retry-After"), attempt)); err != nil {
				return nil, nil, err
			}
			continue
		}
		if resp.StatusCode == http.StatusNotFound {
			return nil, nil, &NotFoundError{Path: path}
		}
		if resp.StatusCode == http.StatusNoContent {
			return nil, nil, nil
		}

		var envelope responseEnvelope
		if err := json.Unmarshal(respBody, &envelope); err != nil {
			return nil, nil, fmt.Errorf("decoding response (status %d): %w", resp.StatusCode, err)
		}
		if !envelope.Success {
			return nil, nil, &APIError{StatusCode: resp.StatusCode, Message: envelope.Message}
		}
		return envelope.Data, envelope.Meta, nil
	}
}

func retryAfterDelay(header string, attempt int) time.Duration {
	if header != "" {
		if secs, err := strconv.Atoi(strings.TrimSpace(header)); err == nil && secs >= 0 {
			return time.Duration(secs) * time.Second
		}
	}
	backoff := time.Duration(1<<uint(attempt-1)) * time.Second
	if backoff > 30*time.Second {
		backoff = 30 * time.Second
	}
	return backoff
}
