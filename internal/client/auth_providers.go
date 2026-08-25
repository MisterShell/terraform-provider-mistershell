package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// ---------------------------------------------------------------------------
// Auth-Provider API models
// ---------------------------------------------------------------------------

// AuthProviderCreateInput: config is an opaque per-provider_type blob; secrets
// are masked **** in the GET-single response and ABSENT from create/patch/list
// responses.
type AuthProviderCreateInput struct {
	Name         string          `json:"name"` // 1..100
	Mnemonic     string          `json:"mnemonic"`
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
	Mnemonic     string `json:"mnemonic"`
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
	// SP URLs are derived server-side from the app_base_url setting plus the
	// immutable mnemonic, and are returned for SAML providers only (nil for LDAP/OIDC).
	// They are read-only: the former config.sp_entity_id / config.sp_acs_url
	// inputs were removed backend-side, and SAMLConfig now rejects them.
	SPEntityID      *string `json:"sp_entity_id"`
	SPACSURL        *string `json:"sp_acs_url"`
	OIDCRedirectURI *string `json:"oidc_redirect_uri"`
}

type AuthProviderListFilter struct{ Search string }

func (c *Client) CreateAuthProvider(ctx context.Context, input AuthProviderCreateInput) (*AuthProviderResponse, error) {
	data, err := c.doRequest(ctx, http.MethodPost, "/api/v1/auth-providers", input)
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
	params.Set("size", "10000")
	if filter.Search != "" {
		params.Set("search", filter.Search)
	}
	return paginateJSON[AuthProviderResponse](ctx, c, "/api/v1/auth-providers", params, "auth providers")
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

func (c *Client) CreateGroupMapping(ctx context.Context, providerID int64, input GroupMappingCreateInput) (*GroupMappingResponse, error) {
	data, err := c.doRequest(ctx, http.MethodPost, fmt.Sprintf("/api/v1/auth-providers/%d/mappings", providerID), input)
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
	data, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/api/v1/auth-providers/%d/mappings", providerID), nil)
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
