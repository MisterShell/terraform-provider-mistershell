package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// ---------------------------------------------------------------------------
// Network Resource API models
// ---------------------------------------------------------------------------

// ResourceTypeInfo is one installed RDK native type returned by MisterShell.
type ResourceTypeInfo struct {
	Type            string          `json:"type"`
	EngineID        string          `json:"engine_id"`
	DisplayName     string          `json:"display_name"`
	Description     string          `json:"description"`
	ConnectorSchema json.RawMessage `json:"connector_schema"`
	CredentialTypes []string        `json:"credential_types"`
}

func (c *Client) ListResourceTypes(ctx context.Context) ([]ResourceTypeInfo, error) {
	data, err := c.doRequest(ctx, http.MethodGet, "/api/v1/resources/types", nil)
	if err != nil {
		return nil, err
	}
	var resourceTypes []ResourceTypeInfo
	if err := json.Unmarshal(data, &resourceTypes); err != nil {
		return nil, fmt.Errorf("decoding resource types: %w", err)
	}
	return resourceTypes, nil
}

func (c *Client) FindResourceType(ctx context.Context, typeID string) (*ResourceTypeInfo, error) {
	resourceTypes, err := c.ListResourceTypes(ctx)
	if err != nil {
		return nil, err
	}
	for i := range resourceTypes {
		if resourceTypes[i].Type == typeID {
			return &resourceTypes[i], nil
		}
	}
	return nil, fmt.Errorf("resource type %q is not installed in MisterShell", typeID)
}

type NetworkResourceCreateInput struct {
	IsEnabled      *bool           `json:"is_enabled,omitempty"`
	Name           string          `json:"name"`
	ResourceType   string          `json:"resource_type"`
	DiscoveryToken string          `json:"discovery_token"`
	LocationID     int64           `json:"location_id"`
	ConnectorData  json.RawMessage `json:"connector_data,omitempty"`
	CredentialID   *int64          `json:"credential_id,omitempty"`
	// SshgwAllowAPIKey is unknown to MisterShell <= 2.0.13, whose create
	// schema forbids extra fields (HTTP 422). Leave it nil unless the caller
	// needs a non-default value.
	SshgwAllowAPIKey *bool `json:"sshgw_allow_api_key,omitempty"`
}

type NetworkResourceUpdateInput struct {
	Name             *string         `json:"name,omitempty"`
	LocationID       *int64          `json:"location_id,omitempty"`
	ConnectorData    json.RawMessage `json:"connector_data,omitempty"`
	CredentialID     *int64          `json:"credential_id,omitempty"`
	IsEnabled        *bool           `json:"is_enabled,omitempty"`
	SshgwAllowAPIKey *bool           `json:"sshgw_allow_api_key,omitempty"`
}

type EffectiveCoordinates struct {
	Latitude           float64 `json:"latitude"`
	Longitude          float64 `json:"longitude"`
	SourceLocationID   *int64  `json:"source_location_id"`
	SourceLocationName *string `json:"source_location_name"`
}

type NetworkResourceResponse struct {
	ID                     int64                 `json:"id"`
	Name                   string                `json:"name"`
	ResourceType           string                `json:"resource_type"`
	ConnectorID            string                `json:"connector_id"`
	ExternalID             *string               `json:"external_id"`
	ConnectorData          json.RawMessage       `json:"connector_data"`
	CredentialID           *int64                `json:"credential_id"`
	LocationID             *int64                `json:"location_id"`
	Status                 string                `json:"status"`
	IsEnabled              bool                  `json:"is_enabled"`
	SshgwAllowAPIKey       bool                  `json:"sshgw_allow_api_key"` // absent on MisterShell <= 2.0.13: decodes as false
	ExtraData              json.RawMessage       `json:"extra_data"`
	CreatedAt              string                `json:"created_at"`
	UpdatedAt              string                `json:"updated_at"`
	LastConnectivityCheck  *string               `json:"last_connectivity_check"`
	LastConnectivityResult json.RawMessage       `json:"last_connectivity_result"`
	LastCollectionAt       *string               `json:"last_collection_at"`
	NextCollectionAt       *string               `json:"next_collection_at"`
	EffectiveCoordinates   *EffectiveCoordinates `json:"effective_coordinates"`
}

