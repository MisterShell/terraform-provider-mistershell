package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

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

// AclUpdateInput is a PATCH body: the resource always sends name +
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
	data, err := c.doRequest(ctx, http.MethodPatch, fmt.Sprintf("/api/v1/session-policy/acls/%d", id), input)
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
	params.Set("size", "10000")
	if filter.Search != "" {
		params.Set("search", filter.Search)
	}
	return paginateJSON[AclResponse](ctx, c, "/api/v1/session-policy/acls", params, "acls")
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

// RuleApproval is the approver configuration of an action=approve rule
// (PolicyApprovalConfig). The backend requires it on approve rules and rejects
// it on accept/deny rules.
type RuleApproval struct {
	ApproverRoleIDs   []int64 `json:"approver_role_ids"`  // 1-100 unique role IDs
	ExpirationSeconds int64   `json:"expiration_seconds"` // pending-request lifetime, 60..2592000
	ValiditySeconds   int64   `json:"validity_seconds"`   // grant lifetime after approval, 60..2592000
	NotifyEmail       bool    `json:"notify_email"`
	AllowSelfApproval bool    `json:"allow_self_approval"`
}

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
	Action        string   `json:"action,omitempty"` // accept|deny|approve, default accept
	Notify        bool     `json:"notify"`
	Log           bool     `json:"log"`
	Enabled       bool     `json:"enabled"`
	// Approval is required with action=approve and omitted otherwise, so
	// accept/deny creates stay valid on backends that predate approvals.
	Approval *RuleApproval `json:"approval,omitempty"`
}

// RuleUpdateInput is a full-replace PUT (RuleUpdate == RuleCreate). Comment uses
// *string with NO omitempty so clearing sends explicit null. Position is *int64
// (omit leaves it unchanged server-side: update only sets it if non-None).
// Approval also has NO omitempty: the backend merges the PATCH onto the stored
// rule, so switching an approve rule to accept/deny must send explicit null.
// Backends before 2.0.10 ignore the unknown field.
type RuleUpdateInput struct {
	Name          string        `json:"name"`
	Comment       *string       `json:"comment"` // no omitempty: explicit null clears
	Position      *int64        `json:"position,omitempty"`
	ResourceTypes []string      `json:"resource_types"`
	SessionTypes  []string      `json:"session_types"`
	LocationIDs   []int64       `json:"location_ids"`
	TagIDs        []int64       `json:"tag_ids"`
	RoleIDs       []int64       `json:"role_ids"`
	CommandAclIDs []int64       `json:"command_acl_ids"`
	Action        string        `json:"action"`
	Notify        bool          `json:"notify"`
	Log           bool          `json:"log"`
	Enabled       bool          `json:"enabled"`
	Approval      *RuleApproval `json:"approval"` // no omitempty: explicit null clears
}

type RuleResponse struct {
	ID            int64         `json:"id"`
	Position      int64         `json:"position"`
	Name          string        `json:"name"`
	Comment       *string       `json:"comment"`
	ResourceTypes []string      `json:"resource_types"`
	SessionTypes  []string      `json:"session_types"`
	LocationIDs   []int64       `json:"location_ids"`
	TagIDs        []int64       `json:"tag_ids"`
	RoleIDs       []int64       `json:"role_ids"`
	CommandAclIDs []int64       `json:"command_acl_ids"`
	Action        string        `json:"action"`
	Notify        bool          `json:"notify"`
	Log           bool          `json:"log"`
	Enabled       bool          `json:"enabled"`
	Approval      *RuleApproval `json:"approval"` // null unless action=approve (absent before 2.0.10)
	HitCount      int64         `json:"hit_count"`
	LastHitAt     *string       `json:"last_hit_at"`
	CreatedAt     string        `json:"created_at"`
	UpdatedAt     string        `json:"updated_at"`
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
	data, err := c.doRequest(ctx, http.MethodPatch, fmt.Sprintf("/api/v1/session-policy/rules/%d", id), input)
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
