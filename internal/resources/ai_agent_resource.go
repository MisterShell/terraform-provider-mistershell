package resources

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-mistershell/internal/client"
)

var (
	_ resource.Resource                = &AIAgentResource{}
	_ resource.ResourceWithImportState = &AIAgentResource{}
)

type AIAgentResource struct {
	client *client.Client
}

type AIAgentResourceModel struct {
	ID             types.Int64          `tfsdk:"id"`
	Name           types.String         `tfsdk:"name"`
	Type           types.String         `tfsdk:"type"`
	Description    types.String         `tfsdk:"description"`
	ModelID        types.Int64          `tfsdk:"model_id"`
	SystemPromptID types.Int64          `tfsdk:"system_prompt_id"`
	Config         jsontypes.Normalized `tfsdk:"config"`
	ToolIDs        types.Set            `tfsdk:"tool_ids"`
	IsBuiltin      types.Bool           `tfsdk:"is_builtin"`
	IsFunctional   types.Bool           `tfsdk:"is_functional"`
	CreatedAt      types.String         `tfsdk:"created_at"`
	UpdatedAt      types.String         `tfsdk:"updated_at"`
}

func NewAIAgentResource() resource.Resource {
	return &AIAgentResource{}
}

func (r *AIAgentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ai_agent"
}

func (r *AIAgentResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages user-defined AI agents (type chat or background) referencing a model and a system prompt; builtin agents (type builtin_*) are managed by MisterShell and exposed read-only via the data source.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Description: "AI agent ID.",
				Computed:    true,
				PlanModifiers: []planmodifier.Int64{
					int64UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Agent name (1-255 characters).",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, 255),
				},
			},
			"type": schema.StringAttribute{
				Description: "User agent type. One of: chat, background. Defaults to chat. Cannot be changed after creation. builtin_* agents are managed by MisterShell and read-only (use the data source).",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("chat"),
				Validators: []validator.String{
					stringvalidator.OneOf("chat", "background"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"description": schema.StringAttribute{
				Description: "Agent description.",
				Optional:    true,
			},
			"model_id": schema.Int64Attribute{
				Description: "FK to a mistershell_ai_model. If omitted the default model is used.",
				Optional:    true,
			},
			"system_prompt_id": schema.Int64Attribute{
				Description: "FK to a mistershell_ai_prompt providing the agent's system prompt.",
				Required:    true,
			},
			"config": schema.StringAttribute{
				Description: "Agent config as JSON (token_budget, temperature, label, icon, etc.). Use jsonencode() in HCL. The server may enrich/reorder this blob, so the planned value is stored from config.",
				Optional:    true,
				CustomType:  jsontypes.NormalizedType{},
			},
			"tool_ids": schema.SetAttribute{
				Description: "Tool IDs the agent may use; empty or unset means ALL tools are allowed. Look up ids with the mistershell_ai_tool data source.",
				Optional:    true,
				Computed:    true,
				ElementType: types.Int64Type,
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.UseStateForUnknown(),
				},
			},
			"is_builtin": schema.BoolAttribute{
				Description: "Whether the agent is a builtin agent managed by MisterShell.",
				Computed:    true,
			},
			"is_functional": schema.BoolAttribute{
				Description: "Whether the agent is currently functional (model and prompt resolve).",
				Computed:    true,
			},
			"created_at": schema.StringAttribute{
				Description: "Creation timestamp.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"updated_at": schema.StringAttribute{
				Description: "Last update timestamp.",
				Computed:    true,
			},
		},
	}
}

func (r *AIAgentResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Resource Configure Type", "Expected *client.Client")
		return
	}
	r.client = c
}

