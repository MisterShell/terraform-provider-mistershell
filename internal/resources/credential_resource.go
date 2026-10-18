package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-mistershell/internal/client"
	"terraform-provider-mistershell/internal/credentialrefs"
	"terraform-provider-mistershell/internal/tfjson"
)

var (
	_ resource.Resource                = &CredentialResource{}
	_ resource.ResourceWithImportState = &CredentialResource{}
)

type CredentialResource struct {
	client *client.Client
}

type CredentialResourceModel struct {
	Source                types.String  `tfsdk:"source"`
	ExternalVaultID       types.Int64   `tfsdk:"external_vault_id"`
	ExternalVaultRef      types.Object  `tfsdk:"external_vault_ref"`
	ExternalVaultName     types.String  `tfsdk:"external_vault_name"`
	ExternalVaultType     types.String  `tfsdk:"external_vault_type"`
	UserSource            types.String  `tfsdk:"user_source"`
	UserExternalVaultID   types.Int64   `tfsdk:"user_external_vault_id"`
	UserExternalVaultRef  types.Object  `tfsdk:"user_external_vault_ref"`
	UserExternalVaultName types.String  `tfsdk:"user_external_vault_name"`
	UserExternalVaultType types.String  `tfsdk:"user_external_vault_type"`
	UserCredentialData    types.Dynamic `tfsdk:"user_credential_data"`

	ID                  types.Int64   `tfsdk:"id"`
	Name                types.String  `tfsdk:"name"`
	CredentialType      types.String  `tfsdk:"credential_type"`
	CredentialData      types.Dynamic `tfsdk:"credential_data"`
	Description         types.String  `tfsdk:"description"`
	RequiresUserMapping types.Bool    `tfsdk:"requires_user_mapping"`
	ExtraData           types.Dynamic `tfsdk:"extra_data"`
	CreatedAt           types.String  `tfsdk:"created_at"`
	UpdatedAt           types.String  `tfsdk:"updated_at"`
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
				Description: "Installed credential type. MisterShell validates it against its runtime credential catalog. Changing type updates this credential in place and requires complete data valid for the new type; backend-managed singleton types cannot be managed by this resource.",
				Required:    true,
			},
			"credential_data": schema.DynamicAttribute{
				Description: "Credential secret data as a native Terraform object. Fields vary by credential_type; see the valid credential_type values table on this page for the fields per type. Readable fields refresh from the API; masked secret fields retain configured values. Imports hydrate readable fields and leave unknown secrets null.",
				Optional:    true,
				Sensitive:   true,
			},
			"description": schema.StringAttribute{
				Description: "Credential description.",
				Optional:    true,
			},
			"requires_user_mapping": schema.BoolAttribute{
				Description: "Whether interactive sessions use the independent user credential source. Automation always uses the platform source.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"extra_data": schema.DynamicAttribute{
				Description: "Non-sensitive metadata as a native Terraform value.",
				Optional:    true,
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
	for _, prefix := range []string{"", "user_"} {
		resp.Schema.Attributes[prefix+"source"] = schema.StringAttribute{Optional: true, Computed: true, Description: "Credential source: local or external. Defaults to local on creation. Changing the user source to external purges saved user fills.", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}}
		resp.Schema.Attributes[prefix+"external_vault_id"] = schema.Int64Attribute{Optional: true, Description: "Positive external vault ID, required for an external source."}
		resp.Schema.Attributes[prefix+"external_vault_ref"] = schema.SingleNestedAttribute{Optional: true, Description: "Vault reference. Templates are passed literally to MisterShell; Terraform never resolves secrets.", Attributes: map[string]schema.Attribute{
			"base":   schema.DynamicAttribute{Required: true, Description: "Vault-specific reference parameters as an object containing strings or integers."},
			"fields": schema.MapAttribute{Required: true, ElementType: types.StringType, Description: "Credential field names mapped to vault keys or secret names."},
		}}
		resp.Schema.Attributes[prefix+"external_vault_name"] = schema.StringAttribute{Computed: true, Description: "Referenced vault name."}
		resp.Schema.Attributes[prefix+"external_vault_type"] = schema.StringAttribute{Computed: true, Description: "Referenced vault type."}
	}
	resp.Schema.Attributes["user_credential_data"] = schema.DynamicAttribute{Optional: true, Sensitive: true, Description: "Literal fields for an external user source. Omit for local user credentials, which are filled individually."}

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

	input, err := credentialCreateInput(&plan)
	if err != nil {
		resp.Diagnostics.AddError("Invalid credential configuration", err.Error())
		return
	}
	cred, err := r.client.CreateCredential(ctx, input)
	if err != nil {
		resp.Diagnostics.AddError("Error creating credential", err.Error())
		return
	}

	// Map response to model but preserve credential_data from plan (API returns masked values).
	configuredExtraData := plan.ExtraData
	if err := mapCredentialResponseToModel(cred, &plan); err != nil {
		resp.Diagnostics.AddError("Error decoding credential", err.Error())
		return
	}
	plan.ExtraData = configuredExtraData
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

	secretKeys, err := r.credentialSecretKeys(ctx, cred.CredentialType)
	if err != nil {
		resp.Diagnostics.AddError("Error reading credential type schema", err.Error())
		return
	}
	state.CredentialData, err = reconcileCredentialData(state.CredentialData, cred.CredentialData, secretKeys)
	if err != nil {
		resp.Diagnostics.AddError("Error decoding credential_data", err.Error())
		return
	}
	state.UserCredentialData, err = reconcileCredentialData(state.UserCredentialData, cred.UserCredentialData, secretKeys)
	if err != nil {
		resp.Diagnostics.AddError("Error decoding user_credential_data", err.Error())
		return
	}
	savedExtraData := state.ExtraData
	extraDataUnchanged, err := dynamicJSONEqual(savedExtraData, cred.ExtraData)
	if err != nil {
		resp.Diagnostics.AddError("Error comparing credential extra_data", err.Error())
		return
	}
	if err := mapCredentialResponseToModel(cred, &state); err != nil {
		resp.Diagnostics.AddError("Error decoding credential", err.Error())
		return
	}
	if extraDataUnchanged {
		state.ExtraData = savedExtraData
	}
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

	input, err := credentialUpdateInput(&plan, state)
	if err != nil {
		resp.Diagnostics.AddError("Invalid credential update", err.Error())
		return
	}
	cred, err := r.client.UpdateCredential(ctx, state.ID.ValueInt64(), input)
	if err != nil {
		resp.Diagnostics.AddError("Error updating credential", err.Error())
		return
	}

	// Preserve credential_data from plan (API response has masked values).
	configuredExtraData := plan.ExtraData
	if err := mapCredentialResponseToModel(cred, &plan); err != nil {
		resp.Diagnostics.AddError("Error decoding credential", err.Error())
		return
	}
	plan.ExtraData = configuredExtraData
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
func mapCredentialResponseToModel(cred *client.CredentialResponse, m *CredentialResourceModel) error {
	m.ID = types.Int64Value(cred.ID)
	m.Name = types.StringValue(cred.Name)
	m.CredentialType = types.StringValue(cred.CredentialType)
	// credential_data is intentionally preserved from the plan/state — API returns ****
	m.Description = stringPtrToValue(cred.Description)
	m.RequiresUserMapping = types.BoolValue(cred.RequiresUserMapping)
	m.Source = types.StringValue(credentialResponseSource(cred.Source))
	m.UserSource = types.StringValue(credentialResponseSource(cred.UserSource))
	m.ExternalVaultID = int64PtrToValue(cred.ExternalVaultID)
	m.UserExternalVaultID = int64PtrToValue(cred.UserExternalVaultID)
	m.ExternalVaultName = stringPtrToValue(cred.ExternalVaultName)
	m.ExternalVaultType = stringPtrToValue(cred.ExternalVaultType)
	m.UserExternalVaultName = stringPtrToValue(cred.UserExternalVaultName)
	m.UserExternalVaultType = stringPtrToValue(cred.UserExternalVaultType)
	var err error
	m.ExternalVaultRef, err = credentialrefs.FromJSON(cred.ExternalVaultRef, m.ExternalVaultRef)
	if err != nil {
		return fmt.Errorf("external_vault_ref: %w", err)
	}
	m.UserExternalVaultRef, err = credentialrefs.FromJSON(cred.UserExternalVaultRef, m.UserExternalVaultRef)
	if err != nil {
		return fmt.Errorf("user_external_vault_ref: %w", err)
	}

	m.ExtraData, err = rawJSONToDynamic(cred.ExtraData)
	if err != nil {
		return fmt.Errorf("extra_data: %w", err)
	}
	m.CreatedAt = types.StringValue(cred.CreatedAt)
	m.UpdatedAt = types.StringValue(cred.UpdatedAt)
	return nil
}

func credentialResponseSource(source string) string {
	if source == "" {
		return "local"
	}
	return source
}

func (r *CredentialResource) credentialSecretKeys(ctx context.Context, typeID string) ([]string, error) {
	catalog, err := r.client.ListCredentialTypes(ctx)
	if err != nil {
		return nil, err
	}
	for _, item := range catalog {
		if item.Type != typeID {
			continue
		}
		var schema struct {
			Properties map[string]struct {
				Secret bool `json:"x-secret"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(item.Schema, &schema); err != nil {
			return nil, err
		}
		var keys []string
		for name, property := range schema.Properties {
			if property.Secret {
				keys = append(keys, name)
			}
		}
		return keys, nil
	}
	return nil, fmt.Errorf("credential type %q is not installed in MisterShell", typeID)
}

func reconcileCredentialData(prior types.Dynamic, raw json.RawMessage, secrets []string) (types.Dynamic, error) {
	// Empty literal dictionaries represent omitted optional data for all-mapped
	// external credentials and per-user local fills.
	var fields map[string]json.RawMessage
	if prior.IsNull() && json.Unmarshal(raw, &fields) == nil && len(fields) == 0 {
		return prior, nil
	}
	return tfjson.Reconcile(prior, raw, secrets)
}
