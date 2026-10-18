package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// ---------------------------------------------------------------------------
// Role API models
// ---------------------------------------------------------------------------

type RoleCreateInput struct {
	Permissions       []string `json:"permissions,omitempty"`
	Name              string   `json:"name"`
	Description       *string  `json:"description,omitempty"`
	LocationScopeMode string   `json:"location_scope_mode"`
	ScopeLocationIDs  []int64  `json:"scope_location_ids,omitempty"`
}

type RoleUpdateInput struct {
	Permissions       *[]string `json:"permissions,omitempty"`
	Name              *string   `json:"name,omitempty"`
	Description       *string   `json:"description"`
	LocationScopeMode *string   `json:"location_scope_mode,omitempty"`
	ScopeLocationIDs  *[]int64  `json:"scope_location_ids,omitempty"`
}

type RoleResponse struct { // POST/PATCH
	IsBuiltin         bool    `json:"is_builtin"`
	ID                int64   `json:"id"`
	Name              string  `json:"name"`
	Description       *string `json:"description"`
	LocationScopeMode string  `json:"location_scope_mode"`
	ScopeLocationIDs  []int64 `json:"scope_location_ids"`
	CreatedAt         string  `json:"created_at"`
	UpdatedAt         string  `json:"updated_at"`
}

type RoleDetailResponse struct { // GET + List
	RoleResponse
	Permissions []string `json:"permissions"`
}

type RoleListFilter struct {
	Search string
}

func (c *Client) CreateRole(ctx context.Context, input RoleCreateInput) (*RoleResponse, error) {
	data, err := c.doRequest(ctx, http.MethodPost, "/api/v1/roles", input)
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
	params.Set("size", "10000")
	if filter.Search != "" {
		params.Set("search", filter.Search)
	}
	return paginateJSON[RoleDetailResponse](ctx, c, "/api/v1/roles", params, "roles")
}

// AddRolePermission assigns a single permission name to a role. The name is a
// path param containing dots and '*', so it is URL-path-escaped.
func (c *Client) AddRolePermission(ctx context.Context, id int64, name string) error {
	_, err := c.doRequest(ctx, http.MethodPut, fmt.Sprintf("/api/v1/roles/%d/permissions/%s", id, url.PathEscape(name)), nil)
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
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Module      string         `json:"module"`
	Action      string         `json:"action"`
	Roles       []RoleResponse `json:"roles"`
}

type PermissionListFilter struct {
	Module string
	Search string
}

func (c *Client) ListPermissions(ctx context.Context, filter PermissionListFilter) ([]PermissionInfo, error) {
	params := url.Values{}
	params.Set("size", "10000")
	if filter.Module != "" {
		params.Set("module", filter.Module)
	}
	if filter.Search != "" {
		params.Set("search", filter.Search)
	}
	return paginateJSON[PermissionInfo](ctx, c, "/api/v1/permissions", params, "permissions")
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
