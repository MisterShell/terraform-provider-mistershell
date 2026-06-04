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

// ---------------------------------------------------------------------------
// Tag API models
// ---------------------------------------------------------------------------

type TagCreateInput struct {
	Name        string  `json:"name"`
	Color       string  `json:"color,omitempty"` // backend defaults "grey"
	Description *string `json:"description,omitempty"`
}

type TagUpdateInput struct {
	Name        *string `json:"name,omitempty"`
	Color       *string `json:"color,omitempty"` // color min_length 1: never send null
	Description *string `json:"description"`     // no omitempty: explicit null clears
}

type TagResponse struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	Color       string  `json:"color"`
	Description *string `json:"description"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

// TagListItem is the List endpoint shape (resource_count, no timestamps).
type TagListItem struct {
	ID            int64   `json:"id"`
	Name          string  `json:"name"`
	Color         string  `json:"color"`
	Description   *string `json:"description"`
	ResourceCount int64   `json:"resource_count"`
}

type TagListFilter struct {
	Search string
}

// TagAssignmentInput is the resource_ids assignment body for the whole-list PUT.
type TagAssignmentInput struct {
	ResourceIDs []int64 `json:"resource_ids"`
}

func (c *Client) CreateTag(ctx context.Context, input TagCreateInput) (*TagResponse, error) {
	data, err := c.doRequest(ctx, http.MethodPost, "/api/v1/tags/", input)
	if err != nil {
		return nil, err
	}
	var tag TagResponse
	if err := json.Unmarshal(data, &tag); err != nil {
		return nil, fmt.Errorf("decoding tag: %w", err)
	}
	return &tag, nil
}

func (c *Client) GetTag(ctx context.Context, id int64) (*TagResponse, error) {
	data, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/api/v1/tags/%d", id), nil)
	if err != nil {
		return nil, err
	}
	var tag TagResponse
	if err := json.Unmarshal(data, &tag); err != nil {
		return nil, fmt.Errorf("decoding tag: %w", err)
	}
	return &tag, nil
}

func (c *Client) UpdateTag(ctx context.Context, id int64, input TagUpdateInput) (*TagResponse, error) {
	data, err := c.doRequest(ctx, http.MethodPatch, fmt.Sprintf("/api/v1/tags/%d", id), input)
	if err != nil {
		return nil, err
	}
	var tag TagResponse
	if err := json.Unmarshal(data, &tag); err != nil {
		return nil, fmt.Errorf("decoding tag: %w", err)
	}
	return &tag, nil
}

func (c *Client) DeleteTag(ctx context.Context, id int64) error {
	_, err := c.doRequest(ctx, http.MethodDelete, fmt.Sprintf("/api/v1/tags/%d", id), nil)
	return err
}

func (c *Client) ListTags(ctx context.Context, filter TagListFilter) ([]TagListItem, error) {
	params := url.Values{}
	params.Set("page", "1")
	params.Set("size", "10000")
	if filter.Search != "" {
		params.Set("search", filter.Search)
	}
	data, err := c.doRequest(ctx, http.MethodGet, "/api/v1/tags/?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	var tags []TagListItem
	if err := json.Unmarshal(data, &tags); err != nil {
		return nil, fmt.Errorf("decoding tags: %w", err)
	}
	return tags, nil
}

// SetTagAssignments replaces the whole set of resource ids assigned to the tag.
// An empty slice clears all assignments (sends [], not null).
func (c *Client) SetTagAssignments(ctx context.Context, id int64, resourceIDs []int64) error {
	if resourceIDs == nil {
		resourceIDs = []int64{}
	}
	_, err := c.doRequest(ctx, http.MethodPut, fmt.Sprintf("/api/v1/tags/%d/assignments", id), TagAssignmentInput{ResourceIDs: resourceIDs})
	return err
}

func (c *Client) GetTagResourceIDs(ctx context.Context, id int64) ([]int64, error) {
	data, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/api/v1/tags/%d/resource-ids", id), nil)
	if err != nil {
		return nil, err
	}
	var ids []int64
	if err := json.Unmarshal(data, &ids); err != nil {
		return nil, fmt.Errorf("decoding tag resource ids: %w", err)
	}
	return ids, nil
}

// ---------------------------------------------------------------------------
// Role API models
// ---------------------------------------------------------------------------

type RoleCreateInput struct {
	Name             string  `json:"name"`
	Description      *string `json:"description,omitempty"`
	ScopeLocationIDs []int64 `json:"scope_location_ids,omitempty"` // empty = all locations
}

type RoleUpdateInput struct {
	Name             *string  `json:"name,omitempty"`
	Description      *string  `json:"description"`        // no omitempty: explicit null clears
	ScopeLocationIDs *[]int64 `json:"scope_location_ids"` // no omitempty: pointer-to-slice so we can send [] to clear scope, or omit
}

type RoleResponse struct { // POST/PATCH
	ID               int64   `json:"id"`
	Name             string  `json:"name"`
	Description      *string `json:"description"`
	ScopeLocationIDs []int64 `json:"scope_location_ids"`
	CreatedAt        string  `json:"created_at"`
	UpdatedAt        string  `json:"updated_at"`
}

type RoleDetailResponse struct { // GET + List
	RoleResponse
	Permissions []string `json:"permissions"`
}

type RoleListFilter struct {
	Search string
}

func (c *Client) CreateRole(ctx context.Context, input RoleCreateInput) (*RoleResponse, error) {
	data, err := c.doRequest(ctx, http.MethodPost, "/api/v1/roles/", input)
	if err != nil {
		return nil, err
	}
	var role RoleResponse
	if err := json.Unmarshal(data, &role); err != nil {
		return nil, fmt.Errorf("decoding role: %w", err)
	}
	return &role, nil
}

func (c *Client) GetRole(ctx context.Context, id int64) (*RoleDetailResponse, error) {
	data, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/api/v1/roles/%d", id), nil)
	if err != nil {
		return nil, err
	}
	var role RoleDetailResponse
	if err := json.Unmarshal(data, &role); err != nil {
		return nil, fmt.Errorf("decoding role: %w", err)
	}
	return &role, nil
}

func (c *Client) UpdateRole(ctx context.Context, id int64, input RoleUpdateInput) (*RoleResponse, error) {
	data, err := c.doRequest(ctx, http.MethodPatch, fmt.Sprintf("/api/v1/roles/%d", id), input)
	if err != nil {
		return nil, err
	}
	var role RoleResponse
	if err := json.Unmarshal(data, &role); err != nil {
		return nil, fmt.Errorf("decoding role: %w", err)
	}
	return &role, nil
}

func (c *Client) DeleteRole(ctx context.Context, id int64) error {
	_, err := c.doRequest(ctx, http.MethodDelete, fmt.Sprintf("/api/v1/roles/%d", id), nil)
	return err
}

func (c *Client) ListRoles(ctx context.Context, filter RoleListFilter) ([]RoleDetailResponse, error) {
	params := url.Values{}
	params.Set("page", "1")
	params.Set("size", "10000")
	if filter.Search != "" {
		params.Set("search", filter.Search)
	}
	data, err := c.doRequest(ctx, http.MethodGet, "/api/v1/roles/?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	var roles []RoleDetailResponse
	if err := json.Unmarshal(data, &roles); err != nil {
		return nil, fmt.Errorf("decoding roles: %w", err)
	}
	return roles, nil
}

// AddRolePermission assigns a single permission name to a role. The name is a
// path param containing dots and '*', so it is URL-path-escaped.
func (c *Client) AddRolePermission(ctx context.Context, id int64, name string) error {
	_, err := c.doRequest(ctx, http.MethodPost, fmt.Sprintf("/api/v1/roles/%d/permissions/%s", id, url.PathEscape(name)), nil)
	return err
}

// RemoveRolePermission unassigns a single permission name from a role. Idempotent server-side.
func (c *Client) RemoveRolePermission(ctx context.Context, id int64, name string) error {
	_, err := c.doRequest(ctx, http.MethodDelete, fmt.Sprintf("/api/v1/roles/%d/permissions/%s", id, url.PathEscape(name)), nil)
	return err
}

// ---------------------------------------------------------------------------
// Permission API models (read-only registry)
// ---------------------------------------------------------------------------

type PermissionInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Module      string `json:"module"`
	Action      string `json:"action"`
}

type PermissionListFilter struct {
	Module string
	Search string
}

func (c *Client) ListPermissions(ctx context.Context, filter PermissionListFilter) ([]PermissionInfo, error) {
	params := url.Values{}
	params.Set("page", "1")
	params.Set("size", "10000")
	if filter.Module != "" {
		params.Set("module", filter.Module)
	}
	if filter.Search != "" {
		params.Set("search", filter.Search)
	}
	data, err := c.doRequest(ctx, http.MethodGet, "/api/v1/permissions/?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	// PermissionWithRolesInfo is a superset; we only decode the exposed fields.
	var perms []PermissionInfo
	if err := json.Unmarshal(data, &perms); err != nil {
		return nil, fmt.Errorf("decoding permissions: %w", err)
	}
	return perms, nil
}

func (c *Client) GetPermission(ctx context.Context, name string) (*PermissionInfo, error) {
	data, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/api/v1/permissions/%s", url.PathEscape(name)), nil)
	if err != nil {
		return nil, err
	}
	var perm PermissionInfo
	if err := json.Unmarshal(data, &perm); err != nil {
		return nil, fmt.Errorf("decoding permission: %w", err)
	}
	return &perm, nil
}

func (c *Client) ListPermissionModules(ctx context.Context) ([]string, error) {
	data, err := c.doRequest(ctx, http.MethodGet, "/api/v1/permissions/modules", nil)
	if err != nil {
		return nil, err
	}
	var modules []string
	if err := json.Unmarshal(data, &modules); err != nil {
		return nil, fmt.Errorf("decoding permission modules: %w", err)
	}
	return modules, nil
}

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

// LogDestinationTestResult is the /{id}/test payload (always 200).
type LogDestinationTestResult struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
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
	params.Set("page", "1")
	params.Set("size", "10000")
	if filter.Search != "" {
		params.Set("search", filter.Search)
	}
	data, err := c.doRequest(ctx, http.MethodGet, "/api/v1/log-destinations?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	var dests []LogDestinationResponse
	if err := json.Unmarshal(data, &dests); err != nil {
		return nil, fmt.Errorf("decoding log destinations: %w", err)
	}
	return dests, nil
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

// TestLogDestination sends a synthetic event (POST …/{id}/test). Always 200 with
// a {success,error} payload. Not surfaced as a resource/data source; used by e2e.
func (c *Client) TestLogDestination(ctx context.Context, id int64) (*LogDestinationTestResult, error) {
	data, err := c.doRequest(ctx, http.MethodPost, fmt.Sprintf("/api/v1/log-destinations/%d/test", id), nil)
	if err != nil {
		return nil, err
	}
	var result LogDestinationTestResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("decoding log destination test result: %w", err)
	}
	return &result, nil
}

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
	params.Set("page", "1")
	params.Set("size", "10000")
	if filter.Search != "" {
		params.Set("search", filter.Search)
	}
	data, err := c.doRequest(ctx, http.MethodGet, "/api/v1/settings/?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	var settings []SettingResponse
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, fmt.Errorf("decoding settings: %w", err)
	}
	return settings, nil
}
