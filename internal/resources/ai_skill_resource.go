package resources

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-mistershell/internal/client"
)

var (
	_ resource.Resource                = &AISkillResource{}
	_ resource.ResourceWithImportState = &AISkillResource{}
)

type AISkillResource struct {
	client *client.Client
}

type AISkillResourceModel struct {
	ID            types.Int64  `tfsdk:"id"`
	Name          types.String `tfsdk:"name"`
	Description   types.String `tfsdk:"description"`
	Body          types.String `tfsdk:"body"`
	AgentTypes    types.Set    `tfsdk:"agent_types"`
	ResourceTypes types.Set    `tfsdk:"resource_types"`
	IsEnabled     types.Bool   `tfsdk:"is_enabled"`
	IsBuiltin     types.Bool   `tfsdk:"is_builtin"`
	CreatedAt     types.String `tfsdk:"created_at"`
	UpdatedAt     types.String `tfsdk:"updated_at"`
}

func NewAISkillResource() resource.Resource {
	return &AISkillResource{}
}

func (r *AISkillResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ai_skill"
}

func (r *AISkillResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a MisterShell custom AI skill: a markdown platform brief surfaced to agents via list_skills. Builtin skills (is_builtin=true) are managed by MisterShell — only their is_enabled flag is mutable server-side — and are exposed read-only via the mistershell_ai_skill data source. This resource manages user-defined skills only.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Description: "AI skill ID.",
				Computed:    true,
				PlanModifiers: []planmodifier.Int64{
					int64UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Skill name (1-255 characters).",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, 255),
				},
			},
			"description": schema.StringAttribute{
				Description: "Optional human-readable description of the skill.",
				Optional:    true,
			},
			"body": schema.StringAttribute{
				Description: "Markdown platform-brief content surfaced to agents. May contain Jinja2 template variables.",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"agent_types": schema.SetAttribute{
				Description: "Restrict skill discovery to these agent types; omit for no restriction. The backend converts an empty list to null, so OMIT this attribute rather than passing an empty list ([]) to avoid drift.",
				Optional:    true,
				ElementType: types.StringType,
				Validators: []validator.Set{
					setvalidator.ValueStringsAre(stringvalidator.OneOf(client.SupportedAIAgentTypes...)),
				},
			},
			"resource_types": schema.SetAttribute{
				Description: "Restrict skill discovery to these resource-type keys (a discovery filter only, not an auth gate); omit for no restriction. The backend validates these against its resource-type registry (not OneOf-validated here) and converts an empty list to null, so OMIT this attribute rather than passing an empty list ([]) to avoid drift.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"is_enabled": schema.BoolAttribute{
				Description: "Whether the skill is enabled. Defaults to true.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
			},
			"is_builtin": schema.BoolAttribute{
				Description: "Whether this is a builtin (MisterShell-managed) skill. Always false for skills created by this resource; builtin skills are read-only (only is_enabled is mutable server-side) and are exposed via the data source.",
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

func (r *AISkillResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *AISkillResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan AISkillResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	input := client.AISkillCreateInput{
		Name:          plan.Name.ValueString(),
		Description:   stringValueToPtr(plan.Description),
		Body:          plan.Body.ValueString(),
		AgentTypes:    stringSetToSlice(plan.AgentTypes),
		ResourceTypes: stringSetToSlice(plan.ResourceTypes),
		IsEnabled:     boolValueToPtr(plan.IsEnabled),
	}

	skill, err := r.client.CreateAISkill(ctx, input)
	if err != nil {
		resp.Diagnostics.AddError("Error creating AI skill", err.Error())
		return
	}

	mapAISkillResponseToModel(skill, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *AISkillResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state AISkillResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	skill, err := r.client.GetAISkill(ctx, state.ID.ValueInt64())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading AI skill", err.Error())
		return
	}

	mapAISkillResponseToModel(skill, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *AISkillResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan AISkillResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state AISkillResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := plan.Name.ValueString()
	body := plan.Body.ValueString()
	// AgentTypes/ResourceTypes UpdateInput fields have no omitempty, so an empty
	// (nil) slice clears the existing set server-side — matching an omitted attr.
	input := client.AISkillUpdateInput{
		Name:          &name,
		Description:   stringValueToPtr(plan.Description),
		Body:          &body,
		AgentTypes:    stringSetToSlice(plan.AgentTypes),
		ResourceTypes: stringSetToSlice(plan.ResourceTypes),
		IsEnabled:     boolValueToPtr(plan.IsEnabled),
	}

	skill, err := r.client.UpdateAISkill(ctx, state.ID.ValueInt64(), input)
	if err != nil {
		resp.Diagnostics.AddError("Error updating AI skill", err.Error())
		return
	}

	mapAISkillResponseToModel(skill, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *AISkillResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state AISkillResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Builtin skills (is_builtin=true) cannot be deleted; the API returns 403,
	// which is surfaced verbatim here.
	if err := r.client.DeleteAISkill(ctx, state.ID.ValueInt64()); err != nil {
		resp.Diagnostics.AddError("Error deleting AI skill", err.Error())
	}
}

func (r *AISkillResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("Expected integer ID, got: %s", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), types.Int64Value(id))...)
}

// stringSliceToSetOrNull converts []string to a types.Set of String, yielding a
// NULL set when the slice is empty/nil. The server returns null for empty
// agent_types/resource_types, so mapping [] -> null keeps an omitted attribute
// null and avoids perpetual drift.
func stringSliceToSetOrNull(vals []string) types.Set {
	if len(vals) == 0 {
		return types.SetNull(types.StringType)
	}
	return stringSliceToSet(vals)
}

// mapAISkillResponseToModel copies an API response into the tfsdk model.
func mapAISkillResponseToModel(skill *client.AISkillResponse, m *AISkillResourceModel) {
	m.ID = types.Int64Value(skill.ID)
	m.Name = types.StringValue(skill.Name)
	m.Description = stringPtrToValue(skill.Description)
	m.Body = types.StringValue(skill.Body)
	m.AgentTypes = stringSliceToSetOrNull(skill.AgentTypes)
	m.ResourceTypes = stringSliceToSetOrNull(skill.ResourceTypes)
	m.IsEnabled = types.BoolValue(skill.IsEnabled)
	m.IsBuiltin = types.BoolValue(skill.IsBuiltin)
	m.CreatedAt = types.StringValue(skill.CreatedAt)
	m.UpdatedAt = types.StringValue(skill.UpdatedAt)
}
