package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// ---------------------------------------------------------------------------
// Log Destination API models
// ---------------------------------------------------------------------------

// config is an opaque per-type blob; secrets are masked in responses.
type LogDestinationCreateInput struct {
	Name        string          `json:"name"`
	Enabled     *bool           `json:"enabled,omitempty"` // default true server-side
	Type        string          `json:"type"`
	Streams     []string        `json:"streams"`                // minItems 1
	MinSeverity string          `json:"min_severity,omitempty"` // default "info"
	Config      json.RawMessage `json:"config"`
}

// LogDestinationUpdateInput is a FULL-REPLACE PUT — identical shape to create
// (no PATCH semantics).
type LogDestinationUpdateInput struct {
	Name        string          `json:"name"`
	Enabled     *bool           `json:"enabled,omitempty"`
	Type        string          `json:"type"`
	Streams     []string        `json:"streams"`
	MinSeverity string          `json:"min_severity,omitempty"`
	Config      json.RawMessage `json:"config"`
}

type LogDestinationResponse struct {
	ID          int64           `json:"id"`
	Name        string          `json:"name"`
	Enabled     bool            `json:"enabled"`
	Type        string          `json:"type"`
	Streams     []string        `json:"streams"`
	MinSeverity string          `json:"min_severity"`
	Config      json.RawMessage `json:"config"` // secrets masked as ****
	CreatedAt   string          `json:"created_at"`
	UpdatedAt   string          `json:"updated_at"`
}

type LogDestinationListFilter struct {
	Search string
}

// LogDestinationPreset is {key,label,vendor,type,default_config}.
type LogDestinationPreset struct {
	Key           string          `json:"key"`
	Label         string          `json:"label"`
	Vendor        string          `json:"vendor"`
	Type          string          `json:"type"`
	DefaultConfig json.RawMessage `json:"default_config"`
}

func (c *Client) CreateLogDestination(ctx context.Context, input LogDestinationCreateInput) (*LogDestinationResponse, error) {
	data, err := c.doRequest(ctx, http.MethodPost, "/api/v1/log-destinations", input)
	if err != nil {
		return nil, err
	}
	var dest LogDestinationResponse
	if err := json.Unmarshal(data, &dest); err != nil {
		return nil, fmt.Errorf("decoding log destination: %w", err)
	}
	return &dest, nil
}

func (c *Client) GetLogDestination(ctx context.Context, id int64) (*LogDestinationResponse, error) {
	data, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/api/v1/log-destinations/%d", id), nil)
	if err != nil {
		return nil, err
	}
	var dest LogDestinationResponse
	if err := json.Unmarshal(data, &dest); err != nil {
		return nil, fmt.Errorf("decoding log destination: %w", err)
	}
	return &dest, nil
}

func (c *Client) UpdateLogDestination(ctx context.Context, id int64, input LogDestinationUpdateInput) (*LogDestinationResponse, error) {
	data, err := c.doRequest(ctx, http.MethodPut, fmt.Sprintf("/api/v1/log-destinations/%d", id), input)
	if err != nil {
		return nil, err
	}
	var dest LogDestinationResponse
	if err := json.Unmarshal(data, &dest); err != nil {
		return nil, fmt.Errorf("decoding log destination: %w", err)
	}
	return &dest, nil
}

func (c *Client) DeleteLogDestination(ctx context.Context, id int64) error {
	_, err := c.doRequest(ctx, http.MethodDelete, fmt.Sprintf("/api/v1/log-destinations/%d", id), nil)
	return err
}

func (c *Client) ListLogDestinations(ctx context.Context, filter LogDestinationListFilter) ([]LogDestinationResponse, error) {
	params := url.Values{}
	params.Set("size", "10000")
	if filter.Search != "" {
		params.Set("search", filter.Search)
	}
	return paginateJSON[LogDestinationResponse](ctx, c, "/api/v1/log-destinations", params, "log destinations")
}

func (c *Client) ListLogDestinationPresets(ctx context.Context) ([]LogDestinationPreset, error) {
	data, err := c.doRequest(ctx, http.MethodGet, "/api/v1/log-destinations/presets", nil)
	if err != nil {
		return nil, err
	}
	var presets []LogDestinationPreset
	if err := json.Unmarshal(data, &presets); err != nil {
		return nil, fmt.Errorf("decoding log destination presets: %w", err)
	}
	return presets, nil
}
