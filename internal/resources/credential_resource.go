package resources

import (
	"context"
	"encoding/json"
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
	_ resource.Resource                = &CredentialResource{}
	_ resource.ResourceWithImportState = &CredentialResource{}
)

type CredentialResource struct {
	client *client.Client
}

type CredentialResourceModel struct {
	ID                  types.Int64          `tfsdk:"id"`
	Name                types.String         `tfsdk:"name"`
	CredentialType      types.String         `tfsdk:"credential_type"`
	CredentialData      jsontypes.Normalized `tfsdk:"credential_data"`
	Description         types.String         `tfsdk:"description"`
	RequiresUserMapping types.Bool           `tfsdk:"requires_user_mapping"`
	ExtraData           jsontypes.Normalized `tfsdk:"extra_data"`
	CreatedAt           types.String         `tfsdk:"created_at"`
	UpdatedAt           types.String         `tfsdk:"updated_at"`
}

func NewCredentialResource() resource.Resource {
	return &CredentialResource{}
}

func (r *CredentialResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_credential"
}

func (r *CredentialResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a MisterShell credential. Credentials are encrypted at rest and used by network resources for authentication.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Description: "Credential ID.",
				Computed:    true,
				PlanModifiers: []planmodifier.Int64{
					int64UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Unique credential name.",
				Required:    true,
			},
			"credential_type": schema.StringAttribute{
				Description: "Credential type. Cannot be changed after creation.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.OneOf(client.SupportedCredentialTypes...),
				},
			},
			"credential_data": schema.StringAttribute{
				Description: "Credential secret data as JSON. Use jsonencode() in HCL. Fields vary by credential_type; see the valid credential_type values table on this page for the fields per type. Note: the API masks secret values in responses, so this value is stored from config, not read back from the server.",
				Required:    true,
				Sensitive:   true,
				CustomType:  jsontypes.NormalizedType{},
			},
			"description": schema.StringAttribute{
				Description: "Credential description.",
				Optional:    true,
			},
			"requires_user_mapping": schema.BoolAttribute{
				Description: "Whether users must fill their own credential copy for interactive sessions.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"extra_data": schema.StringAttribute{
				Description: "Non-sensitive metadata as JSON. Use jsonencode() in HCL.",
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

func (r *CredentialResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *CredentialResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan CredentialResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	input := client.CredentialCreateInput{
		Name:           plan.Name.ValueString(),
		CredentialType: plan.CredentialType.ValueString(),
		CredentialData: json.RawMessage(plan.CredentialData.ValueString()),
	}
	if !plan.Description.IsNull() {
		v := plan.Description.ValueString()
		input.Description = &v
	}
	if !plan.RequiresUserMapping.IsNull() && !plan.RequiresUserMapping.IsUnknown() {
		v := plan.RequiresUserMapping.ValueBool()
		input.RequiresUserMapping = &v
	}
	input.ExtraData = normalizedToRawJSON(plan.ExtraData)

	cred, err := r.client.CreateCredential(ctx, input)
	if err != nil {
		resp.Diagnostics.AddError("Error creating credential", err.Error())
		return
	}

	// Map response to model but preserve credential_data from plan (API returns masked values).
	mapCredentialResponseToModel(cred, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *CredentialResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state CredentialResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	cred, err := r.client.GetCredential(ctx, state.ID.ValueInt64())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading credential", err.Error())
		return
	}

	// Update all fields EXCEPT credential_data — the API returns masked values (****),
	// so we keep the user-provided value already in state.
	savedCredentialData := state.CredentialData
	mapCredentialResponseToModel(cred, &state)
	state.CredentialData = savedCredentialData
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *CredentialResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan CredentialResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state CredentialResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	input := client.CredentialUpdateInput{}
	if !plan.Name.Equal(state.Name) {
		v := plan.Name.ValueString()
		input.Name = &v
	}
	input.Description = stringValueToPtr(plan.Description)
	if !plan.RequiresUserMapping.Equal(state.RequiresUserMapping) {
		v := plan.RequiresUserMapping.ValueBool()
		input.RequiresUserMapping = &v
	}
	input.ExtraData = normalizedToRawJSON(plan.ExtraData)

	// Always send credential_data on update — the user's config value, not the masked API value.
	if !plan.CredentialData.IsNull() && !plan.CredentialData.IsUnknown() {
		input.CredentialData = json.RawMessage(plan.CredentialData.ValueString())
	}

	cred, err := r.client.UpdateCredential(ctx, state.ID.ValueInt64(), input)
	if err != nil {
		resp.Diagnostics.AddError("Error updating credential", err.Error())
		return
	}

	// Preserve credential_data from plan (API response has masked values).
	mapCredentialResponseToModel(cred, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *CredentialResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state CredentialResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteCredential(ctx, state.ID.ValueInt64()); err != nil {
		resp.Diagnostics.AddError("Error deleting credential", err.Error())
	}
}

func (r *CredentialResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("Expected integer ID, got: %s", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), types.Int64Value(id))...)
}

// mapCredentialResponseToModel maps API response to Terraform model.
// Note: credential_data is set from the plan value by the caller, since the API returns masked values.
func mapCredentialResponseToModel(cred *client.CredentialResponse, m *CredentialResourceModel) {
	m.ID = types.Int64Value(cred.ID)
	m.Name = types.StringValue(cred.Name)
	m.CredentialType = types.StringValue(cred.CredentialType)
	// credential_data is intentionally preserved from the plan/state — API returns ****
	m.Description = stringPtrToValue(cred.Description)
	m.RequiresUserMapping = types.BoolValue(cred.RequiresUserMapping)
	m.ExtraData = rawJSONToNormalized(cred.ExtraData)
	m.CreatedAt = types.StringValue(cred.CreatedAt)
	m.UpdatedAt = types.StringValue(cred.UpdatedAt)
}
