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
	"strconv"
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
	sleep func(ctx context.Context, d time.Duration) error
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

	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshaling request body: %w", err)
		}
	}

	for attempt := 1; ; attempt++ {
		var bodyReader io.Reader
		if bodyBytes != nil {
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
		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("reading response: %w", err)
		}

		if resp.StatusCode == http.StatusTooManyRequests && attempt < retryMaxAttempts {
			wait := retryAfterDelay(resp.Header.Get("Retry-After"), attempt)
			if err := c.waitRetry(ctx, wait); err != nil {
				return nil, err
			}
			continue
		}

		if resp.StatusCode == http.StatusNotFound {
			return nil, &NotFoundError{Path: path}
		}
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
}

// retryAfterDelay parses a Retry-After header (delta-seconds) and falls back to
// capped exponential backoff when it is absent or unparseable.
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
	params.Set("size", "10000")
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
	ID                     int64           `json:"id"`
	Name                   string          `json:"name"`
	ResourceType           string          `json:"resource_type"`
	ConnectorID            string          `json:"connector_id"`
	ExternalID             string          `json:"external_id"`
	ConnectorData          json.RawMessage `json:"connector_data"`
	CredentialID           *int64          `json:"credential_id"`
	LocationID             *int64          `json:"location_id"`
	Status                 string          `json:"status"`
	IsEnabled              bool            `json:"is_enabled"`
	ExtraData              json.RawMessage `json:"extra_data"`
	CreatedAt              string          `json:"created_at"`
	UpdatedAt              string          `json:"updated_at"`
	LastConnectivityCheck  *string         `json:"last_connectivity_check"`
	LastConnectivityResult json.RawMessage `json:"last_connectivity_result"`
	LastCollectionAt       *string         `json:"last_collection_at"`
	NextCollectionAt       *string         `json:"next_collection_at"`
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
	IsEnabled    *bool
}

