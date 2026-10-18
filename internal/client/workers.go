package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// ---------------------------------------------------------------------------
// Worker API models
// ---------------------------------------------------------------------------

// WorkerCreateInput: is_default is intentionally absent — the backend forbids
// creating a default worker. config/config_schema are opaque blobs.
type WorkerCreateInput struct {
	Name         string          `json:"name"`
	Description  *string         `json:"description,omitempty"`
	LocationID   int64           `json:"location_id"`
	Config       json.RawMessage `json:"config,omitempty"`
	ConfigSchema json.RawMessage `json:"config_schema,omitempty"`
	IsEnabled    bool            `json:"is_enabled"`
}

// WorkerUpdateInput sends only changed fields; RawMessage distinguishes omission from null.
type WorkerUpdateInput struct {
	Name         *string         `json:"name,omitempty"`
	Description  json.RawMessage `json:"description,omitempty"`
	LocationID   *int64          `json:"location_id,omitempty"`
	Config       json.RawMessage `json:"config,omitempty"`
	ConfigSchema json.RawMessage `json:"config_schema,omitempty"`
	IsEnabled    *bool           `json:"is_enabled,omitempty"`
}

// WorkerCreatedResponse is the one-time creation shape. Runtime registration
// fields are deliberately absent because the worker has not connected yet.
type WorkerCreatedResponse struct {
	ID            int64           `json:"id"`
	Name          string          `json:"name"`
	Description   *string         `json:"description"`
	Status        string          `json:"status"`
	IsDefault     bool            `json:"is_default"`
	IsEnabled     bool            `json:"is_enabled"`
	LocationID    int64           `json:"location_id"`
	Config        json.RawMessage `json:"config"`
	ConfigVersion *string         `json:"config_version"`
	ConfigSchema  json.RawMessage `json:"config_schema"`
	Token         string          `json:"token"`
	CreatedAt     string          `json:"created_at"`
	UpdatedAt     string          `json:"updated_at"`
}

// WorkerResponse is the detail/created shape. Token is returned ONLY by POST
// create and POST regenerate-token; it is never readable again.
type WorkerResponse struct {
	ID               int64           `json:"id"`
	Name             string          `json:"name"`
	Description      *string         `json:"description"`
	Status           string          `json:"status"`
	IsDefault        bool            `json:"is_default"`
	IsEnabled        bool            `json:"is_enabled"`
	ActiveTaskCount  int64           `json:"active_task_count"`
	TotalTaskCount   int64           `json:"total_task_count"`
	LastHeartbeatAt  *string         `json:"last_heartbeat_at"`
	Version          *string         `json:"version"`
	RDKContractMajor *int64          `json:"rdk_contract_major"`
	RDKCatalogID     *string         `json:"rdk_catalog_identity"`
	RDKCompatible    bool            `json:"rdk_compatible"`
	ConnectedAt      *string         `json:"connected_at"`
	CreatedAt        string          `json:"created_at"`
	UpdatedAt        string          `json:"updated_at"`
	LocationID       int64           `json:"location_id"`
	PresenceIP       *string         `json:"presence_ip"`
	PresenceIPSource *string         `json:"presence_ip_source"`
	Config           json.RawMessage `json:"config"`
	ConfigVersion    *string         `json:"config_version"`
	ConfigSchema     json.RawMessage `json:"config_schema"`
	Token            string          `json:"token,omitempty"`
}

type WorkerListFilter struct {
	Search string
}

func (c *Client) CreateWorker(ctx context.Context, input WorkerCreateInput) (*WorkerCreatedResponse, error) {
	data, err := c.doRequest(ctx, http.MethodPost, "/api/v1/workers", input)
	if err != nil {
		return nil, err
	}
	var worker WorkerCreatedResponse
	if err := json.Unmarshal(data, &worker); err != nil {
		return nil, fmt.Errorf("decoding worker: %w", err)
	}
	return &worker, nil
}

func (c *Client) GetWorker(ctx context.Context, id int64) (*WorkerResponse, error) {
	data, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/api/v1/workers/%d", id), nil)
	if err != nil {
		return nil, err
	}
	var worker WorkerResponse
	if err := json.Unmarshal(data, &worker); err != nil {
		return nil, fmt.Errorf("decoding worker: %w", err)
	}
	return &worker, nil
}

// UpdateWorker patches a worker. Default workers accept config-only PATCH requests.
func (c *Client) UpdateWorker(ctx context.Context, id int64, input WorkerUpdateInput) (*WorkerResponse, error) {
	data, err := c.doRequest(ctx, http.MethodPatch, fmt.Sprintf("/api/v1/workers/%d", id), input)
	if err != nil {
		return nil, err
	}
	var worker WorkerResponse
	if err := json.Unmarshal(data, &worker); err != nil {
		return nil, fmt.Errorf("decoding worker: %w", err)
	}
	return &worker, nil
}

// DeleteWorker removes a worker. The default worker (is_default=true) returns
// 403 on DELETE.
func (c *Client) DeleteWorker(ctx context.Context, id int64) error {
	_, err := c.doRequest(ctx, http.MethodDelete, fmt.Sprintf("/api/v1/workers/%d", id), nil)
	return err
}

func (c *Client) ListWorkers(ctx context.Context, filter WorkerListFilter) ([]WorkerResponse, error) {
	params := url.Values{}
	params.Set("size", "10000")
	if filter.Search != "" {
		params.Set("search", filter.Search)
	}
	return paginateJSON[WorkerResponse](ctx, c, "/api/v1/workers", params, "workers")
}
