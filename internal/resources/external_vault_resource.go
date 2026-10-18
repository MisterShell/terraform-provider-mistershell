package resources

import (
	"context"
	"fmt"
	"regexp"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"terraform-provider-mistershell/internal/client"
)

var (
	_ resource.Resource                   = &ExternalVaultResource{}
	_ resource.ResourceWithImportState    = &ExternalVaultResource{}
	_ resource.ResourceWithValidateConfig = &ExternalVaultResource{}
)

type ExternalVaultResource struct{ client *client.Client }
type ExternalVaultResourceModel struct {
	ID              types.Int64   `tfsdk:"id"`
	Name            types.String  `tfsdk:"name"`
	Mnemonic        types.String  `tfsdk:"mnemonic"`
	Description     types.String  `tfsdk:"description"`
	VaultType       types.String  `tfsdk:"vault_type"`
	IsEnabled       types.Bool    `tfsdk:"is_enabled"`
	Config          types.Dynamic `tfsdk:"config"`
	CacheTTLSeconds types.Int64   `tfsdk:"cache_ttl_seconds"`
	LastCheckAt     types.String  `tfsdk:"last_check_at"`
	LastCheckStatus types.String  `tfsdk:"last_check_status"`
	LastCheckError  types.String  `tfsdk:"last_check_error"`
	CredentialCount types.Int64   `tfsdk:"credential_count"`
	CreatedAt       types.String  `tfsdk:"created_at"`
	UpdatedAt       types.String  `tfsdk:"updated_at"`
}

