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
	reqURL := c.BaseURL + path

	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return nil, nil, fmt.Errorf("marshaling request body: %w", err)
		}
	}

	for attempt := 1; ; attempt++ {
		var bodyReader io.Reader
		if bodyBytes != nil {
			bodyReader = bytes.NewReader(bodyBytes)
		}
		req, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
		if err != nil {
			return nil, nil, fmt.Errorf("creating request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
		req.Header.Set("Content-Type", "application/json")
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
			return nil, nil, fmt.Errorf("decoding response (status %d, body: %s): %w", resp.StatusCode, string(respBody), err)
		}
		if !envelope.Success {
			return nil, nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, envelope.Message)
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
