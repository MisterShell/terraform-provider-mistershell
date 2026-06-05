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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-mistershell/internal/client"
)

var (
	_ resource.Resource                = &AIModelResource{}
	_ resource.ResourceWithImportState = &AIModelResource{}
)

type AIModelResource struct {
	client *client.Client
}

type AIModelResourceModel struct {
	ID        types.Int64          `tfsdk:"id"`
	Name      types.String         `tfsdk:"name"`
	Provider  types.String         `tfsdk:"model_provider"`
	ModelID   types.String         `tfsdk:"model_id"`
	Config    jsontypes.Normalized `tfsdk:"config"`
	IsDefault types.Bool           `tfsdk:"is_default"`
	CreatedAt types.String         `tfsdk:"created_at"`
	UpdatedAt types.String         `tfsdk:"updated_at"`
}

func NewAIModelResource() resource.Resource {
	return &AIModelResource{}
}

func (r *AIModelResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ai_model"
}

func (r *AIModelResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a MisterShell AI model registration, pairing a provider and provider-specific model id with an opaque per-provider config blob (api_key, base_url, etc.). The config attribute is sensitive: provider secrets are masked by the API and stored from config (not read back).",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Description: "AI model ID.",
				Computed:    true,
				PlanModifiers: []planmodifier.Int64{
					int64UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "AI model name (1-255 characters).",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, 255),
				},
			},
			"model_provider": schema.StringAttribute{
				Description: "AI provider backing this model. One of: anthropic, azure_openai, bedrock, cohere, google, mistral, ollama, openai, openrouter, xai. (Named `model_provider` because `provider` is a reserved Terraform argument.)",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.OneOf(client.SupportedAIModelProviders...),
				},
			},
			"model_id": schema.StringAttribute{
				Description: "Provider-specific model identifier (e.g. \"claude-3-opus\", \"gpt-4o\").",
				Required:    true,
			},
			"config": schema.StringAttribute{
				Description: "Per-provider config as JSON. Use jsonencode() in HCL. Common keys: api_key, base_url, context_window, api_version, region, deployment. Secrets such as api_key are write-only: they are masked as '***' by the API on read and stored from config, not read back. See the config field tables on this page.",
				Optional:    true,
				Sensitive:   true,
				CustomType:  jsontypes.NormalizedType{},
			},
			"is_default": schema.BoolAttribute{
				Description: "Whether this is the default AI model. Defaults to false. Only one model may be the default: setting this to true clears the default flag on all other models. Manage is_default from a single resource to avoid drift.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
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

func (r *AIModelResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *AIModelResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan AIModelResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	input := client.AIModelCreateInput{
		Name:      plan.Name.ValueString(),
		Provider:  plan.Provider.ValueString(),
		ModelID:   plan.ModelID.ValueString(),
		Config:    normalizedToRawJSON(plan.Config),
		IsDefault: boolValueToPtr(plan.IsDefault),
	}

	model, err := r.client.CreateAIModel(ctx, input)
	if err != nil {
		resp.Diagnostics.AddError("Error creating AI model", err.Error())
		return
	}

	// Preserve config from plan: provider secrets (e.g. api_key) are masked as
	// '***' by the API, so the response config differs from the planned config.
	savedConfig := plan.Config
	mapAIModelResponseToModel(model, &plan)
	plan.Config = savedConfig
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *AIModelResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state AIModelResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	model, err := r.client.GetAIModel(ctx, state.ID.ValueInt64())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading AI model", err.Error())
		return
	}

	// Preserve the config already in state (the user's value). Provider secrets
	// (e.g. api_key) are masked as '***' by the API, so reflecting the server
	// config would differ from the planned config and break Terraform's
	// apply-consistency contract for this Sensitive attribute. Use the
	// stored-from-config rule (like log_destination's config / credential_data).
	savedConfig := state.Config
	mapAIModelResponseToModel(model, &state)
	state.Config = savedConfig
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *AIModelResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan AIModelResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state AIModelResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	input := client.AIModelUpdateInput{
		Name:      stringValueToPtr(plan.Name),
		Provider:  stringValueToPtr(plan.Provider),
		ModelID:   stringValueToPtr(plan.ModelID),
		Config:    normalizedToRawJSON(plan.Config),
		IsDefault: boolValueToPtr(plan.IsDefault),
	}

	model, err := r.client.UpdateAIModel(ctx, state.ID.ValueInt64(), input)
	if err != nil {
		resp.Diagnostics.AddError("Error updating AI model", err.Error())
		return
	}

	// Preserve config from plan (provider secrets are masked '***' by the API).
	savedConfig := plan.Config
	mapAIModelResponseToModel(model, &plan)
	plan.Config = savedConfig
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *AIModelResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state AIModelResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// The API returns 409 if the model is still referenced by an agent; let that
	// error surface so the user can detach the agent first.
	if err := r.client.DeleteAIModel(ctx, state.ID.ValueInt64()); err != nil {
		resp.Diagnostics.AddError("Error deleting AI model", err.Error())
	}
}

func (r *AIModelResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("Expected integer ID, got: %s", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), types.Int64Value(id))...)
}

// mapAIModelResponseToModel maps the API response to the Terraform model.
// config is set by the caller per the stored-from-config secret rule.
func mapAIModelResponseToModel(model *client.AIModelResponse, m *AIModelResourceModel) {
	m.ID = types.Int64Value(model.ID)
	m.Name = types.StringValue(model.Name)
	m.Provider = types.StringValue(model.Provider)
	m.ModelID = types.StringValue(model.ModelID)
	m.IsDefault = types.BoolValue(model.IsDefault)
	m.CreatedAt = types.StringValue(model.CreatedAt)
	m.UpdatedAt = types.StringValue(model.UpdatedAt)
}