func NewExternalVaultResource() resource.Resource { return &ExternalVaultResource{} }
func (r *ExternalVaultResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_external_vault"
}
func (r *ExternalVaultResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{Description: "Manages an external vault connection. MisterShell resolves secrets; Terraform never fetches them. Requires the VAULT_EXTERNAL capability (Pro or Enterprise) and an upgraded Core deployment. Sensitive configuration remains in Terraform state.", Attributes: map[string]schema.Attribute{
		"id":                schema.Int64Attribute{Computed: true, Description: "External vault ID.", PlanModifiers: []planmodifier.Int64{int64UseStateForUnknown()}},
		"name":              schema.StringAttribute{Required: true, Description: "Unique vault name (1–128 characters).", Validators: []validator.String{stringvalidator.LengthBetween(1, 128)}},
		"mnemonic":          schema.StringAttribute{Required: true, Description: "Immutable identifier: 2–49 lowercase letters, digits or hyphens, starting with a letter or digit.", Validators: []validator.String{stringvalidator.RegexMatches(regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,48}$`), "must contain 2–49 lowercase letters, digits or hyphens and start with a letter or digit")}, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"vault_type":        schema.StringAttribute{Required: true, Description: "hashicorp, openbao, aws_secrets_manager or azure_key_vault. Immutable.", Validators: []validator.String{stringvalidator.OneOf("hashicorp", "openbao", "aws_secrets_manager", "azure_key_vault")}, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"description":       schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString(""), Description: "Description (up to 255 characters).", Validators: []validator.String{stringvalidator.LengthAtMost(255)}},
		"is_enabled":        schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true), Description: "Whether the vault can be used by credentials. Defaults to true."},
		"cache_ttl_seconds": schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(60), Description: "Secret cache lifetime, 0–3600 seconds. Defaults to 60.", Validators: []validator.Int64{int64validator.Between(0, 3600)}},
		"config":            schema.DynamicAttribute{Required: true, Sensitive: true, Description: "Native vault-type configuration object. Secret values are preserved across masked reads. Nonsecret managed keys refresh from the API. Removing a key resets its catalog default or nullable value; unsupported removal returns a diagnostic. Imported masked secrets become null; leaving them null or omitted preserves stored credentials during unrelated edits. Supply a plaintext secret to replace it. Ambient/Kubernetes authentication uses MisterShell Core's identity and environment."},
		"last_check_at":     schema.StringAttribute{Computed: true, Description: "Timestamp of the last check. Terraform does not initiate checks."},
		"last_check_status": schema.StringAttribute{Computed: true, Description: "Status of the last check."},
		"last_check_error":  schema.StringAttribute{Computed: true, Description: "Error from the last check, if any."},
		"credential_count":  schema.Int64Attribute{Computed: true, Description: "Number of credentials referencing this vault. References prevent deletion."},
		"created_at":        schema.StringAttribute{Computed: true, Description: "Creation timestamp.", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
		"updated_at":        schema.StringAttribute{Computed: true, Description: "Last update timestamp."},
	}}
}
func (r *ExternalVaultResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
func (r *ExternalVaultResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config types.Dynamic
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("config"), &config)...)
	if resp.Diagnostics.HasError() || config.IsNull() || config.IsUnknown() || config.IsUnderlyingValueUnknown() {
		return
	}
	switch config.UnderlyingValue().(type) {
	case types.Object, types.Map:
		return
	default:
		resp.Diagnostics.AddAttributeError(path.Root("config"), "Invalid vault configuration", "config must be a native Terraform object or map.")
	}
}
func (r *ExternalVaultResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ExternalVaultResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	cfg, err := dynamicToRawJSON(plan.Config)
	if err != nil {
		resp.Diagnostics.AddError("Error encoding vault config", err.Error())
		return
	}
	enabled, ttl := plan.IsEnabled.ValueBool(), plan.CacheTTLSeconds.ValueInt64()
	result, err := r.client.CreateExternalVault(ctx, client.ExternalVaultCreateInput{Name: plan.Name.ValueString(), Mnemonic: plan.Mnemonic.ValueString(), Description: plan.Description.ValueString(), VaultType: plan.VaultType.ValueString(), IsEnabled: &enabled, CacheTTLSeconds: &ttl, Config: cfg})
	if err != nil {
		resp.Diagnostics.AddError("Error creating external vault", err.Error())
		return
	}
	mapExternalVaultToModel(result, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}
func (r *ExternalVaultResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ExternalVaultResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	result, err := r.client.GetExternalVault(ctx, state.ID.ValueInt64())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading external vault", err.Error())
		return
	}
	catalog, err := r.client.ListExternalVaultTypes(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading vault type catalog", err.Error())
		return
	}
	var selected *client.ExternalVaultType
	for i := range catalog {
		if catalog[i].VaultType == result.VaultType {
			selected = &catalog[i]
			break
		}
	}
	if selected == nil {
		resp.Diagnostics.AddError("Missing vault type", "The backend catalog does not contain this vault type.")
		return
	}
	config, err := externalVaultRefreshConfig(ctx, state.Config, result.Config, selected.SecretConfigKeys)
	if err != nil {
		resp.Diagnostics.AddError("Error decoding vault config", err.Error())
		return
	}
	state.Config = config
	mapExternalVaultToModel(result, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
func (r *ExternalVaultResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state ExternalVaultResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	input := client.ExternalVaultUpdateInput{}
	if !plan.Name.Equal(state.Name) {
		v := plan.Name.ValueString()
		input.Name = &v
	}
	if !plan.Description.Equal(state.Description) {
		v := plan.Description.ValueString()
		input.Description = &v
	}
	if !plan.IsEnabled.Equal(state.IsEnabled) {
		v := plan.IsEnabled.ValueBool()
		input.IsEnabled = &v
	}
	if !plan.CacheTTLSeconds.Equal(state.CacheTTLSeconds) {
		v := plan.CacheTTLSeconds.ValueInt64()
		input.CacheTTLSeconds = &v
	}
	if !plan.Config.Equal(state.Config) {
		catalog, err := r.client.ListExternalVaultTypes(ctx)
		if err != nil {
			resp.Diagnostics.AddError("Error reading vault type catalog", err.Error())
			return
		}
		var selected *client.ExternalVaultType
		for i := range catalog {
			if catalog[i].VaultType == plan.VaultType.ValueString() {
				selected = &catalog[i]
				break
			}
		}
		if selected == nil {
			resp.Diagnostics.AddError("Missing vault type", "The backend catalog does not contain this vault type.")
			return
		}
		config, err := externalVaultConfigPatch(state.Config, plan.Config, *selected)
		if err != nil {
			resp.Diagnostics.AddAttributeError(path.Root("config"), "Cannot update vault configuration", err.Error())
			return
		}
		input.Config = config
	}
	result, err := r.client.UpdateExternalVault(ctx, state.ID.ValueInt64(), input)
	if err != nil {
		resp.Diagnostics.AddError("Error updating external vault", err.Error())
		return
	}
	mapExternalVaultToModel(result, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}
func (r *ExternalVaultResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ExternalVaultResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteExternalVault(ctx, state.ID.ValueInt64()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting external vault", err.Error())
	}
}
func (r *ExternalVaultResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil || id <= 0 {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("Expected a positive numeric vault ID, got %q", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), types.Int64Value(id))...)
}
func mapExternalVaultToModel(v *client.ExternalVaultResponse, m *ExternalVaultResourceModel) {
	m.ID = types.Int64Value(v.ID)
	m.Name = types.StringValue(v.Name)
	m.Mnemonic = types.StringValue(v.Mnemonic)
	m.Description = types.StringValue(v.Description)
	m.VaultType = types.StringValue(v.VaultType)
	m.IsEnabled = types.BoolValue(v.IsEnabled)
	m.CacheTTLSeconds = types.Int64Value(v.CacheTTLSeconds)
	m.LastCheckAt = stringPtrToValue(v.LastCheckAt)
	m.LastCheckStatus = stringPtrToValue(v.LastCheckStatus)
	m.LastCheckError = stringPtrToValue(v.LastCheckError)
	m.CredentialCount = types.Int64Value(v.CredentialCount)
	m.CreatedAt = types.StringValue(v.CreatedAt)
	m.UpdatedAt = types.StringValue(v.UpdatedAt)
}
