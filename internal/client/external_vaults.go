package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// ExternalVaultCreateInput config contains connection settings, never resolved credentials.
type ExternalVaultCreateInput struct {
	Name            string          `json:"name"`
	Mnemonic        string          `json:"mnemonic"`
	Description     string          `json:"description"`
	VaultType       string          `json:"vault_type"`
	IsEnabled       *bool           `json:"is_enabled,omitempty"`
	Config          json.RawMessage `json:"config"`
	CacheTTLSeconds *int64          `json:"cache_ttl_seconds,omitempty"`
}

// ExternalVaultUpdateInput omits untouched fields. Config is an overlay on the server.
type ExternalVaultUpdateInput struct {
	Name            *string         `json:"name,omitempty"`
	Description     *string         `json:"description,omitempty"`
	IsEnabled       *bool           `json:"is_enabled,omitempty"`
	Config          json.RawMessage `json:"config,omitempty"`
	CacheTTLSeconds *int64          `json:"cache_ttl_seconds,omitempty"`
}

type ExternalVaultResponse struct {
	ID              int64           `json:"id"`
	Name            string          `json:"name"`
	Mnemonic        string          `json:"mnemonic"`
	Description     string          `json:"description"`
	VaultType       string          `json:"vault_type"`
	IsEnabled       bool            `json:"is_enabled"`
	Config          json.RawMessage `json:"config"`
	CacheTTLSeconds int64           `json:"cache_ttl_seconds"`
	LastCheckAt     *string         `json:"last_check_at"`
	LastCheckStatus *string         `json:"last_check_status"`
	LastCheckError  *string         `json:"last_check_error"`
	CredentialCount int64           `json:"credential_count"`
	CreatedAt       string          `json:"created_at"`
	UpdatedAt       string          `json:"updated_at"`
}

type ExternalVaultType struct {
	VaultType               string          `json:"vault_type"`
	DisplayName             string          `json:"display_name"`
	ConfigSchema            json.RawMessage `json:"config_schema"`
	BaseRefSchema           json.RawMessage `json:"base_ref_schema"`
	SecretConfigKeys        []string        `json:"secret_config_keys"`
	SupportsSubjectIdentity bool            `json:"supports_subject_identity"`
	FieldRefHint            string          `json:"field_ref_hint"`
}

type ExternalVaultListFilter struct{ Search string }

func (c *Client) externalVaultRequest(ctx context.Context, method, path string, input any) (*ExternalVaultResponse, error) {
	raw, err := c.doRequest(ctx, method, path, input)
	if err != nil {
		return nil, err
	}
	var result ExternalVaultResponse
	if err = json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("decoding external vault: %w", err)
	}
	return &result, nil
}
func (c *Client) CreateExternalVault(ctx context.Context, input ExternalVaultCreateInput) (*ExternalVaultResponse, error) {
	return c.externalVaultRequest(ctx, http.MethodPost, "/api/v1/external-vaults", input)
}
func (c *Client) GetExternalVault(ctx context.Context, id int64) (*ExternalVaultResponse, error) {
	return c.externalVaultRequest(ctx, http.MethodGet, fmt.Sprintf("/api/v1/external-vaults/%d", id), nil)
}
func (c *Client) UpdateExternalVault(ctx context.Context, id int64, input ExternalVaultUpdateInput) (*ExternalVaultResponse, error) {
	return c.externalVaultRequest(ctx, http.MethodPatch, fmt.Sprintf("/api/v1/external-vaults/%d", id), input)
}
func (c *Client) DeleteExternalVault(ctx context.Context, id int64) error {
	_, err := c.doRequest(ctx, http.MethodDelete, fmt.Sprintf("/api/v1/external-vaults/%d", id), nil)
	return err
}
func (c *Client) ListExternalVaults(ctx context.Context, filter ExternalVaultListFilter) ([]ExternalVaultResponse, error) {
	params := url.Values{"size": {"10000"}}
	if filter.Search != "" {
		params.Set("search", filter.Search)
	}
	return paginateJSON[ExternalVaultResponse](ctx, c, "/api/v1/external-vaults", params, "external vaults")
}
func (c *Client) ListExternalVaultTypes(ctx context.Context) ([]ExternalVaultType, error) {
	raw, err := c.doRequest(ctx, http.MethodGet, "/api/v1/external-vaults/types", nil)
	if err != nil {
		return nil, err
	}
	var result []ExternalVaultType
	if err = json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("decoding external vault types: %w", err)
	}
	return result, nil
}

// FindExternalVault filters every search page by exact name/mnemonic, then gets detail.
func (c *Client) FindExternalVault(ctx context.Context, name, mnemonic string) (*ExternalVaultResponse, error) {
	search := name
	if search == "" {
		search = mnemonic
	}
	if search == "" {
		return nil, fmt.Errorf("specify an external vault name or mnemonic")
	}
	items, err := c.ListExternalVaults(ctx, ExternalVaultListFilter{Search: SearchPrefix(search)})
	if err != nil {
		return nil, err
	}
	var matches []ExternalVaultResponse
	for _, v := range items {
		if (name == "" || v.Name == name) && (mnemonic == "" || v.Mnemonic == mnemonic) {
			matches = append(matches, v)
		}
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("no external vault matches the exact lookup criteria")
	}
	if len(matches) > 1 {
		return nil, fmt.Errorf("multiple external vaults match the exact lookup criteria; use an ID")
	}
	return c.GetExternalVault(ctx, matches[0].ID)
}