type DiscoveryTestInput struct {
	EngineID       string          `json:"engine_id"`
	ExpectedTypeID *string         `json:"expected_type_id,omitempty"`
	CredentialID   int64           `json:"credential_id"`
	ConnectorData  json.RawMessage `json:"connector_data"`
	LocationID     int64           `json:"location_id"`
}

type discoveryTestResponse struct {
	TaskID                   int64   `json:"task_id"`
	Status                   string  `json:"status"`
	DiscoveryToken           *string `json:"discovery_token"`
	DiscoveryProofTTLSeconds *int64  `json:"discovery_proof_ttl_seconds"`
}

type DiscoveryProofStatusInput struct {
	DiscoveryToken string `json:"discovery_token"`
}

type DiscoveryProofStatusResponse struct {
	State string `json:"state"`
}

type WorkerTaskDetailResponse struct {
	ID            int64           `json:"id"`
	Status        string          `json:"status"`
	Result        json.RawMessage `json:"result"`
	ResultSuccess *bool           `json:"result_success"`
	ResultMessage *string         `json:"result_message"`
	ErrorMessage  *string         `json:"error_message"`
}

type discoveryTaskResult struct {
	Discovery struct {
		Status   string  `json:"status"`
		EngineID string  `json:"engine_id"`
		TypeID   *string `json:"type_id"`
	} `json:"discovery"`
}

type DiscoveryResult struct {
	TaskID         int64
	DiscoveryToken string
	EngineID       string
	ResourceType   string
}

func (c *Client) DiscoverNetworkResource(ctx context.Context, input DiscoveryTestInput) (*DiscoveryResult, error) {
	started, err := c.startNetworkResourceDiscovery(ctx, input)
	if err != nil {
		return nil, err
	}
	if started.DiscoveryToken == nil || *started.DiscoveryToken == "" {
		return nil, fmt.Errorf("resource discovery task %d did not return a discovery token", started.TaskID)
	}

	discovered, err := c.waitNetworkResourceDiscovery(ctx, started.TaskID)
	if err != nil {
		return nil, err
	}
	if err := c.waitDiscoveryProof(ctx, *started.DiscoveryToken); err != nil {
		return nil, err
	}

	return &DiscoveryResult{
		TaskID:         started.TaskID,
		DiscoveryToken: *started.DiscoveryToken,
		EngineID:       discovered.Discovery.EngineID,
		ResourceType:   *discovered.Discovery.TypeID,
	}, nil
}

func (c *Client) startNetworkResourceDiscovery(ctx context.Context, input DiscoveryTestInput) (*discoveryTestResponse, error) {
	data, err := c.doRequest(ctx, http.MethodPost, "/api/v1/resources/test/discovery", input)
	if err != nil {
		return nil, err
	}
	var started discoveryTestResponse
	if err := json.Unmarshal(data, &started); err != nil {
		return nil, fmt.Errorf("decoding resource discovery response: %w", err)
	}
	return &started, nil
}

func (c *Client) getWorkerTask(ctx context.Context, taskID int64) (*WorkerTaskDetailResponse, error) {
	data, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/api/v1/worker-tasks/%d", taskID), nil)
	if err != nil {
		return nil, err
	}
	var task WorkerTaskDetailResponse
	if err := json.Unmarshal(data, &task); err != nil {
		return nil, fmt.Errorf("decoding worker task %d: %w", taskID, err)
	}
	return &task, nil
}

func (c *Client) waitNetworkResourceDiscovery(ctx context.Context, taskID int64) (*discoveryTaskResult, error) {
	for {
		task, err := c.getWorkerTask(ctx, taskID)
		if err != nil {
			return nil, err
		}
		switch task.Status {
		case "pending", "queued", "cancel_requested", "running":
			if err := c.waitRetry(ctx, c.pollInterval); err != nil {
				return nil, err
			}
		case "success":
			if task.ResultSuccess == nil || !*task.ResultSuccess {
				return nil, fmt.Errorf("resource discovery task %d reported unsuccessful completion: %s", taskID, taskMessage(task))
			}
			var result discoveryTaskResult
			if err := json.Unmarshal(task.Result, &result); err != nil {
				return nil, fmt.Errorf("decoding resource discovery task %d result: %w", taskID, err)
			}
			if result.Discovery.TypeID == nil || *result.Discovery.TypeID == "" || result.Discovery.EngineID == "" {
				return nil, fmt.Errorf("resource discovery task %d returned no selected resource type or engine", taskID)
			}
			return &result, nil
		case "failed", "cancelled", "timeout", "no_worker":
			return nil, fmt.Errorf("resource discovery task %d ended with status %s: %s", taskID, task.Status, taskMessage(task))
		default:
			return nil, fmt.Errorf("resource discovery task %d returned unknown status %q", taskID, task.Status)
		}
	}
}