func (c *Client) ListNetworkResources(ctx context.Context, filter NetworkResourceListFilter) ([]NetworkResourceResponse, error) {
	params := url.Values{}
	params.Set("page", "1")
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
	params.Set("size", "10000")
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

// ResourceTagAssignment is the body of PUT /resources/{id}/tags — the
// resource-centric counterpart to TagAssignmentInput. It replaces the resource's
// tag set exactly (an empty slice clears all tags).
type ResourceTagAssignment struct {
	TagIDs []int64 `json:"tag_ids"`
}

// SetResourceTags replaces the whole set of tag ids assigned to a resource via
// PUT /api/v1/resources/{id}/tags. An empty slice clears all tags (sends [], not
// null). This is the resource-centric ownership direction (vs SetTagAssignments,
// which is tag-centric); own each tag<->resource edge from one side only.
func (c *Client) SetResourceTags(ctx context.Context, resourceID int64, tagIDs []int64) ([]TagResponse, error) {
	if tagIDs == nil {
		tagIDs = []int64{}
	}
	// PUT returns an empty (NoneType) body, so re-read the authoritative tag set.
	if _, err := c.doRequest(ctx, http.MethodPut, fmt.Sprintf("/api/v1/resources/%d/tags", resourceID), ResourceTagAssignment{TagIDs: tagIDs}); err != nil {
		return nil, err
	}
	return c.GetResourceTags(ctx, resourceID)
}

// GetResourceTags returns the full tag objects assigned to a resource via
// GET /api/v1/resources/{id}/tags.
func (c *Client) GetResourceTags(ctx context.Context, resourceID int64) ([]TagResponse, error) {
	data, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/api/v1/resources/%d/tags", resourceID), nil)
	if err != nil {
		return nil, err
	}
	var tags []TagResponse
	if err := json.Unmarshal(data, &tags); err != nil {
		return nil, fmt.Errorf("decoding resource tags: %w", err)
	}
	return tags, nil
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
// User API models
// ---------------------------------------------------------------------------

type UserCreateInput struct {
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	// Password is required for LOCAL users and ignored by the backend for
	// external auth types. omitempty keeps it out of the body when empty.
	Password string `json:"password,omitempty"`
	AuthType string `json:"auth_type,omitempty"`
}

// UserUpdateInput carries a PATCH. Unlike the nullable "clear-to-null"
// UpdateInputs elsewhere (e.g. RoleUpdateInput.Description), none of these
// fields have clear semantics, so all use omitempty: a nil pointer is omitted
// (leave unchanged), and omitempty on a *pointer* checks nilness only, so a
// non-nil *bool(false) is still sent.
type UserUpdateInput struct {
	FirstName *string `json:"first_name,omitempty"`
	LastName  *string `json:"last_name,omitempty"`
	IsActive  *bool   `json:"is_active,omitempty"`
}

type UserResponse struct { // POST/PATCH
	ID            int64  `json:"id"`
	Email         string `json:"email"`
	FirstName     string `json:"first_name"`
	LastName      string `json:"last_name"`
	AuthType      string `json:"auth_type"`
	IsActive      bool   `json:"is_active"`
	EmailVerified bool   `json:"email_verified"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

type UserDetailResponse struct { // GET + List
	UserResponse
	Roles       []RoleResponse `json:"roles"`
	Permissions []string       `json:"permissions"`
}

type UserListFilter struct {
	Search string
}

type adminResetPasswordRequest struct {
	NewPassword string `json:"new_password"`
}

func (c *Client) CreateUser(ctx context.Context, input UserCreateInput) (*UserResponse, error) {
	data, err := c.doRequest(ctx, http.MethodPost, "/api/v1/users/", input)
	if err != nil {
		return nil, err
	}
	var user UserResponse
	if err := json.Unmarshal(data, &user); err != nil {
		return nil, fmt.Errorf("decoding user: %w", err)
	}
	return &user, nil
}

func (c *Client) GetUser(ctx context.Context, id int64) (*UserDetailResponse, error) {
	data, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/api/v1/users/%d", id), nil)
	if err != nil {
		return nil, err
	}
	var user UserDetailResponse
	if err := json.Unmarshal(data, &user); err != nil {
		return nil, fmt.Errorf("decoding user: %w", err)
	}
	return &user, nil
}

func (c *Client) UpdateUser(ctx context.Context, id int64, input UserUpdateInput) (*UserResponse, error) {
	data, err := c.doRequest(ctx, http.MethodPatch, fmt.Sprintf("/api/v1/users/%d", id), input)
	if err != nil {
		return nil, err
	}
	var user UserResponse
	if err := json.Unmarshal(data, &user); err != nil {
		return nil, fmt.Errorf("decoding user: %w", err)
	}
	return &user, nil
}

func (c *Client) DeleteUser(ctx context.Context, id int64) error {
	_, err := c.doRequest(ctx, http.MethodDelete, fmt.Sprintf("/api/v1/users/%d", id), nil)
	return err
}

func (c *Client) ListUsers(ctx context.Context, filter UserListFilter) ([]UserDetailResponse, error) {
	params := url.Values{}
	params.Set("page", "1")
	params.Set("size", "10000")
	if filter.Search != "" {
		params.Set("search", filter.Search)
	}
	data, err := c.doRequest(ctx, http.MethodGet, "/api/v1/users/?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	var users []UserDetailResponse
	if err := json.Unmarshal(data, &users); err != nil {
		return nil, fmt.Errorf("decoding users: %w", err)
	}
	return users, nil
}

// ResetUserPassword sets a new password for a user via the admin reset endpoint.
// The backend requires the new password to be at least 8 characters.
func (c *Client) ResetUserPassword(ctx context.Context, id int64, newPassword string) error {
	body := adminResetPasswordRequest{NewPassword: newPassword}
	_, err := c.doRequest(ctx, http.MethodPost, fmt.Sprintf("/api/v1/users/%d/reset-password", id), body)
	return err
}

// AddUserRole assigns a single role to a user. The backend may return 409 if the
// role is already assigned; callers surface that as an error.
func (c *Client) AddUserRole(ctx context.Context, userID, roleID int64) error {
	_, err := c.doRequest(ctx, http.MethodPost, fmt.Sprintf("/api/v1/users/%d/roles/%d", userID, roleID), nil)
	return err
}

// RemoveUserRole unassigns a single role from a user. Idempotent server-side.
func (c *Client) RemoveUserRole(ctx context.Context, userID, roleID int64) error {
	_, err := c.doRequest(ctx, http.MethodDelete, fmt.Sprintf("/api/v1/users/%d/roles/%d", userID, roleID), nil)
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

// ---------------------------------------------------------------------------
// Session-Policy ACL API models
// ---------------------------------------------------------------------------

// AclPattern is one ordered match pattern within an ACL.
type AclPattern struct {
	Pattern string `json:"pattern"`        // 1..512
	Type    string `json:"type,omitempty"` // "glob" (default) | "regex"
}

type AclCreateInput struct {
	Name        string       `json:"name"`                  // 1..128
	Description string       `json:"description,omitempty"` // default "", <=255
	Patterns    []AclPattern `json:"patterns"`              // default []
}

// AclUpdateInput is a partial PATCH-style PUT: the resource always sends name +
// description + patterns + enabled because it manages all of them.
type AclUpdateInput struct {
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Patterns    []AclPattern `json:"patterns"`
	Enabled     bool         `json:"enabled"`
}

type AclResponse struct {
	ID          int64        `json:"id"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Patterns    []AclPattern `json:"patterns"`
	IsBuiltin   bool         `json:"is_builtin"`
	Enabled     bool         `json:"enabled"`
	CreatedAt   string       `json:"created_at"`
	UpdatedAt   string       `json:"updated_at"`
}

type AclListFilter struct{ Search string }

func (c *Client) CreateAcl(ctx context.Context, input AclCreateInput) (*AclResponse, error) {
	data, err := c.doRequest(ctx, http.MethodPost, "/api/v1/session-policy/acls", input)
	if err != nil {
		return nil, err
	}
	var acl AclResponse
	if err := json.Unmarshal(data, &acl); err != nil {
		return nil, fmt.Errorf("decoding acl: %w", err)
	}
	return &acl, nil
}

func (c *Client) UpdateAcl(ctx context.Context, id int64, input AclUpdateInput) (*AclResponse, error) {
	data, err := c.doRequest(ctx, http.MethodPut, fmt.Sprintf("/api/v1/session-policy/acls/%d", id), input)
	if err != nil {
		return nil, err
	}
	var acl AclResponse
	if err := json.Unmarshal(data, &acl); err != nil {
		return nil, fmt.Errorf("decoding acl: %w", err)
	}
	return &acl, nil
}

func (c *Client) DeleteAcl(ctx context.Context, id int64) error {
	_, err := c.doRequest(ctx, http.MethodDelete, fmt.Sprintf("/api/v1/session-policy/acls/%d", id), nil)
	return err
}

func (c *Client) ListAcls(ctx context.Context, filter AclListFilter) ([]AclResponse, error) {
	params := url.Values{}
	params.Set("page", "1")
	params.Set("size", "1000")
	if filter.Search != "" {
		params.Set("search", filter.Search)
	}
	data, err := c.doRequest(ctx, http.MethodGet, "/api/v1/session-policy/acls?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	var acls []AclResponse
	if err := json.Unmarshal(data, &acls); err != nil {
		return nil, fmt.Errorf("decoding acls: %w", err)
	}
	return acls, nil
}

// GetAcl fetches a single ACL via GET /session-policy/acls/{id}. (Previously
// synthesized from the list endpoint; the backend added GET-single.)
func (c *Client) GetAcl(ctx context.Context, id int64) (*AclResponse, error) {
	data, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/api/v1/session-policy/acls/%d", id), nil)
	if err != nil {
		return nil, err
	}
	var acl AclResponse
	if err := json.Unmarshal(data, &acl); err != nil {
		return nil, fmt.Errorf("decoding acl: %w", err)
	}
	return &acl, nil
}

// ---------------------------------------------------------------------------
// Session-Policy Rule API models
// ---------------------------------------------------------------------------

// RuleCreateInput: empty selector lists mean "Any". resource_types reuse the
// inventory type strings.
type RuleCreateInput struct {
	Name          string   `json:"name,omitempty"`     // <=128, default ""
	Comment       *string  `json:"comment,omitempty"`  //
	Position      *int64   `json:"position,omitempty"` // 1-based; omit -> append at max+10
	ResourceTypes []string `json:"resource_types"`     // [] = Any
	SessionTypes  []string `json:"session_types"`      // [] = Any; items shell|graphical
	LocationIDs   []int64  `json:"location_ids"`
	TagIDs        []int64  `json:"tag_ids"`
	RoleIDs       []int64  `json:"role_ids"`
	CommandAclIDs []int64  `json:"command_acl_ids"`
	Action        string   `json:"action,omitempty"` // accept|deny, default accept
	Notify        bool     `json:"notify"`
	Log           bool     `json:"log"`
	Enabled       bool     `json:"enabled"`
}

// RuleUpdateInput is a full-replace PUT (RuleUpdate == RuleCreate). Comment uses
// *string with NO omitempty so clearing sends explicit null. Position is *int64
// (omit leaves it unchanged server-side: update only sets it if non-None).
type RuleUpdateInput struct {
	Name          string   `json:"name"`
	Comment       *string  `json:"comment"` // no omitempty: explicit null clears
	Position      *int64   `json:"position,omitempty"`
	ResourceTypes []string `json:"resource_types"`
	SessionTypes  []string `json:"session_types"`
	LocationIDs   []int64  `json:"location_ids"`
	TagIDs        []int64  `json:"tag_ids"`
	RoleIDs       []int64  `json:"role_ids"`
	CommandAclIDs []int64  `json:"command_acl_ids"`
	Action        string   `json:"action"`
	Notify        bool     `json:"notify"`
	Log           bool     `json:"log"`
	Enabled       bool     `json:"enabled"`
}

type RuleResponse struct {
	ID            int64    `json:"id"`
	Position      int64    `json:"position"`
	Name          string   `json:"name"`
	Comment       *string  `json:"comment"`
	ResourceTypes []string `json:"resource_types"`
	SessionTypes  []string `json:"session_types"`
	LocationIDs   []int64  `json:"location_ids"`
	TagIDs        []int64  `json:"tag_ids"`
	RoleIDs       []int64  `json:"role_ids"`
	CommandAclIDs []int64  `json:"command_acl_ids"`
	Action        string   `json:"action"`
	Notify        bool     `json:"notify"`
	Log           bool     `json:"log"`
	Enabled       bool     `json:"enabled"`
	HitCount      int64    `json:"hit_count"`
	LastHitAt     *string  `json:"last_hit_at"`
	CreatedAt     string   `json:"created_at"`
	UpdatedAt     string   `json:"updated_at"`
}

// RuleReorderInput is the whole-collection reorder payload (TESTS ONLY).
type RuleReorderInput struct {
	OrderedIDs []int64 `json:"ordered_ids"`
}

func (c *Client) CreateRule(ctx context.Context, input RuleCreateInput) (*RuleResponse, error) {
	data, err := c.doRequest(ctx, http.MethodPost, "/api/v1/session-policy/rules", input)
	if err != nil {
		return nil, err
	}
	var rule RuleResponse
	if err := json.Unmarshal(data, &rule); err != nil {
		return nil, fmt.Errorf("decoding rule: %w", err)
	}
	return &rule, nil
}

func (c *Client) UpdateRule(ctx context.Context, id int64, input RuleUpdateInput) (*RuleResponse, error) {
	data, err := c.doRequest(ctx, http.MethodPut, fmt.Sprintf("/api/v1/session-policy/rules/%d", id), input)
	if err != nil {
		return nil, err
	}
	var rule RuleResponse
	if err := json.Unmarshal(data, &rule); err != nil {
		return nil, fmt.Errorf("decoding rule: %w", err)
	}
	return &rule, nil
}

func (c *Client) DeleteRule(ctx context.Context, id int64) error {
	_, err := c.doRequest(ctx, http.MethodDelete, fmt.Sprintf("/api/v1/session-policy/rules/%d", id), nil)
	return err
}

// ListRules returns the full ordered list (not paginated).
func (c *Client) ListRules(ctx context.Context) ([]RuleResponse, error) {
	data, err := c.doRequest(ctx, http.MethodGet, "/api/v1/session-policy/rules", nil)
	if err != nil {
		return nil, err
	}
	var rules []RuleResponse
	if err := json.Unmarshal(data, &rules); err != nil {
		return nil, fmt.Errorf("decoding rules: %w", err)
	}
	return rules, nil
}

// GetRule fetches a single rule via GET /session-policy/rules/{id}. (Previously
// synthesized from the list endpoint; the backend added GET-single.)
func (c *Client) GetRule(ctx context.Context, id int64) (*RuleResponse, error) {
	data, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/api/v1/session-policy/rules/%d", id), nil)
	if err != nil {
		return nil, err
	}
	var rule RuleResponse
	if err := json.Unmarshal(data, &rule); err != nil {
		return nil, fmt.Errorf("decoding rule: %w", err)
	}
	return &rule, nil
}

// ReorderRules rewrites positions for the listed ids (TESTS ONLY — no resource
// lifecycle uses this; ordering is declarative via the position field).
func (c *Client) ReorderRules(ctx context.Context, orderedIDs []int64) error {
	if orderedIDs == nil {
		orderedIDs = []int64{}
	}
	_, err := c.doRequest(ctx, http.MethodPost, "/api/v1/session-policy/rules/reorder", RuleReorderInput{OrderedIDs: orderedIDs})
	return err
}

// ClearRuleHits resets a rule's hit counter (TESTS ONLY — not surfaced as a
// resource/data source).
func (c *Client) ClearRuleHits(ctx context.Context, id int64) (*RuleResponse, error) {
	data, err := c.doRequest(ctx, http.MethodPost, fmt.Sprintf("/api/v1/session-policy/rules/%d/clear-hits", id), nil)
	if err != nil {
		return nil, err
	}
	var rule RuleResponse
	if err := json.Unmarshal(data, &rule); err != nil {
		return nil, fmt.Errorf("decoding rule: %w", err)
	}
	return &rule, nil
}

// ---------------------------------------------------------------------------
// Auth-Provider API models
// ---------------------------------------------------------------------------

// AuthProviderCreateInput: config is an opaque per-provider_type blob; secrets
// are masked **** in the GET-single response and ABSENT from create/patch/list
// responses.
type AuthProviderCreateInput struct {
	Name         string          `json:"name"`                    // 1..100
	ProviderType string          `json:"provider_type"`           // LDAP|OIDC|SAML
	IsEnabled    *bool           `json:"is_enabled,omitempty"`    // default true
	DisplayOrder *int64          `json:"display_order,omitempty"` // >=0; omitted -> appended at max+1
	Config       json.RawMessage `json:"config"`                  // LDAPConfig|OIDCConfig|SAMLConfig
}

// AuthProviderUpdateInput is a PATCH: all fields optional. display_order is
// settable here (NOT on create). Pointers so we only send changed fields.
type AuthProviderUpdateInput struct {
	Name         *string         `json:"name,omitempty"`
	IsEnabled    *bool           `json:"is_enabled,omitempty"`
	DisplayOrder *int64          `json:"display_order,omitempty"`
	Config       json.RawMessage `json:"config,omitempty"` // omit to leave config untouched
}

// AuthProviderResponse is the create/list/patch response: NO config.
type AuthProviderResponse struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	ProviderType string `json:"provider_type"`
	IsEnabled    bool   `json:"is_enabled"`
	DisplayOrder int64  `json:"display_order"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

// AuthProviderDetailResponse is the GET-single response: adds masked config +
// mappings count.
type AuthProviderDetailResponse struct {
	AuthProviderResponse
	Config             json.RawMessage `json:"config"` // secrets masked ****
	GroupMappingsCount int64           `json:"group_mappings_count"`
}

type AuthProviderListFilter struct{ Search string }

type AuthProviderReorderItem struct {
	ID           int64 `json:"id"`
	DisplayOrder int64 `json:"display_order"`
}

// AuthProviderReorderInput is the whole-set reorder payload (TESTS ONLY).
type AuthProviderReorderInput struct {
	Items []AuthProviderReorderItem `json:"items"`
}

type AuthProviderTestResult struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Details json.RawMessage `json:"details,omitempty"`
}

func (c *Client) CreateAuthProvider(ctx context.Context, input AuthProviderCreateInput) (*AuthProviderResponse, error) {
	data, err := c.doRequest(ctx, http.MethodPost, "/api/v1/auth-providers/", input)
	if err != nil {
		return nil, err
	}
	var ap AuthProviderResponse
	if err := json.Unmarshal(data, &ap); err != nil {
		return nil, fmt.Errorf("decoding auth provider: %w", err)
	}
	return &ap, nil
}

// GetAuthProvider is a real GET-single returning the masked config.
func (c *Client) GetAuthProvider(ctx context.Context, id int64) (*AuthProviderDetailResponse, error) {
	data, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/api/v1/auth-providers/%d", id), nil)
	if err != nil {
		return nil, err
	}
	var ap AuthProviderDetailResponse
	if err := json.Unmarshal(data, &ap); err != nil {
		return nil, fmt.Errorf("decoding auth provider: %w", err)
	}
	return &ap, nil
}

func (c *Client) UpdateAuthProvider(ctx context.Context, id int64, input AuthProviderUpdateInput) (*AuthProviderResponse, error) {
	data, err := c.doRequest(ctx, http.MethodPatch, fmt.Sprintf("/api/v1/auth-providers/%d", id), input)
	if err != nil {
		return nil, err
	}
	var ap AuthProviderResponse
	if err := json.Unmarshal(data, &ap); err != nil {
		return nil, fmt.Errorf("decoding auth provider: %w", err)
	}
	return &ap, nil
}

func (c *Client) DeleteAuthProvider(ctx context.Context, id int64) error {
	_, err := c.doRequest(ctx, http.MethodDelete, fmt.Sprintf("/api/v1/auth-providers/%d", id), nil)
	return err
}

func (c *Client) ListAuthProviders(ctx context.Context, filter AuthProviderListFilter) ([]AuthProviderResponse, error) {
	params := url.Values{}
	params.Set("page", "1")
	params.Set("size", "10000")
	if filter.Search != "" {
		params.Set("search", filter.Search)
	}
	data, err := c.doRequest(ctx, http.MethodGet, "/api/v1/auth-providers/?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	var aps []AuthProviderResponse
	if err := json.Unmarshal(data, &aps); err != nil {
		return nil, fmt.Errorf("decoding auth providers: %w", err)
	}
	return aps, nil
}

// ReorderAuthProviders rewrites the whole contiguous order set (TESTS ONLY — no
// resource lifecycle uses this; ordering is declarative via display_order).
func (c *Client) ReorderAuthProviders(ctx context.Context, items []AuthProviderReorderItem) error {
	if items == nil {
		items = []AuthProviderReorderItem{}
	}
	_, err := c.doRequest(ctx, http.MethodPost, "/api/v1/auth-providers/reorder", AuthProviderReorderInput{Items: items})
	return err
}

// TestAuthProvider triggers a connection test (TESTS ONLY — not surfaced as a
// resource/data source).
func (c *Client) TestAuthProvider(ctx context.Context, id int64) (*AuthProviderTestResult, error) {
	data, err := c.doRequest(ctx, http.MethodPost, fmt.Sprintf("/api/v1/auth-providers/%d/test", id), nil)
	if err != nil {
		return nil, err
	}
	var result AuthProviderTestResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("decoding auth provider test result: %w", err)
	}
	return &result, nil
}

// ---------------------------------------------------------------------------
// Auth-Provider Group Mapping API models
// ---------------------------------------------------------------------------

type GroupMappingCreateInput struct {
	ExternalGroup string `json:"external_group"` // 1..500
	RoleID        int64  `json:"role_id"`
}

// GroupMappingUpdateInput is a PATCH: both optional.
type GroupMappingUpdateInput struct {
	ExternalGroup *string `json:"external_group,omitempty"`
	RoleID        *int64  `json:"role_id,omitempty"`
}

type GroupMappingResponse struct {
	ID             int64  `json:"id"`
	AuthProviderID int64  `json:"auth_provider_id"`
	ExternalGroup  string `json:"external_group"`
	RoleID         int64  `json:"role_id"`
	RoleName       string `json:"role_name"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

// BulkMappingResult is the /mappings/bulk response (TESTS ONLY).
type BulkMappingResult struct {
	Created []GroupMappingResponse `json:"created"`
}

// BulkMappingCreateInput is the /mappings/bulk request (TESTS ONLY).
type BulkMappingCreateInput struct {
	Mappings []GroupMappingCreateInput `json:"mappings"`
}

func (c *Client) CreateGroupMapping(ctx context.Context, providerID int64, input GroupMappingCreateInput) (*GroupMappingResponse, error) {
	data, err := c.doRequest(ctx, http.MethodPost, fmt.Sprintf("/api/v1/auth-providers/%d/mappings/", providerID), input)
	if err != nil {
		return nil, err
	}
	var m GroupMappingResponse
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("decoding group mapping: %w", err)
	}
	return &m, nil
}

func (c *Client) UpdateGroupMapping(ctx context.Context, providerID, mappingID int64, input GroupMappingUpdateInput) (*GroupMappingResponse, error) {
	data, err := c.doRequest(ctx, http.MethodPatch, fmt.Sprintf("/api/v1/auth-providers/%d/mappings/%d", providerID, mappingID), input)
	if err != nil {
		return nil, err
	}
	var m GroupMappingResponse
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("decoding group mapping: %w", err)
	}
	return &m, nil
}

func (c *Client) DeleteGroupMapping(ctx context.Context, providerID, mappingID int64) error {
	_, err := c.doRequest(ctx, http.MethodDelete, fmt.Sprintf("/api/v1/auth-providers/%d/mappings/%d", providerID, mappingID), nil)
	return err
}

func (c *Client) ListGroupMappings(ctx context.Context, providerID int64) ([]GroupMappingResponse, error) {
	data, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/api/v1/auth-providers/%d/mappings/", providerID), nil)
	if err != nil {
		return nil, err
	}
	var ms []GroupMappingResponse
	if err := json.Unmarshal(data, &ms); err != nil {
		return nil, fmt.Errorf("decoding group mappings: %w", err)
	}
	return ms, nil
}

// GetGroupMapping fetches a single mapping via the provider-scoped
// GET /auth-providers/{provider_id}/mappings/{mapping_id}. (Previously
// synthesized from the list endpoint; the backend added GET-single.) A 404 —
// whether the mapping or the parent provider is gone — maps to *NotFoundError.
func (c *Client) GetGroupMapping(ctx context.Context, providerID, mappingID int64) (*GroupMappingResponse, error) {
	data, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/api/v1/auth-providers/%d/mappings/%d", providerID, mappingID), nil)
	if err != nil {
		return nil, err
	}
	var m GroupMappingResponse
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("decoding group mapping: %w", err)
	}
	return &m, nil
}