func (r *AIAgentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan AIAgentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	input := client.AIAgentCreateInput{
		Name:           plan.Name.ValueString(),
		Type:           plan.Type.ValueString(),
		SystemPromptID: plan.SystemPromptID.ValueInt64(),
		Config:         normalizedToRawJSON(plan.Config),
		ToolIDs:        int64SetToSlice(plan.ToolIDs),
	}
	input.Description = stringValueToPtr(plan.Description)
	input.ModelID = int64ValueToPtr(plan.ModelID)

	agent, err := r.client.CreateAIAgent(ctx, input)
	if err != nil {
		resp.Diagnostics.AddError("Error creating AI agent", err.Error())
		return
	}

	// Preserve config from plan (stored-from-config): the server may enrich/reorder
	// the blob, so reflecting the server value would break apply-consistency.
	savedConfig := plan.Config
	mapAIAgentResponseToModel(agent, &plan)
	plan.Config = savedConfig
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *AIAgentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state AIAgentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	agent, err := r.client.GetAIAgent(ctx, state.ID.ValueInt64())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading AI agent", err.Error())
		return
	}

	// Preserve config from state (stored-from-config): server enriches/reorders the
	// blob and for builtin agents config is merged, so reflecting it would drift.
	savedConfig := state.Config
	mapAIAgentResponseToModel(agent, &state)
	state.Config = savedConfig
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *AIAgentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan AIAgentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state AIAgentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := state.ID.ValueInt64()

	input := client.AIAgentUpdateInput{
		Config: normalizedToRawJSON(plan.Config),
	}
	if !plan.Name.Equal(state.Name) {
		v := plan.Name.ValueString()
		input.Name = &v
	}
	// Description and ModelID omit omitempty in the client struct so a cleared
	// value sends explicit null to the PATCH endpoint.
	input.Description = stringValueToPtr(plan.Description)
	input.ModelID = int64ValueToPtr(plan.ModelID)
	input.SystemPromptID = int64ValueToPtr(plan.SystemPromptID)

	agent, err := r.client.UpdateAIAgent(ctx, id, input)
	if err != nil {
		resp.Diagnostics.AddError("Error updating AI agent", err.Error())
		return
	}

	// Tools are managed via the dedicated endpoint, not the PATCH body. Reconcile
	// only when the planned tool set is known and differs from current state.
	if !plan.ToolIDs.IsUnknown() && !plan.ToolIDs.Equal(state.ToolIDs) {
		toolIDs := int64SetToSlice(plan.ToolIDs)
		if toolIDs == nil {
			toolIDs = []int64{}
		}
		updated, err := r.client.SetAIAgentTools(ctx, id, toolIDs)
		if err != nil {
			resp.Diagnostics.AddError("Error setting AI agent tools", err.Error())
			return
		}
		agent = updated
	}

	// Preserve config from plan (stored-from-config).
	savedConfig := plan.Config
	mapAIAgentResponseToModel(agent, &plan)
	plan.Config = savedConfig
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *AIAgentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state AIAgentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// The API returns 409 if the agent still has chat sessions; surface the error.
	if err := r.client.DeleteAIAgent(ctx, state.ID.ValueInt64()); err != nil {
		resp.Diagnostics.AddError("Error deleting AI agent", err.Error())
	}
}

func (r *AIAgentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("Expected integer ID, got: %s", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), types.Int64Value(id))...)
}

// mapAIAgentResponseToModel copies the API response into the model. It sets every
// field EXCEPT config, which the caller preserves from plan/state per the
// stored-from-config rule.
func mapAIAgentResponseToModel(agent *client.AIAgentResponse, m *AIAgentResourceModel) {
	m.ID = types.Int64Value(agent.ID)
	m.Name = types.StringValue(agent.Name)
	m.Type = types.StringValue(agent.Type)
	m.Description = stringPtrToValue(agent.Description)
	m.ModelID = int64PtrToValue(agent.ModelID)
	m.SystemPromptID = int64PtrToValue(agent.SystemPromptID)
	m.ToolIDs = int64SliceToSet(agent.ToolIDs)
	m.IsBuiltin = types.BoolValue(agent.IsBuiltin)
	m.IsFunctional = types.BoolValue(agent.IsFunctional)
	m.CreatedAt = types.StringValue(agent.CreatedAt)
	m.UpdatedAt = types.StringValue(agent.UpdatedAt)
}
