package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
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
	_ resource.Resource                   = &AuthProviderResource{}
	_ resource.ResourceWithImportState    = &AuthProviderResource{}
	_ resource.ResourceWithValidateConfig = &AuthProviderResource{}
)

type AuthProviderResource struct {
	client *client.Client
}

type AuthProviderResourceModel struct {
	ID                 types.Int64   `tfsdk:"id"`
	Name               types.String  `tfsdk:"name"`
	Mnemonic           types.String  `tfsdk:"mnemonic"`
	ProviderType       types.String  `tfsdk:"provider_type"`
	IsEnabled          types.Bool    `tfsdk:"is_enabled"`
	DisplayOrder       types.Int64   `tfsdk:"display_order"`
	Config             types.Dynamic `tfsdk:"config"`
	GroupMappingsCount types.Int64   `tfsdk:"group_mappings_count"`
	SPEntityID         types.String  `tfsdk:"sp_entity_id"`
	SPACSURL           types.String  `tfsdk:"sp_acs_url"`
	OIDCRedirectURI    types.String  `tfsdk:"oidc_redirect_uri"`
	CreatedAt          types.String  `tfsdk:"created_at"`
	UpdatedAt          types.String  `tfsdk:"updated_at"`
}

func NewAuthProviderResource() resource.Resource {
	return &AuthProviderResource{}
}

func (r *AuthProviderResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_auth_provider"
}