// BulkCreateGroupMappings inserts several mappings at once (TESTS ONLY — not a
// resource; it is an imperative batch insert).
func (c *Client) BulkCreateGroupMappings(ctx context.Context, providerID int64, inputs []GroupMappingCreateInput) (*BulkMappingResult, error) {
	if inputs == nil {
		inputs = []GroupMappingCreateInput{}
	}
	data, err := c.doRequest(ctx, http.MethodPost, fmt.Sprintf("/api/v1/auth-providers/%d/mappings/bulk", providerID), BulkMappingCreateInput{Mappings: inputs})
	if err != nil {
		return nil, err
	}
	var result BulkMappingResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("decoding bulk mapping result: %w", err)
	}
	return &result, nil
}

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
}

// WorkerUpdateInput is a PATCH: Description omits omitempty so clearing sends
// explicit null.
type WorkerUpdateInput struct {
	Name         *string         `json:"name,omitempty"`
	Description  *string         `json:"description"`
	LocationID   *int64          `json:"location_id,omitempty"`
	Config       json.RawMessage `json:"config,omitempty"`
	ConfigSchema json.RawMessage `json:"config_schema,omitempty"`
	IsEnabled    *bool           `json:"is_enabled,omitempty"`
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

func (c *Client) CreateWorker(ctx context.Context, input WorkerCreateInput) (*WorkerResponse, error) {
	data, err := c.doRequest(ctx, http.MethodPost, "/api/v1/workers/", input)
	if err != nil {
		return nil, err
	}
	var worker WorkerResponse
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

// UpdateWorker patches a worker. The default worker (is_default=true) returns
// 403 on PATCH.
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
	params.Set("page", "1")
	params.Set("size", "10000")
	if filter.Search != "" {
		params.Set("search", filter.Search)
	}
	data, err := c.doRequest(ctx, http.MethodGet, "/api/v1/workers/?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	var workers []WorkerResponse
	if err := json.Unmarshal(data, &workers); err != nil {
		return nil, fmt.Errorf("decoding workers: %w", err)
	}
	return workers, nil
}

// RegenerateWorkerToken issues a fresh token for the worker, returned in
// WorkerResponse.Token. The default worker (is_default=true) returns 403; the
// previous token is invalidated and never readable again.
func (c *Client) RegenerateWorkerToken(ctx context.Context, id int64) (*WorkerResponse, error) {
	data, err := c.doRequest(ctx, http.MethodPost, fmt.Sprintf("/api/v1/workers/%d/regenerate-token", id), nil)
	if err != nil {
		return nil, err
	}
	var worker WorkerResponse
	if err := json.Unmarshal(data, &worker); err != nil {
		return nil, fmt.Errorf("decoding worker: %w", err)
	}
	return &worker, nil
}

// ---------------------------------------------------------------------------
// AI Model API models
// ---------------------------------------------------------------------------

type AIModelCreateInput struct {
	Name      string          `json:"name"`
	Provider  string          `json:"provider"`
	ModelID   string          `json:"model_id"`
	Config    json.RawMessage `json:"config,omitempty"`
	IsDefault *bool           `json:"is_default,omitempty"`
}

type AIModelUpdateInput struct {
	Name      *string         `json:"name,omitempty"`
	Provider  *string         `json:"provider,omitempty"`
	ModelID   *string         `json:"model_id,omitempty"`
	Config    json.RawMessage `json:"config,omitempty"`
	IsDefault *bool           `json:"is_default,omitempty"`
}

// AIModelResponse: config carries provider secrets (e.g. api_key) masked as
// "***" in reads.
type AIModelResponse struct {
	ID        int64           `json:"id"`
	Name      string          `json:"name"`
	Provider  string          `json:"provider"`
	ModelID   string          `json:"model_id"`
	Config    json.RawMessage `json:"config"`
	IsDefault bool            `json:"is_default"`
	CreatedAt string          `json:"created_at"`
	UpdatedAt string          `json:"updated_at"`
}

type AIModelListFilter struct {
	Search string
}

func (c *Client) CreateAIModel(ctx context.Context, input AIModelCreateInput) (*AIModelResponse, error) {
	data, err := c.doRequest(ctx, http.MethodPost, "/api/v1/ai/models", input)
	if err != nil {
		return nil, err
	}
	var model AIModelResponse
	if err := json.Unmarshal(data, &model); err != nil {
		return nil, fmt.Errorf("decoding ai model: %w", err)
	}
	return &model, nil
}

func (c *Client) GetAIModel(ctx context.Context, id int64) (*AIModelResponse, error) {
	data, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/api/v1/ai/models/%d", id), nil)
	if err != nil {
		return nil, err
	}
	var model AIModelResponse
	if err := json.Unmarshal(data, &model); err != nil {
		return nil, fmt.Errorf("decoding ai model: %w", err)
	}
	return &model, nil
}

func (c *Client) UpdateAIModel(ctx context.Context, id int64, input AIModelUpdateInput) (*AIModelResponse, error) {
	data, err := c.doRequest(ctx, http.MethodPatch, fmt.Sprintf("/api/v1/ai/models/%d", id), input)
	if err != nil {
		return nil, err
	}
	var model AIModelResponse
	if err := json.Unmarshal(data, &model); err != nil {
		return nil, fmt.Errorf("decoding ai model: %w", err)
	}
	return &model, nil
}

// DeleteAIModel removes a model. Returns 409 if the model is referenced by an
// agent.
func (c *Client) DeleteAIModel(ctx context.Context, id int64) error {
	_, err := c.doRequest(ctx, http.MethodDelete, fmt.Sprintf("/api/v1/ai/models/%d", id), nil)
	return err
}

func (c *Client) ListAIModels(ctx context.Context, filter AIModelListFilter) ([]AIModelResponse, error) {
	params := url.Values{}
	params.Set("page", "1")
	params.Set("size", "10000")
	if filter.Search != "" {
		params.Set("search", filter.Search)
	}
	data, err := c.doRequest(ctx, http.MethodGet, "/api/v1/ai/models?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	var models []AIModelResponse
	if err := json.Unmarshal(data, &models); err != nil {
		return nil, fmt.Errorf("decoding ai models: %w", err)
	}
	return models, nil
}

// ---------------------------------------------------------------------------
// AI Prompt API models
// ---------------------------------------------------------------------------

type AIPromptCreateInput struct {
	Name           string          `json:"name"`
	Type           string          `json:"type"`
	Content        string          `json:"content"`
	Description    *string         `json:"description,omitempty"`
	VariableSchema json.RawMessage `json:"variable_schema,omitempty"`
}

// AIPromptUpdateInput is a PATCH. type is immutable and absent here (a user
// prompt cannot become system). Description omits omitempty so clearing sends
// explicit null.
type AIPromptUpdateInput struct {
	Name           *string         `json:"name,omitempty"`
	Content        *string         `json:"content,omitempty"`
	Description    *string         `json:"description"`
	VariableSchema json.RawMessage `json:"variable_schema,omitempty"`
}

type AIPromptResponse struct {
	ID             int64           `json:"id"`
	Name           string          `json:"name"`
	Type           string          `json:"type"`
	Content        string          `json:"content"`
	Description    *string         `json:"description"`
	VariableSchema json.RawMessage `json:"variable_schema"`
	CreatedAt      string          `json:"created_at"`
	UpdatedAt      string          `json:"updated_at"`
}

type AIPromptListFilter struct {
	Search string
}

// CreateAIPrompt creates a prompt. Creating with type other than "user" is
// rejected; type="system" prompts are builtin and return 403 on create.
func (c *Client) CreateAIPrompt(ctx context.Context, input AIPromptCreateInput) (*AIPromptResponse, error) {
	data, err := c.doRequest(ctx, http.MethodPost, "/api/v1/ai/prompts", input)
	if err != nil {
		return nil, err
	}
	var prompt AIPromptResponse
	if err := json.Unmarshal(data, &prompt); err != nil {
		return nil, fmt.Errorf("decoding ai prompt: %w", err)
	}
	return &prompt, nil
}

func (c *Client) GetAIPrompt(ctx context.Context, id int64) (*AIPromptResponse, error) {
	data, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/api/v1/ai/prompts/%d", id), nil)
	if err != nil {
		return nil, err
	}
	var prompt AIPromptResponse
	if err := json.Unmarshal(data, &prompt); err != nil {
		return nil, fmt.Errorf("decoding ai prompt: %w", err)
	}
	return &prompt, nil
}

// UpdateAIPrompt patches a prompt. type="system" prompts are builtin and return
// 403 on PATCH.
func (c *Client) UpdateAIPrompt(ctx context.Context, id int64, input AIPromptUpdateInput) (*AIPromptResponse, error) {
	data, err := c.doRequest(ctx, http.MethodPatch, fmt.Sprintf("/api/v1/ai/prompts/%d", id), input)
	if err != nil {
		return nil, err
	}
	var prompt AIPromptResponse
	if err := json.Unmarshal(data, &prompt); err != nil {
		return nil, fmt.Errorf("decoding ai prompt: %w", err)
	}
	return &prompt, nil
}

// DeleteAIPrompt removes a prompt. type="system" prompts are builtin and return
// 403 on DELETE; returns 403 if referenced by an agent.
func (c *Client) DeleteAIPrompt(ctx context.Context, id int64) error {
	_, err := c.doRequest(ctx, http.MethodDelete, fmt.Sprintf("/api/v1/ai/prompts/%d", id), nil)
	return err
}

func (c *Client) ListAIPrompts(ctx context.Context, filter AIPromptListFilter) ([]AIPromptResponse, error) {
	params := url.Values{}
	params.Set("page", "1")
	params.Set("size", "10000")
	if filter.Search != "" {
		params.Set("search", filter.Search)
	}
	data, err := c.doRequest(ctx, http.MethodGet, "/api/v1/ai/prompts?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	var prompts []AIPromptResponse
	if err := json.Unmarshal(data, &prompts); err != nil {
		return nil, fmt.Errorf("decoding ai prompts: %w", err)
	}
	return prompts, nil
}

// ---------------------------------------------------------------------------
// AI Agent API models
// ---------------------------------------------------------------------------

type AIAgentCreateInput struct {
	Name           string          `json:"name"`
	Type           string          `json:"type,omitempty"` // default "chat"
	Description    *string         `json:"description,omitempty"`
	ModelID        *int64          `json:"model_id,omitempty"`
	SystemPromptID int64           `json:"system_prompt_id"`
	Config         json.RawMessage `json:"config,omitempty"`
	ToolIDs        []int64         `json:"tool_ids,omitempty"`
}

// AIAgentUpdateInput is a PATCH. type is immutable and absent here; tool changes
// go through SetAIAgentTools. Description and ModelID omit omitempty so clearing
// sends explicit null.
type AIAgentUpdateInput struct {
	Name           *string         `json:"name,omitempty"`
	Description    *string         `json:"description"`
	ModelID        *int64          `json:"model_id"`
	SystemPromptID *int64          `json:"system_prompt_id,omitempty"`
	Config         json.RawMessage `json:"config,omitempty"`
}

// AIAgentResponse: empty ToolIDs means "all tools allowed"; non-empty restricts.
type AIAgentResponse struct {
	ID             int64           `json:"id"`
	Name           string          `json:"name"`
	Type           string          `json:"type"`
	Description    *string         `json:"description"`
	ModelID        *int64          `json:"model_id"`
	SystemPromptID *int64          `json:"system_prompt_id"`
	Config         json.RawMessage `json:"config"`
	IsBuiltin      bool            `json:"is_builtin"`
	IsFunctional   bool            `json:"is_functional"`
	ToolIDs        []int64         `json:"tool_ids"`
	CreatedAt      string          `json:"created_at"`
	UpdatedAt      string          `json:"updated_at"`
}

// AIAgentToolsInput is the dedicated PUT body for replacing an agent's tool set.
type AIAgentToolsInput struct {
	ToolIDs []int64 `json:"tool_ids"`
}

type AIAgentListFilter struct {
	Search string
}

// CreateAIAgent creates an agent. Agents with type starting "builtin_" cannot be
// created (403).
func (c *Client) CreateAIAgent(ctx context.Context, input AIAgentCreateInput) (*AIAgentResponse, error) {
	data, err := c.doRequest(ctx, http.MethodPost, "/api/v1/ai/agents", input)
	if err != nil {
		return nil, err
	}
	var agent AIAgentResponse
	if err := json.Unmarshal(data, &agent); err != nil {
		return nil, fmt.Errorf("decoding ai agent: %w", err)
	}
	return &agent, nil
}

func (c *Client) GetAIAgent(ctx context.Context, id int64) (*AIAgentResponse, error) {
	data, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/api/v1/ai/agents/%d", id), nil)
	if err != nil {
		return nil, err
	}
	var agent AIAgentResponse
	if err := json.Unmarshal(data, &agent); err != nil {
		return nil, fmt.Errorf("decoding ai agent: %w", err)
	}
	return &agent, nil
}

// UpdateAIAgent patches an agent. Builtins (type starting "builtin_") CAN be
// patched, but their config is MERGED, not replaced.
func (c *Client) UpdateAIAgent(ctx context.Context, id int64, input AIAgentUpdateInput) (*AIAgentResponse, error) {
	data, err := c.doRequest(ctx, http.MethodPatch, fmt.Sprintf("/api/v1/ai/agents/%d", id), input)
	if err != nil {
		return nil, err
	}
	var agent AIAgentResponse
	if err := json.Unmarshal(data, &agent); err != nil {
		return nil, fmt.Errorf("decoding ai agent: %w", err)
	}
	return &agent, nil
}

// DeleteAIAgent removes an agent. Agents with type starting "builtin_" return
// 403; returns 409 if the agent has chat sessions.
func (c *Client) DeleteAIAgent(ctx context.Context, id int64) error {
	_, err := c.doRequest(ctx, http.MethodDelete, fmt.Sprintf("/api/v1/ai/agents/%d", id), nil)
	return err
}

func (c *Client) ListAIAgents(ctx context.Context, filter AIAgentListFilter) ([]AIAgentResponse, error) {
	params := url.Values{}
	params.Set("page", "1")
	params.Set("size", "10000")
	if filter.Search != "" {
		params.Set("search", filter.Search)
	}
	data, err := c.doRequest(ctx, http.MethodGet, "/api/v1/ai/agents?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	var agents []AIAgentResponse
	if err := json.Unmarshal(data, &agents); err != nil {
		return nil, fmt.Errorf("decoding ai agents: %w", err)
	}
	return agents, nil
}

// SetAIAgentTools replaces the agent's tool set via the dedicated PUT. An empty
// slice means "all tools allowed"; non-empty restricts. If the PUT response body
// is the agent it is decoded directly; otherwise it falls back to GetAIAgent.
func (c *Client) SetAIAgentTools(ctx context.Context, id int64, toolIDs []int64) (*AIAgentResponse, error) {
	if toolIDs == nil {
		toolIDs = []int64{}
	}
	data, err := c.doRequest(ctx, http.MethodPut, fmt.Sprintf("/api/v1/ai/agents/%d/tools", id), AIAgentToolsInput{ToolIDs: toolIDs})
	if err != nil {
		return nil, err
	}
	var agent AIAgentResponse
	if err := json.Unmarshal(data, &agent); err != nil || agent.ID == 0 {
		return c.GetAIAgent(ctx, id)
	}
	return &agent, nil
}

// ---------------------------------------------------------------------------
// AI Skill API models
// ---------------------------------------------------------------------------

type AISkillCreateInput struct {
	Name          string   `json:"name"`
	Description   *string  `json:"description,omitempty"`
	Body          string   `json:"body"`
	AgentTypes    []string `json:"agent_types,omitempty"`
	ResourceTypes []string `json:"resource_types,omitempty"`
	IsEnabled     *bool    `json:"is_enabled,omitempty"`
}

// AISkillUpdateInput is a PATCH. Description omits omitempty so clearing sends
// explicit null; AgentTypes/ResourceTypes omit omitempty so an empty list
// replaces the existing set.
type AISkillUpdateInput struct {
	Name          *string  `json:"name,omitempty"`
	Description   *string  `json:"description"`
	Body          *string  `json:"body,omitempty"`
	AgentTypes    []string `json:"agent_types"`
	ResourceTypes []string `json:"resource_types"`
	IsEnabled     *bool    `json:"is_enabled,omitempty"`
}

type AISkillResponse struct {
	ID            int64    `json:"id"`
	Name          string   `json:"name"`
	Description   *string  `json:"description"`
	Body          string   `json:"body"`
	AgentTypes    []string `json:"agent_types"`
	ResourceTypes []string `json:"resource_types"`
	IsEnabled     bool     `json:"is_enabled"`
	IsBuiltin     bool     `json:"is_builtin"`
	CreatedAt     string   `json:"created_at"`
	UpdatedAt     string   `json:"updated_at"`
}

type AISkillListFilter struct {
	Search string
}

// CreateAISkill creates a skill. Builtin skills (is_builtin=true) cannot be
// created (403).
func (c *Client) CreateAISkill(ctx context.Context, input AISkillCreateInput) (*AISkillResponse, error) {
	data, err := c.doRequest(ctx, http.MethodPost, "/api/v1/ai/skills", input)
	if err != nil {
		return nil, err
	}
	var skill AISkillResponse
	if err := json.Unmarshal(data, &skill); err != nil {
		return nil, fmt.Errorf("decoding ai skill: %w", err)
	}
	return &skill, nil
}

func (c *Client) GetAISkill(ctx context.Context, id int64) (*AISkillResponse, error) {
	data, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/api/v1/ai/skills/%d", id), nil)
	if err != nil {
		return nil, err
	}
	var skill AISkillResponse
	if err := json.Unmarshal(data, &skill); err != nil {
		return nil, fmt.Errorf("decoding ai skill: %w", err)
	}
	return &skill, nil
}

// UpdateAISkill patches a skill. On a builtin (is_builtin=true) only is_enabled
// is mutable; patching name/body/description/agent_types/resource_types returns
// 403.
func (c *Client) UpdateAISkill(ctx context.Context, id int64, input AISkillUpdateInput) (*AISkillResponse, error) {
	data, err := c.doRequest(ctx, http.MethodPatch, fmt.Sprintf("/api/v1/ai/skills/%d", id), input)
	if err != nil {
		return nil, err
	}
	var skill AISkillResponse
	if err := json.Unmarshal(data, &skill); err != nil {
		return nil, fmt.Errorf("decoding ai skill: %w", err)
	}
	return &skill, nil
}

// DeleteAISkill removes a skill. Builtin skills (is_builtin=true) cannot be
// deleted (403).
func (c *Client) DeleteAISkill(ctx context.Context, id int64) error {
	_, err := c.doRequest(ctx, http.MethodDelete, fmt.Sprintf("/api/v1/ai/skills/%d", id), nil)
	return err
}

func (c *Client) ListAISkills(ctx context.Context, filter AISkillListFilter) ([]AISkillResponse, error) {
	params := url.Values{}
	params.Set("page", "1")
	params.Set("size", "10000")
	if filter.Search != "" {
		params.Set("search", filter.Search)
	}
	data, err := c.doRequest(ctx, http.MethodGet, "/api/v1/ai/skills?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	var skills []AISkillResponse
	if err := json.Unmarshal(data, &skills); err != nil {
		return nil, fmt.Errorf("decoding ai skills: %w", err)
	}
	return skills, nil
}

// ---------------------------------------------------------------------------
// AI Tool API models (read-only registry)
// ---------------------------------------------------------------------------

// AIToolResponse: tools are backend-builtin and read-only; they are referenced
// by id from agents.
type AIToolResponse struct {
	ID                 int64           `json:"id"`
	Name               string          `json:"name"`
	Description        string          `json:"description"`
	Handler            string          `json:"handler"`
	InputSchema        json.RawMessage `json:"input_schema"`
	RequiredPermission *string         `json:"required_permission"`
	CreatedAt          string          `json:"created_at"`
}

type AIToolListFilter struct {
	Search string
}

func (c *Client) GetAITool(ctx context.Context, id int64) (*AIToolResponse, error) {
	data, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/api/v1/ai/tools/%d", id), nil)
	if err != nil {
		return nil, err
	}
	var tool AIToolResponse
	if err := json.Unmarshal(data, &tool); err != nil {
		return nil, fmt.Errorf("decoding ai tool: %w", err)
	}
	return &tool, nil
}

func (c *Client) ListAITools(ctx context.Context, filter AIToolListFilter) ([]AIToolResponse, error) {
	params := url.Values{}
	params.Set("page", "1")
	params.Set("size", "10000")
	if filter.Search != "" {
		params.Set("search", filter.Search)
	}
	data, err := c.doRequest(ctx, http.MethodGet, "/api/v1/ai/tools?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	var tools []AIToolResponse
	if err := json.Unmarshal(data, &tools); err != nil {
		return nil, fmt.Errorf("decoding ai tools: %w", err)
	}
	return tools, nil
}
