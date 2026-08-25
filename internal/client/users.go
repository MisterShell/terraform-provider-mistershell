package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

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
	IsActive bool   `json:"is_active"`
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
	data, err := c.doRequest(ctx, http.MethodPost, "/api/v1/users", input)
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
	params.Set("size", "10000")
	if filter.Search != "" {
		params.Set("search", filter.Search)
	}
	return paginateJSON[UserDetailResponse](ctx, c, "/api/v1/users", params, "users")
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
	_, err := c.doRequest(ctx, http.MethodPut, fmt.Sprintf("/api/v1/users/%d/roles/%d", userID, roleID), nil)
	return err
}

// RemoveUserRole unassigns a single role from a user. Idempotent server-side.
func (c *Client) RemoveUserRole(ctx context.Context, userID, roleID int64) error {
	_, err := c.doRequest(ctx, http.MethodDelete, fmt.Sprintf("/api/v1/users/%d/roles/%d", userID, roleID), nil)
	return err
}
