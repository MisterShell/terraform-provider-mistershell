package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Client is an HTTP client for the MisterShell API.
type Client struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
}

// NewClient creates a MisterShell API client.
func NewClient(baseURL, apiKey string, httpClient *http.Client) *Client {
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		APIKey:     apiKey,
		HTTPClient: httpClient,
	}
}

// apiEnvelope is the standard MisterShell API response wrapper.
type apiEnvelope struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
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

// doRequest executes an HTTP request and returns the unwrapped data payload.
func (c *Client) doRequest(ctx context.Context, method, path string, body interface{}) (json.RawMessage, error) {
	reqURL := c.BaseURL + path

	var bodyReader io.Reader
	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshaling request body: %w", err)
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, &NotFoundError{Path: path}
	}

	// DELETE with 204 has no body
	if resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}

	var envelope apiEnvelope
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		return nil, fmt.Errorf("decoding response (status %d, body: %s): %w", resp.StatusCode, string(respBody), err)
	}

	if !envelope.Success {
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, envelope.Message)
	}

	return envelope.Data, nil
}

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
	ID          int64           `json:"id"`
	Name        string          `json:"name"`
	Kind        string          `json:"kind"`
	Description *string         `json:"description"`
	ParentID    *int64          `json:"parent_id"`
	Latitude    *float64        `json:"latitude"`
	Longitude   *float64        `json:"longitude"`
	ExtraData   json.RawMessage `json:"extra_data"`
	CreatedAt   string          `json:"created_at"`
	UpdatedAt   string          `json:"updated_at"`
}

