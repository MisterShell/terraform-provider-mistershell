package resources

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-mistershell/internal/client"
)

var (
	_ resource.Resource                = &AuthProviderMappingResource{}
	_ resource.ResourceWithImportState = &AuthProviderMappingResource{}
)

type AuthProviderMappingResource struct {
	client *client.Client
}

type AuthProviderMappingResourceModel struct {
	ID            types.Int64  `tfsdk:"id"`
	ProviderID    types.Int64  `tfsdk:"provider_id"`
	ExternalGroup types.String `tfsdk:"external_group"`
	RoleID        types.Int64  `tfsdk:"role_id"`
	RoleName      types.String `tfsdk:"role_name"`
	CreatedAt     types.String `tfsdk:"created_at"`
	UpdatedAt     types.String `tfsdk:"updated_at"`
}

func NewAuthProviderMappingResource() resource.Resource {
	return &AuthProviderMappingResource{}
}

func (r *AuthProviderMappingResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_auth_provider_mapping"
}

func (r *AuthProviderMappingResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a MisterShell external group->role mapping under an auth provider. (provider_id, external_group) is unique. Import format is \"<provider_id>:<mapping_id>\". Create and update require an active license; deleting the parent provider cascades to its mappings.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Description: "Mapping ID.",
				Computed:    true,
				PlanModifiers: []planmodifier.Int64{
					int64UseStateForUnknown(),
				},
			},
			"provider_id": schema.Int64Attribute{
				Description: "ID of the auth provider this mapping belongs to. Cannot be changed after creation.",
				Required:    true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"external_group": schema.StringAttribute{
				Description: "External group identifier (LDAP DN / OIDC claim value / SAML attribute value), 1-500 characters.",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, 500),
				},
			},
			"role_id": schema.Int64Attribute{
				Description: "ID of the MisterShell role granted to members of the external group.",
				Required:    true,
			},
			"role_name": schema.StringAttribute{
				Description: "Name of the mapped role (read-only).",
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

func (r *AuthProviderMappingResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *AuthProviderMappingResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan AuthProviderMappingResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	input := client.GroupMappingCreateInput{
		ExternalGroup: plan.ExternalGroup.ValueString(),
		RoleID:        plan.RoleID.ValueInt64(),
	}

	m, err := r.client.CreateGroupMapping(ctx, plan.ProviderID.ValueInt64(), input)
	if err != nil {
		resp.Diagnostics.AddError("Error creating group mapping", err.Error())
		return
	}

	mapGroupMappingResponseToModel(m, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *AuthProviderMappingResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state AuthProviderMappingResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	m, err := r.client.GetGroupMapping(ctx, state.ProviderID.ValueInt64(), state.ID.ValueInt64())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading group mapping", err.Error())
		return
	}

	mapGroupMappingResponseToModel(m, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *AuthProviderMappingResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan AuthProviderMappingResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state AuthProviderMappingResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	input := client.GroupMappingUpdateInput{}
	if !plan.ExternalGroup.Equal(state.ExternalGroup) {
		v := plan.ExternalGroup.ValueString()
		input.ExternalGroup = &v
	}
	if !plan.RoleID.Equal(state.RoleID) {
		v := plan.RoleID.ValueInt64()
		input.RoleID = &v
	}

	m, err := r.client.UpdateGroupMapping(ctx, state.ProviderID.ValueInt64(), state.ID.ValueInt64(), input)
	if err != nil {
		resp.Diagnostics.AddError("Error updating group mapping", err.Error())
		return
	}

	mapGroupMappingResponseToModel(m, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *AuthProviderMappingResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state AuthProviderMappingResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteGroupMapping(ctx, state.ProviderID.ValueInt64(), state.ID.ValueInt64())
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting group mapping", err.Error())
	}
}

func (r *AuthProviderMappingResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, ":", 2)
	if len(parts) != 2 {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("Expected \"<provider_id>:<mapping_id>\", got: %s", req.ID))
		return
	}
	providerID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("Expected integer provider_id, got: %s", parts[0]))
		return
	}
	mappingID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("Expected integer mapping_id, got: %s", parts[1]))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("provider_id"), types.Int64Value(providerID))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), types.Int64Value(mappingID))...)
}

func mapGroupMappingResponseToModel(m *client.GroupMappingResponse, model *AuthProviderMappingResourceModel) {
	model.ID = types.Int64Value(m.ID)
	model.ProviderID = types.Int64Value(m.AuthProviderID)
	model.ExternalGroup = types.StringValue(m.ExternalGroup)
	model.RoleID = types.Int64Value(m.RoleID)
	model.RoleName = types.StringValue(m.RoleName)
	model.CreatedAt = types.StringValue(m.CreatedAt)
	model.UpdatedAt = types.StringValue(m.UpdatedAt)
}
