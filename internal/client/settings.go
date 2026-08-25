package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// ---------------------------------------------------------------------------
// App Settings API models
// ---------------------------------------------------------------------------

type SettingUpdateInput struct {
	Value json.RawMessage `json:"value"` // heterogeneous: true / 90 / "localhost"
}

type SettingResponse struct {
	Key         string          `json:"key"`
	Description string          `json:"description"`
	IsSecret    bool            `json:"is_secret"`
	Value       json.RawMessage `json:"value"`   // masked **** if is_secret
	Default     json.RawMessage `json:"default"` // masked **** if secret & non-empty
	Schema      json.RawMessage `json:"schema"`  // JSON Schema or null
	IsEncrypted bool            `json:"is_encrypted"`
	CreatedAt   string          `json:"created_at"`
	UpdatedAt   string          `json:"updated_at"`
}

type SettingListFilter struct {
	Search string
}

func (c *Client) GetSetting(ctx context.Context, key string) (*SettingResponse, error) {
	data, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/api/v1/settings/%s", url.PathEscape(key)), nil)
	if err != nil {
		return nil, err
	}
	var setting SettingResponse
	if err := json.Unmarshal(data, &setting); err != nil {
		return nil, fmt.Errorf("decoding setting: %w", err)
	}
	return &setting, nil
}

func (c *Client) UpdateSetting(ctx context.Context, key string, input SettingUpdateInput) (*SettingResponse, error) {
	data, err := c.doRequest(ctx, http.MethodPut, fmt.Sprintf("/api/v1/settings/%s", url.PathEscape(key)), input)
	if err != nil {
		return nil, err
	}
	var setting SettingResponse
	if err := json.Unmarshal(data, &setting); err != nil {
		return nil, fmt.Errorf("decoding setting: %w", err)
	}
	return &setting, nil
}

// ResetSetting resets the key to its registry default (POST …/{key}/reset).
func (c *Client) ResetSetting(ctx context.Context, key string) (*SettingResponse, error) {
	data, err := c.doRequest(ctx, http.MethodPost, fmt.Sprintf("/api/v1/settings/%s/reset", url.PathEscape(key)), nil)
	if err != nil {
		return nil, err
	}
	var setting SettingResponse
	if err := json.Unmarshal(data, &setting); err != nil {
		return nil, fmt.Errorf("decoding setting: %w", err)
	}
	return &setting, nil
}

func (c *Client) ListSettings(ctx context.Context, filter SettingListFilter) ([]SettingResponse, error) {
	params := url.Values{}
	params.Set("size", "10000")
	if filter.Search != "" {
		params.Set("search", filter.Search)
	}
	return paginateJSON[SettingResponse](ctx, c, "/api/v1/settings", params, "settings")
}
