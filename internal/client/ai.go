package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

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
	params.Set("size", "10000")
	if filter.Search != "" {
		params.Set("search", filter.Search)
	}
	return paginateJSON[AIModelResponse](ctx, c, "/api/v1/ai/models", params, "ai models")
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
	params.Set("size", "10000")
	if filter.Search != "" {
		params.Set("search", filter.Search)
	}
	return paginateJSON[AIPromptResponse](ctx, c, "/api/v1/ai/prompts", params, "ai prompts")
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

// AIAgentUpdateInput is a PATCH supporting chat/background type changes; tool changes
// go through SetAIAgentTools. Description and ModelID omit omitempty so clearing
// sends explicit null.
type AIAgentUpdateInput struct {
	Type           *string         `json:"type,omitempty"`
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
	params.Set("size", "10000")
	if filter.Search != "" {
		params.Set("search", filter.Search)
	}
	return paginateJSON[AIAgentResponse](ctx, c, "/api/v1/ai/agents", params, "ai agents")
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
	params.Set("size", "10000")
	if filter.Search != "" {
		params.Set("search", filter.Search)
	}
	return paginateJSON[AISkillResponse](ctx, c, "/api/v1/ai/skills", params, "ai skills")
}

// ---------------------------------------------------------------------------
// AI Tool API models (read-only registry)
// ---------------------------------------------------------------------------

// AIToolResponse: tools are backend-builtin and read-only; they are referenced
// by id from agents.
type AIToolResponse struct {
	ID                     int64           `json:"id"`
	Name                   string          `json:"name"`
	Description            string          `json:"description"`
	Handler                string          `json:"handler"`
	InputSchema            json.RawMessage `json:"input_schema"`
	RequiredPermission     *string         `json:"required_permission"`
	RestrictedToAgentTypes []string        `json:"restricted_to_agent_types"`
	CreatedAt              string          `json:"created_at"`
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
	params.Set("size", "10000")
	if filter.Search != "" {
		params.Set("search", filter.Search)
	}
	return paginateJSON[AIToolResponse](ctx, c, "/api/v1/ai/tools", params, "ai tools")
}

// UpdateAIAgentSettings changes only the explicitly owned built-in settings.
// It deliberately omits identity and unrelated nullable fields.
func (c *Client) UpdateAIAgentSettings(ctx context.Context, id int64, input map[string]any) (*AIAgentResponse, error) {
	data, err := c.doRequest(ctx, http.MethodPatch, fmt.Sprintf("/api/v1/ai/agents/%d", id), input)
	if err != nil {
		return nil, err
	}
	var result AIAgentResponse
	if err = json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
func (c *Client) UpdateAISkillEnabled(ctx context.Context, id int64, enabled bool) (*AISkillResponse, error) {
	data, err := c.doRequest(ctx, http.MethodPatch, fmt.Sprintf("/api/v1/ai/skills/%d", id), map[string]bool{"is_enabled": enabled})
	if err != nil {
		return nil, err
	}
	var result AISkillResponse
	if err = json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

type AIGuardrailResponse struct {
	ResourceType     string `json:"resource_type"`
	DisplayName      string `json:"display_name"`
	Description      string `json:"ai_guardrails_description"`
	AllowlistEnabled bool   `json:"allowlist_enabled"`
}

func (c *Client) ListAIGuardrails(ctx context.Context) ([]AIGuardrailResponse, error) {
	data, err := c.doRequest(ctx, http.MethodGet, "/api/v1/ai/guardrails", nil)
	if err != nil {
		return nil, err
	}
	var result []AIGuardrailResponse
	if err = json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}
func (c *Client) SetAIGuardrail(ctx context.Context, resourceType string, enabled bool) (*AIGuardrailResponse, error) {
	data, err := c.doRequest(ctx, http.MethodPut, "/api/v1/ai/guardrails/"+url.PathEscape(resourceType), map[string]bool{"allowlist_enabled": enabled})
	if err != nil {
		return nil, err
	}
	var result AIGuardrailResponse
	if err = json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
func (c *Client) ResetAIGuardrail(ctx context.Context, resourceType string) error {
	_, err := c.doRequest(ctx, http.MethodDelete, "/api/v1/ai/guardrails/"+url.PathEscape(resourceType), nil)
	return err
}
func (c *Client) GetAIPromptVariables(ctx context.Context) (json.RawMessage, error) {
	return c.doRequest(ctx, http.MethodGet, "/api/v1/ai/prompt-variables", nil)
}
