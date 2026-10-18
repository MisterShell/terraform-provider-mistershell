package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// ---------------------------------------------------------------------------
// Credential API models
// ---------------------------------------------------------------------------

// CredentialTypeInfo is one installed RDK credential type returned by MisterShell.
type CredentialTypeInfo struct {
	Type        string          `json:"type"`
	DisplayName string          `json:"display_name"`
	Description string          `json:"description"`
	Schema      json.RawMessage `json:"schema"`
}

func (c *Client) ListCredentialTypes(ctx context.Context) ([]CredentialTypeInfo, error) {
	data, err := c.doRequest(ctx, http.MethodGet, "/api/v1/credentials/types", nil)
	if err != nil {
		return nil, err
	}
	var credentialTypes []CredentialTypeInfo
	if err := json.Unmarshal(data, &credentialTypes); err != nil {
		return nil, fmt.Errorf("decoding credential types: %w", err)
	}
	return credentialTypes, nil
}

type CredentialCreateInput struct {
	Source               string          `json:"source,omitempty"`
	ExternalVaultID      *int64          `json:"external_vault_id,omitempty"`
	ExternalVaultRef     json.RawMessage `json:"external_vault_ref,omitempty"`
	UserSource           string          `json:"user_source,omitempty"`
	UserExternalVaultID  *int64          `json:"user_external_vault_id,omitempty"`
	UserExternalVaultRef json.RawMessage `json:"user_external_vault_ref,omitempty"`
	UserCredentialData   json.RawMessage `json:"user_credential_data,omitempty"`

	Name                string          `json:"name"`
	CredentialType      string          `json:"credential_type"`
	CredentialData      json.RawMessage `json:"credential_data"`
	Description         *string         `json:"description,omitempty"`
	RequiresUserMapping *bool           `json:"requires_user_mapping,omitempty"`
	ExtraData           json.RawMessage `json:"extra_data,omitempty"`
}

type CredentialUpdateInput struct {
	Source               string          `json:"source,omitempty"`
	ExternalVaultID      json.RawMessage `json:"external_vault_id,omitempty"`
	ExternalVaultRef     json.RawMessage `json:"external_vault_ref,omitempty"`
	UserSource           string          `json:"user_source,omitempty"`
	UserExternalVaultID  json.RawMessage `json:"user_external_vault_id,omitempty"`
	UserExternalVaultRef json.RawMessage `json:"user_external_vault_ref,omitempty"`
	UserCredentialData   json.RawMessage `json:"user_credential_data,omitempty"`

	Name                *string         `json:"name,omitempty"`
	Description         json.RawMessage `json:"description,omitempty"`
	CredentialType      *string         `json:"credential_type,omitempty"`
	CredentialData      json.RawMessage `json:"credential_data,omitempty"`
	RequiresUserMapping *bool           `json:"requires_user_mapping,omitempty"`
	ExtraData           json.RawMessage `json:"extra_data,omitempty"`
}

type CredentialResponse struct {
	Source                string          `json:"source,omitempty"`
	ExternalVaultID       *int64          `json:"external_vault_id,omitempty"`
	ExternalVaultRef      json.RawMessage `json:"external_vault_ref,omitempty"`
	ExternalVaultName     *string         `json:"external_vault_name"`
	ExternalVaultType     *string         `json:"external_vault_type"`
	UserSource            string          `json:"user_source,omitempty"`
	UserExternalVaultID   *int64          `json:"user_external_vault_id,omitempty"`
	UserExternalVaultRef  json.RawMessage `json:"user_external_vault_ref,omitempty"`
	UserExternalVaultName *string         `json:"user_external_vault_name"`
	UserExternalVaultType *string         `json:"user_external_vault_type"`
	UserCredentialData    json.RawMessage `json:"user_credential_data,omitempty"`

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
	data, err := c.doRequest(ctx, http.MethodPost, "/api/v1/credentials", input)
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
	params.Set("size", "10000")
	if filter.Search != "" {
		params.Set("search", filter.Search)
	}
	if filter.CredentialType != "" {
		params.Set("credential_type", filter.CredentialType)
	}
	return paginateJSON[CredentialResponse](ctx, c, "/api/v1/credentials", params, "credentials")
}
