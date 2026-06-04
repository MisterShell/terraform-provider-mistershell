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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-mistershell/internal/client"
)

var (
	_ resource.Resource                = &AIPromptResource{}
	_ resource.ResourceWithImportState = &AIPromptResource{}
)

type AIPromptResource struct {
	client *client.Client
}

type AIPromptResourceModel struct {
	ID             types.Int64          `tfsdk:"id"`
	Name           types.String         `tfsdk:"name"`
	Type           types.String         `tfsdk:"type"`
	Content        types.String         `tfsdk:"content"`
	Description    types.String         `tfsdk:"description"`
	VariableSchema jsontypes.Normalized `tfsdk:"variable_schema"`
	CreatedAt      types.String         `tfsdk:"created_at"`
	UpdatedAt      types.String         `tfsdk:"updated_at"`
}

func NewAIPromptResource() resource.Resource {
	return &AIPromptResource{}
}

func (r *AIPromptResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ai_prompt"
}

func (r *AIPromptResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a custom MisterShell AI prompt (type=\"user\"). Builtin type=\"system\" prompts cannot be created, edited, or deleted (the API returns 403) and are available through the mistershell_ai_prompt data source.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Description: "AI prompt ID.",
				Computed:    true,
				PlanModifiers: []planmodifier.Int64{
					int64UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "AI prompt name (1-255 characters).",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, 255),
				},
			},
			"type": schema.StringAttribute{
				Description: "Prompt type. Must be \"user\" (the default). Builtin \"system\" prompts are read-only and cannot be managed by this resource — use the mistershell_ai_prompt data source to read them. Immutable: changing it forces replacement.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("user"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.OneOf("user"),
				},
			},
			"content": schema.StringAttribute{
				Description: "The prompt text. May contain Jinja2 template variables such as {{resource_id}}.",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"description": schema.StringAttribute{
				Description: "Optional human-readable description of the prompt.",
				Optional:    true,
			},
			"variable_schema": schema.StringAttribute{
				Description: "Optional JSON Schema documenting the template variables used in content. Use jsonencode() in HCL. Stored from config.",
				Optional:    true,
				CustomType:  jsontypes.NormalizedType{},
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

func (r *AIPromptResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *AIPromptResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan AIPromptResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	input := client.AIPromptCreateInput{
		Name:           plan.Name.ValueString(),
		Type:           plan.Type.ValueString(),
		Content:        plan.Content.ValueString(),
		Description:    stringValueToPtr(plan.Description),
		VariableSchema: normalizedToRawJSON(plan.VariableSchema),
	}

	prompt, err := r.client.CreateAIPrompt(ctx, input)
	if err != nil {
		resp.Diagnostics.AddError("Error creating AI prompt", err.Error())
		return
	}

	// variable_schema is stored from config like other opaque JSON blobs to avoid
	// any byte-wise drift from server-side key reordering.
	savedSchema := plan.VariableSchema
	mapAIPromptResponseToModel(prompt, &plan)
	plan.VariableSchema = savedSchema
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *AIPromptResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state AIPromptResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	prompt, err := r.client.GetAIPrompt(ctx, state.ID.ValueInt64())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading AI prompt", err.Error())
		return
	}

	// Preserve variable_schema from state (stored from config).
	savedSchema := state.VariableSchema
	mapAIPromptResponseToModel(prompt, &state)
	state.VariableSchema = savedSchema
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *AIPromptResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan AIPromptResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state AIPromptResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	input := client.AIPromptUpdateInput{
		Name:           stringValueToPtr(plan.Name),
		Content:        stringValueToPtr(plan.Content),
		Description:    stringValueToPtr(plan.Description),
		VariableSchema: normalizedToRawJSON(plan.VariableSchema),
	}

	prompt, err := r.client.UpdateAIPrompt(ctx, state.ID.ValueInt64(), input)
	if err != nil {
		resp.Diagnostics.AddError("Error updating AI prompt", err.Error())
		return
	}

	// Preserve variable_schema from plan (stored from config).
	savedSchema := plan.VariableSchema
	mapAIPromptResponseToModel(prompt, &plan)
	plan.VariableSchema = savedSchema
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *AIPromptResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state AIPromptResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// The API returns 403 if this is a builtin (type="system") prompt or if the
	// prompt is referenced by an agent — surface the error to the user.
	if err := r.client.DeleteAIPrompt(ctx, state.ID.ValueInt64()); err != nil {
		resp.Diagnostics.AddError("Error deleting AI prompt", err.Error())
	}
}

func (r *AIPromptResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("Expected integer ID, got: %s", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), types.Int64Value(id))...)
}

// mapAIPromptResponseToModel maps the API response to the Terraform model.
// variable_schema is set by the caller (stored from config).
func mapAIPromptResponseToModel(prompt *client.AIPromptResponse, m *AIPromptResourceModel) {
	m.ID = types.Int64Value(prompt.ID)
	m.Name = types.StringValue(prompt.Name)
	m.Type = types.StringValue(prompt.Type)
	m.Content = types.StringValue(prompt.Content)
	m.Description = stringPtrToValue(prompt.Description)
	m.CreatedAt = types.StringValue(prompt.CreatedAt)
	m.UpdatedAt = types.StringValue(prompt.UpdatedAt)
}