func (c *Client) CreateLocation(ctx context.Context, input LocationCreateInput) (*LocationResponse, error) {
	data, err := c.doRequest(ctx, http.MethodPost, "/api/v1/locations/", input)
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
	params.Set("page", "1")
	params.Set("size", "100")
	if filter.Search != "" {
		params.Set("search", filter.Search)
	}
	if filter.ParentID != nil {
		params.Set("parent_id", fmt.Sprintf("%d", *filter.ParentID))
	}
	if filter.Kind != "" {
		params.Set("kind", filter.Kind)
	}
	data, err := c.doRequest(ctx, http.MethodGet, "/api/v1/locations/?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	var locs []LocationResponse
	if err := json.Unmarshal(data, &locs); err != nil {
		return nil, fmt.Errorf("decoding locations: %w", err)
	}
	return locs, nil
}

// ---------------------------------------------------------------------------
// Network Resource API models
// ---------------------------------------------------------------------------

type NetworkResourceCreateInput struct {
	Name          string          `json:"name"`
	ResourceType  string          `json:"resource_type"`
	ExternalID    string          `json:"external_id"`
	LocationID    int64           `json:"location_id"`
	ConnectorData json.RawMessage `json:"connector_data,omitempty"`
	CredentialID  *int64          `json:"credential_id,omitempty"`
	ExtraData     json.RawMessage `json:"extra_data,omitempty"`
}

type NetworkResourceUpdateInput struct {
	Name          *string         `json:"name,omitempty"`
	ExternalID    *string         `json:"external_id,omitempty"`
	LocationID    *int64          `json:"location_id,omitempty"`
	ConnectorData json.RawMessage `json:"connector_data"`
	CredentialID  *int64          `json:"credential_id"`
	ExtraData     json.RawMessage `json:"extra_data"`
	IsEnabled     *bool           `json:"is_enabled,omitempty"`
}

type NetworkResourceResponse struct {
	ID                    int64           `json:"id"`
	Name                  string          `json:"name"`
	ResourceType          string          `json:"resource_type"`
	ConnectorID           string          `json:"connector_id"`
	ExternalID            string          `json:"external_id"`
	ConnectorData         json.RawMessage `json:"connector_data"`
	CredentialID          *int64          `json:"credential_id"`
	LocationID            *int64          `json:"location_id"`
	Status                string          `json:"status"`
	HealthStatus          string          `json:"health_status"`
	IsEnabled             bool            `json:"is_enabled"`
	ExtraData             json.RawMessage `json:"extra_data"`
	CreatedAt             string          `json:"created_at"`
	UpdatedAt             string          `json:"updated_at"`
	LastConnectivityCheck *string         `json:"last_connectivity_check"`
	LastCollectionAt      *string         `json:"last_collection_at"`
	NextCollectionAt      *string         `json:"next_collection_at"`
	LastSnapshotAt        *string         `json:"last_snapshot_at"`
	LastHealthAt          *string         `json:"last_health_at"`
}

func (c *Client) CreateNetworkResource(ctx context.Context, input NetworkResourceCreateInput) (*NetworkResourceResponse, error) {
	data, err := c.doRequest(ctx, http.MethodPost, "/api/v1/resources/", input)
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
	HealthStatus string
	IsEnabled    *bool
}

func (c *Client) ListNetworkResources(ctx context.Context, filter NetworkResourceListFilter) ([]NetworkResourceResponse, error) {
	params := url.Values{}
	params.Set("page", "1")
	params.Set("size", "100")
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
	if filter.HealthStatus != "" {
		params.Set("health_status", filter.HealthStatus)
	}
	if filter.IsEnabled != nil {
		params.Set("is_enabled", fmt.Sprintf("%t", *filter.IsEnabled))
	}
	data, err := c.doRequest(ctx, http.MethodGet, "/api/v1/resources/?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	var resources []NetworkResourceResponse
	if err := json.Unmarshal(data, &resources); err != nil {
		return nil, fmt.Errorf("decoding network resources: %w", err)
	}
	return resources, nil
}

// ---------------------------------------------------------------------------
// Credential API models
// ---------------------------------------------------------------------------

type CredentialCreateInput struct {
	Name                string          `json:"name"`
	CredentialType      string          `json:"credential_type"`
	CredentialData      json.RawMessage `json:"credential_data"`
	Description         *string         `json:"description,omitempty"`
	RequiresUserMapping *bool           `json:"requires_user_mapping,omitempty"`
	ExtraData           json.RawMessage `json:"extra_data,omitempty"`
}

type CredentialUpdateInput struct {
	Name                *string         `json:"name,omitempty"`
	Description         *string         `json:"description"`
	CredentialType      *string         `json:"credential_type,omitempty"`
	CredentialData      json.RawMessage `json:"credential_data,omitempty"`
	RequiresUserMapping *bool           `json:"requires_user_mapping,omitempty"`
	ExtraData           json.RawMessage `json:"extra_data"`
}

type CredentialResponse struct {
	ID                  int64           `json:"id"`
	Name                string          `json:"name"`
	Description         *string         `json:"description"`
	CredentialType      string          `json:"credential_type"`
	RequiresUserMapping bool            `json:"requires_user_mapping"`
	CredentialData      json.RawMessage `json:"credential_data"`
	ExtraData           json.RawMessage `json:"extra_data"`
	CreatedAt           string          `json:"created_at"`
	UpdatedAt           string          `json:"updated_at"`
}

func (c *Client) CreateCredential(ctx context.Context, input CredentialCreateInput) (*CredentialResponse, error) {
	data, err := c.doRequest(ctx, http.MethodPost, "/api/v1/credentials/", input)
	if err != nil {
		return nil, err
	}
	var cred CredentialResponse
	if err := json.Unmarshal(data, &cred); err != nil {
		return nil, fmt.Errorf("decoding credential: %w", err)
	}
	return &cred, nil
}

func (c *Client) GetCredential(ctx context.Context, id int64) (*CredentialResponse, error) {
	data, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/api/v1/credentials/%d", id), nil)
	if err != nil {
		return nil, err
	}
	var cred CredentialResponse
	if err := json.Unmarshal(data, &cred); err != nil {
		return nil, fmt.Errorf("decoding credential: %w", err)
	}
	return &cred, nil
}

func (c *Client) UpdateCredential(ctx context.Context, id int64, input CredentialUpdateInput) (*CredentialResponse, error) {
	data, err := c.doRequest(ctx, http.MethodPatch, fmt.Sprintf("/api/v1/credentials/%d", id), input)
	if err != nil {
		return nil, err
	}
	var cred CredentialResponse
	if err := json.Unmarshal(data, &cred); err != nil {
		return nil, fmt.Errorf("decoding credential: %w", err)
	}
	return &cred, nil
}

func (c *Client) DeleteCredential(ctx context.Context, id int64) error {
	_, err := c.doRequest(ctx, http.MethodDelete, fmt.Sprintf("/api/v1/credentials/%d", id), nil)
	return err
}

type CredentialListFilter struct {
	Search         string
	CredentialType string
}

func (c *Client) ListCredentials(ctx context.Context, filter CredentialListFilter) ([]CredentialResponse, error) {
	params := url.Values{}
	params.Set("page", "1")
	params.Set("size", "100")
	if filter.Search != "" {
		params.Set("search", filter.Search)
	}
	if filter.CredentialType != "" {
		params.Set("credential_type", filter.CredentialType)
	}
	data, err := c.doRequest(ctx, http.MethodGet, "/api/v1/credentials/?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	var creds []CredentialResponse
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("decoding credentials: %w", err)
	}
	return creds, nil
}
