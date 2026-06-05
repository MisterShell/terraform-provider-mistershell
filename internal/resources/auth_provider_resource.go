package resources

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
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
	_ resource.Resource                = &AuthProviderResource{}
	_ resource.ResourceWithImportState = &AuthProviderResource{}
)

type AuthProviderResource struct {
	client *client.Client
}

type AuthProviderResourceModel struct {
	ID                 types.Int64          `tfsdk:"id"`
	Name               types.String         `tfsdk:"name"`
	ProviderType       types.String         `tfsdk:"provider_type"`
	IsEnabled          types.Bool           `tfsdk:"is_enabled"`
	DisplayOrder       types.Int64          `tfsdk:"display_order"`
	Config             jsontypes.Normalized `tfsdk:"config"`
	GroupMappingsCount types.Int64          `tfsdk:"group_mappings_count"`
	CreatedAt          types.String         `tfsdk:"created_at"`
	UpdatedAt          types.String         `tfsdk:"updated_at"`
}

func NewAuthProviderResource() resource.Resource {
	return &AuthProviderResource{}
}

func (r *AuthProviderResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_auth_provider"
}

func (r *AuthProviderResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a MisterShell external authentication provider (LDAP, OIDC, or SAML). The config attribute is an opaque per-provider_type JSON blob carrying secrets; the API masks secrets ('****') and enriches non-secret defaults, so config is stored from your plan (not read back). display_order is set declaratively (honored on create; omitted appends at the end). Create and update require an active license.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Description: "Auth provider ID.",
				Computed:    true,
				PlanModifiers: []planmodifier.Int64{
					int64UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Unique provider name (1-100 characters).",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, 100),
				},
			},
			"provider_type": schema.StringAttribute{
				Description: "Provider type. One of: LDAP, OIDC, SAML. Cannot be changed after creation (the config shape is type-coupled).",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.OneOf(client.SupportedAuthProviderTypes...),
				},
			},
			"is_enabled": schema.BoolAttribute{
				Description: "Whether the provider is enabled. Defaults to true.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
			},
			"display_order": schema.Int64Attribute{
				Description: "Login-page display order (>=0). Omit to keep the server's auto-assigned value (max+1). Assign distinct values for deterministic ordering; set declaratively via PATCH (the provider never calls the reorder endpoint).",
				Optional:    true,
				Computed:    true,
				Validators: []validator.Int64{
					int64validator.AtLeast(0),
				},
				PlanModifiers: []planmodifier.Int64{
					int64UseStateForUnknown(),
				},
			},
			"config": schema.StringAttribute{
				Description: "Per-provider_type config as JSON. Use jsonencode() in HCL. For LDAP: server_url, bind_dn, bind_password (secret), base_dn, and optional user_search_filter/user_attributes/group_search_base/group_search_filter/use_tls/connection_timeout. For OIDC: issuer_url, client_id, client_secret (secret), and optional scopes/claims_mapping/redirect_uri/additional_params. For SAML: idp_entity_id, idp_sso_url, idp_x509_cert, and optional sp_entity_id/sp_acs_url/attribute_mapping/sign_requests/want_assertions_signed/sp_private_key (secret)/sp_x509_cert. The API masks secrets as '****' and enriches non-secret defaults, so this value is stored from config (not read back) and is not importable.",
				Required:    true,
				Sensitive:   true,
				CustomType:  jsontypes.NormalizedType{},
			},
			"group_mappings_count": schema.Int64Attribute{
				Description: "Number of external group->role mappings attached to this provider.",
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

func (r *AuthProviderResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *AuthProviderResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan AuthProviderResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	input := client.AuthProviderCreateInput{
		Name:         plan.Name.ValueString(),
		ProviderType: plan.ProviderType.ValueString(),
		Config:       normalizedToRawJSON(plan.Config),
		// display_order is honored on create since the backend gained the field
		// (previously the server always assigned max+1 and the provider had to
		// PATCH it afterwards); omitted -> appended at max+1, as before.
		DisplayOrder: int64ValueToPtr(plan.DisplayOrder),
	}
	if !plan.IsEnabled.IsNull() && !plan.IsEnabled.IsUnknown() {
		v := plan.IsEnabled.ValueBool()
		input.IsEnabled = &v
	}

	created, err := r.client.CreateAuthProvider(ctx, input)
	if err != nil {
		resp.Diagnostics.AddError("Error creating auth provider", err.Error())
		return
	}

	// GET-single to read the final display_order, group_mappings_count, timestamps.
	detail, err := r.client.GetAuthProvider(ctx, created.ID)
	if err != nil {
		resp.Diagnostics.AddError("Error reading auth provider after create", err.Error())
		return
	}

	// Preserve config from the plan (the API masks secrets and enriches defaults).
	savedConfig := plan.Config
	mapAuthProviderDetailToModel(detail, &plan)
	plan.Config = savedConfig
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *AuthProviderResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state AuthProviderResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	detail, err := r.client.GetAuthProvider(ctx, state.ID.ValueInt64())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading auth provider", err.Error())
		return
	}

	// Preserve config from state (API masks secrets and enriches non-secret defaults).
	savedConfig := state.Config
	mapAuthProviderDetailToModel(detail, &state)
	state.Config = savedConfig
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *AuthProviderResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan AuthProviderResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state AuthProviderResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	input := client.AuthProviderUpdateInput{}
	if !plan.Name.Equal(state.Name) {
		v := plan.Name.ValueString()
		input.Name = &v
	}
	if !plan.IsEnabled.Equal(state.IsEnabled) {
		v := plan.IsEnabled.ValueBool()
		input.IsEnabled = &v
	}
	if !plan.DisplayOrder.Equal(state.DisplayOrder) {
		v := plan.DisplayOrder.ValueInt64()
		input.DisplayOrder = &v
	}
	if !plan.Config.Equal(state.Config) {
		input.Config = normalizedToRawJSON(plan.Config)
	}

	if _, err := r.client.UpdateAuthProvider(ctx, state.ID.ValueInt64(), input); err != nil {
		resp.Diagnostics.AddError("Error updating auth provider", err.Error())
		return
	}

	detail, err := r.client.GetAuthProvider(ctx, state.ID.ValueInt64())
	if err != nil {
		resp.Diagnostics.AddError("Error reading auth provider after update", err.Error())
		return
	}

	// Preserve config from the plan (API masks secrets and enriches defaults).
	savedConfig := plan.Config
	mapAuthProviderDetailToModel(detail, &plan)
	plan.Config = savedConfig
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *AuthProviderResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state AuthProviderResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteAuthProvider(ctx, state.ID.ValueInt64()); err != nil {
		resp.Diagnostics.AddError("Error deleting auth provider", err.Error())
	}
}

func (r *AuthProviderResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("Expected integer ID, got: %s", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), types.Int64Value(id))...)
}

// mapAuthProviderDetailToModel maps the GET-single response to the model.
// config is set by the caller per the stored-from-config rule.
func mapAuthProviderDetailToModel(detail *client.AuthProviderDetailResponse, m *AuthProviderResourceModel) {
	m.ID = types.Int64Value(detail.ID)
	m.Name = types.StringValue(detail.Name)
	m.ProviderType = types.StringValue(detail.ProviderType)
	m.IsEnabled = types.BoolValue(detail.IsEnabled)
	m.DisplayOrder = types.Int64Value(detail.DisplayOrder)
	m.GroupMappingsCount = types.Int64Value(detail.GroupMappingsCount)
	m.CreatedAt = types.StringValue(detail.CreatedAt)
	m.UpdatedAt = types.StringValue(detail.UpdatedAt)
}