func (c *Client) waitDiscoveryProof(ctx context.Context, discoveryToken string) error {
	for {
		data, err := c.doRequest(ctx, http.MethodPost, "/api/v1/resources/test/discovery/proof-status", DiscoveryProofStatusInput{DiscoveryToken: discoveryToken})
		if err != nil {
			return err
		}
		var proof DiscoveryProofStatusResponse
		if err := json.Unmarshal(data, &proof); err != nil {
			return fmt.Errorf("decoding resource discovery proof status: %w", err)
		}
		switch proof.State {
		case "ready":
			return nil
		case "pending":
			if err := c.waitRetry(ctx, c.pollInterval); err != nil {
				return err
			}
		case "failed", "missing":
			return fmt.Errorf("resource discovery proof ended with state %s", proof.State)
		default:
			return fmt.Errorf("resource discovery proof returned unknown state %q", proof.State)
		}
	}
}

func taskMessage(task *WorkerTaskDetailResponse) string {
	if task.ErrorMessage != nil && *task.ErrorMessage != "" {
		return *task.ErrorMessage
	}
	if task.ResultMessage != nil && *task.ResultMessage != "" {
		return *task.ResultMessage
	}
	return "no task error detail"
}

func (c *Client) CreateNetworkResource(ctx context.Context, input NetworkResourceCreateInput) (*NetworkResourceResponse, error) {
	data, err := c.doRequest(ctx, http.MethodPost, "/api/v1/resources", input)
	if err != nil {
		return nil, err
	}
	var res NetworkResourceResponse
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, fmt.Errorf("decoding network resource: %w", err)
	}
	return &res, nil
}

func (c *Client) GetNetworkResource(ctx context.Context, id int64) (*NetworkResourceResponse, error) {
	data, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/api/v1/resources/%d", id), nil)
	if err != nil {
		return nil, err
	}
	var res NetworkResourceResponse
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, fmt.Errorf("decoding network resource: %w", err)
	}
	return &res, nil
}

func (c *Client) UpdateNetworkResource(ctx context.Context, id int64, input NetworkResourceUpdateInput) (*NetworkResourceResponse, error) {
	data, err := c.doRequest(ctx, http.MethodPatch, fmt.Sprintf("/api/v1/resources/%d", id), input)
	if err != nil {
		return nil, err
	}
	var res NetworkResourceResponse
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, fmt.Errorf("decoding network resource: %w", err)
	}
	return &res, nil
}

func (c *Client) DeleteNetworkResource(ctx context.Context, id int64) error {
	_, err := c.doRequest(ctx, http.MethodDelete, fmt.Sprintf("/api/v1/resources/%d", id), nil)
	return err
}

type NetworkResourceListFilter struct {
	Search       string
	ResourceType string
	LocationID   *int64
	Status       string
	IsEnabled    *bool
}

func (c *Client) ListNetworkResources(ctx context.Context, filter NetworkResourceListFilter) ([]NetworkResourceResponse, error) {
	params := url.Values{}
	params.Set("size", "10000")
	if filter.Search != "" {
		params.Set("search", filter.Search)
	}
	if filter.ResourceType != "" {
		params.Set("resource_type", filter.ResourceType)
	}
	if filter.LocationID != nil {
		params.Set("location_id", fmt.Sprintf("%d", *filter.LocationID))
	}
	if filter.Status != "" {
		params.Set("status", filter.Status)
	}
	if filter.IsEnabled != nil {
		params.Set("is_enabled", fmt.Sprintf("%t", *filter.IsEnabled))
	}
	return paginateJSON[NetworkResourceResponse](ctx, c, "/api/v1/resources", params, "network resources")
}