func (r *AuthProviderResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a MisterShell external authentication provider (LDAP, OIDC, or SAML). The config attribute is a native per-provider_type Terraform object carrying secrets; the API masks secrets ('****') and enriches non-secret defaults, so non-secret managed config fields are refreshed while masked secrets retain their configured values. display_order is set declaratively (honored on create; omitted appends at the end). Create and update require an active license.",
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
			"mnemonic": schema.StringAttribute{
				Description: "Stable URL token used in login, callback, ACS, and metadata URLs. Immutable after creation.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.RegexMatches(regexp.MustCompile(`^[a-zA-Z0-9-]{1,50}$`), "must contain 1-50 letters, digits, or hyphens"),
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
			"config": schema.DynamicAttribute{
				Description: "Per-provider_type config as a native Terraform object. For LDAP: server_url, bind_dn, bind_password (secret), base_dn, and optional user_search_filter/user_attributes/group_search_base/group_search_filter/use_tls/tls_verify/connection_timeout/mfa_method (none or totp; email MFA cannot be managed with Terraform; removing a managed mfa_method resets it to none, while an already unmanaged method is preserved). For OIDC: issuer_url, client_id, client_secret (secret), and optional scopes/claims_mapping/additional_params/tls_verify/ldap_group_provider_id/mcp_audience/mcp_email_claim. For SAML: idp_entity_id, idp_sso_url, idp_x509_cert (PEM, BEGIN/END CERTIFICATE markers required), and optional attribute_mapping/sign_requests/want_assertions_signed/sp_private_key (secret)/sp_x509_cert; SAML rejects unknown keys (the SP URLs are derived server-side and exposed as the read-only sp_entity_id/sp_acs_url attributes). Managed non-secret fields are refreshed; masked secrets retain configured values. Imports populate non-secret config; supply secret values before applying. Removing a managed optional key restores its API default or clears nullable fields.",
				Required:    true,
				Sensitive:   true,
			},
			"group_mappings_count": schema.Int64Attribute{
				Description: "Number of external group->role mappings attached to this provider.",
				Computed:    true,
			},
			"sp_entity_id": schema.StringAttribute{
				Description: "SAML only (null for LDAP/OIDC): service-provider entity ID derived from app_base_url and mnemonic; register it with your IdP.",
				Computed:    true,
			},
			"sp_acs_url": schema.StringAttribute{
				Description: "SAML only (null for LDAP/OIDC): Assertion Consumer Service URL derived from app_base_url and mnemonic; register it with your IdP.",
				Computed:    true,
			},
			"oidc_redirect_uri": schema.StringAttribute{
				Description: "OIDC callback URL to register with the identity provider. Derived from app_base_url and mnemonic; null for LDAP/SAML.",
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

// terraformMfaMethods are the LDAP config.mfa_method values this provider can
// apply. MisterShell only turns on "email" with a single-use activation proof
// from an interactive emailed-code check, which Terraform cannot perform.
var terraformMfaMethods = []string{"none", "totp"}

// ValidateConfig rejects config.mfa_method values the provider cannot apply,
// so they fail at plan time instead of with HTTP 400 at apply.
func (r *AuthProviderResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config types.Dynamic
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("config"), &config)...)
	if resp.Diagnostics.HasError() || config.IsNull() || config.IsUnknown() || config.IsUnderlyingValueUnknown() {
		return
	}
	var attrs map[string]attr.Value
	switch v := config.UnderlyingValue().(type) {
	case types.Object:
		attrs = v.Attributes()
	case types.Map:
		attrs = v.Elements()
	default:
		return
	}
	method, ok := attrs["mfa_method"].(types.String)
	if !ok || method.IsNull() || method.IsUnknown() || slices.Contains(terraformMfaMethods, method.ValueString()) {
		return
	}
	detail := fmt.Sprintf("config.mfa_method must be one of: %s.", strings.Join(terraformMfaMethods, ", "))
	if method.ValueString() == "email" {
		detail = "Email MFA cannot be managed with Terraform: MisterShell only enables it after an interactive emailed-code check. " +
			"First omit mfa_method from config and apply; this resets any previously managed method to none. Then enable email MFA in the MisterShell UI (Settings > User Access > Auth Providers). Terraform preserves the method while the key remains unmanaged."
	}
	resp.Diagnostics.AddAttributeError(path.Root("config").AtName("mfa_method"), "Unsupported mfa_method", detail)
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
	config, err := dynamicToRawJSON(plan.Config)
	if err != nil {
		resp.Diagnostics.AddError("Error encoding auth provider config", err.Error())
		return
	}

	input := client.AuthProviderCreateInput{
		Name:         plan.Name.ValueString(),
		Mnemonic:     plan.Mnemonic.ValueString(),
		ProviderType: plan.ProviderType.ValueString(),
		Config:       config,
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

	savedConfig := plan.Config
	mapAuthProviderResponseToModel(created, &plan)
	plan.Config = savedConfig
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// GET-single to read the final display_order, group_mappings_count, timestamps.
	detail, err := r.client.GetAuthProvider(ctx, created.ID)
	if err != nil {
		resp.Diagnostics.AddError("Error reading auth provider after create", err.Error())
		return
	}

	// Preserve config from the plan (the API masks secrets and enriches defaults).
	mapAuthProviderDetailToModel(detail, &plan)
	plan.Config = savedConfig
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func mapAuthProviderResponseToModel(provider *client.AuthProviderResponse, m *AuthProviderResourceModel) {
	m.ID = types.Int64Value(provider.ID)
	m.Name = types.StringValue(provider.Name)
	m.Mnemonic = types.StringValue(provider.Mnemonic)
	m.ProviderType = types.StringValue(provider.ProviderType)
	m.IsEnabled = types.BoolValue(provider.IsEnabled)
	m.DisplayOrder = types.Int64Value(provider.DisplayOrder)
	m.GroupMappingsCount = types.Int64Value(0)
	m.SPEntityID = types.StringNull()
	m.SPACSURL = types.StringNull()
	m.OIDCRedirectURI = types.StringNull()
	m.CreatedAt = types.StringValue(provider.CreatedAt)
	m.UpdatedAt = types.StringValue(provider.UpdatedAt)
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

	// The LDAP revision is server-only MFA bookkeeping, not writable config.
	if detail.ProviderType == "LDAP" {
		var configFields map[string]json.RawMessage
		if err := json.Unmarshal(detail.Config, &configFields); err != nil {
			resp.Diagnostics.AddError("Error reading auth provider config", err.Error())
			return
		}
		delete(configFields, "mfa_revision")
		detail.Config, err = json.Marshal(configFields)
		if err != nil {
			resp.Diagnostics.AddError("Error reading auth provider config", err.Error())
			return
		}
	}
	config, err := authProviderRefreshConfig(detail.ProviderType, state.Config, detail.Config)
	if err != nil {
		resp.Diagnostics.AddError("Error reading auth provider config", err.Error())
		return
	}
	mapAuthProviderDetailToModel(detail, &state)
	state.Config = config
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
		config, err := authProviderConfigPatch(plan.ProviderType.ValueString(), state.Config, plan.Config)
		if err != nil {
			resp.Diagnostics.AddError("Error encoding auth provider config", err.Error())
			return
		}
		input.Config = config
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
	m.Mnemonic = types.StringValue(detail.Mnemonic)
	m.ProviderType = types.StringValue(detail.ProviderType)
	m.IsEnabled = types.BoolValue(detail.IsEnabled)
	m.DisplayOrder = types.Int64Value(detail.DisplayOrder)
	m.GroupMappingsCount = types.Int64Value(detail.GroupMappingsCount)
	m.SPEntityID = stringPtrToValue(detail.SPEntityID)
	m.SPACSURL = stringPtrToValue(detail.SPACSURL)
	m.OIDCRedirectURI = stringPtrToValue(detail.OIDCRedirectURI)
	m.CreatedAt = types.StringValue(detail.CreatedAt)
	m.UpdatedAt = types.StringValue(detail.UpdatedAt)
}
