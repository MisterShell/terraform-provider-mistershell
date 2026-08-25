package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// ---------------------------------------------------------------------------
// Location API models
// ---------------------------------------------------------------------------

type LocationCreateInput struct {
	Name        string          `json:"name"`
	Kind        string          `json:"kind,omitempty"`
	Description *string         `json:"description,omitempty"`
	ParentID    *int64          `json:"parent_id,omitempty"`
	Latitude    *float64        `json:"latitude,omitempty"`
	Longitude   *float64        `json:"longitude,omitempty"`
	ExtraData   json.RawMessage `json:"extra_data,omitempty"`
}

type LocationUpdateInput struct {
	Name        *string         `json:"name,omitempty"`
	Kind        *string         `json:"kind,omitempty"`
	Description *string         `json:"description"`
	ParentID    *int64          `json:"parent_id"`
	Latitude    *float64        `json:"latitude"`
	Longitude   *float64        `json:"longitude"`
	ExtraData   json.RawMessage `json:"extra_data"`
}

type LocationResponse struct {
	ID            int64           `json:"id"`
	Name          string          `json:"name"`
	Kind          string          `json:"kind"`
	Description   *string         `json:"description"`
	ParentID      *int64          `json:"parent_id"`
	Latitude      *float64        `json:"latitude"`
	Longitude     *float64        `json:"longitude"`
	ExtraData     json.RawMessage `json:"extra_data"`
	ResourceCount *int64          `json:"resource_count"`
	Path          *string         `json:"path"`
	CreatedAt     string          `json:"created_at"`
	UpdatedAt     string          `json:"updated_at"`
}

func (c *Client) CreateLocation(ctx context.Context, input LocationCreateInput) (*LocationResponse, error) {
	data, err := c.doRequest(ctx, http.MethodPost, "/api/v1/locations", input)
	if err != nil {
		return nil, err
	}
	var loc LocationResponse
	if err := json.Unmarshal(data, &loc); err != nil {
		return nil, fmt.Errorf("decoding location: %w", err)
	}
	return &loc, nil
}

func (c *Client) GetLocation(ctx context.Context, id int64) (*LocationResponse, error) {
	data, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/api/v1/locations/%d", id), nil)
	if err != nil {
		return nil, err
	}
	var loc LocationResponse
	if err := json.Unmarshal(data, &loc); err != nil {
		return nil, fmt.Errorf("decoding location: %w", err)
	}
	return &loc, nil
}

func (c *Client) UpdateLocation(ctx context.Context, id int64, input LocationUpdateInput) (*LocationResponse, error) {
	data, err := c.doRequest(ctx, http.MethodPatch, fmt.Sprintf("/api/v1/locations/%d", id), input)
	if err != nil {
		return nil, err
	}
	var loc LocationResponse
	if err := json.Unmarshal(data, &loc); err != nil {
		return nil, fmt.Errorf("decoding location: %w", err)
	}
	return &loc, nil
}

func (c *Client) DeleteLocation(ctx context.Context, id int64) error {
	_, err := c.doRequest(ctx, http.MethodDelete, fmt.Sprintf("/api/v1/locations/%d", id), nil)
	return err
}

type LocationListFilter struct {
	Search   string
	ParentID *int64
	Kind     string
}

func (c *Client) ListLocations(ctx context.Context, filter LocationListFilter) ([]LocationResponse, error) {
	params := url.Values{}
	params.Set("size", "10000")
	if filter.Search != "" {
		params.Set("search", filter.Search)
	}
	if filter.ParentID != nil {
		params.Set("parent_id", fmt.Sprintf("%d", *filter.ParentID))
	}
	return paginateJSON[LocationResponse](ctx, c, "/api/v1/locations", params, "locations")
}
