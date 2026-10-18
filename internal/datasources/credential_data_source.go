package datasources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-mistershell/internal/client"
	"terraform-provider-mistershell/internal/credentialrefs"
)

var _ datasource.DataSource = &CredentialDataSource{}

type CredentialDataSource struct {
	client *client.Client
}

type CredentialDataSourceModel struct {
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
	Description         types.String  `tfsdk:"description"`
	RequiresUserMapping types.Bool    `tfsdk:"requires_user_mapping"`
	CredentialData      types.Dynamic `tfsdk:"credential_data"`
	ExtraData           types.Dynamic `tfsdk:"extra_data"`
	CreatedAt           types.String  `tfsdk:"created_at"`
	UpdatedAt           types.String  `tfsdk:"updated_at"`
}

func NewCredentialDataSource() datasource.DataSource {
	return &CredentialDataSource{}
}

func (d *CredentialDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_credential"
}

func (d *CredentialDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a MisterShell credential. Look up by id for a direct fetch, or use name/credential_type to search. Search filters must match exactly one credential. Note: secret fields in credential_data are masked as '****' by the API.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Description: "Credential ID. Use for direct lookup.",
				Optional:    true,
				Computed:    true,
			},
			"name": schema.StringAttribute{
				Description: "Credential name (unique). Used as a search filter when id is not specified.",
				Optional:    true,
				Computed:    true,
			},
			"credential_type": schema.StringAttribute{
				Description: "Installed credential type filter. Query MisterShell's runtime credential catalog for current values.",
				Optional:    true,
				Computed:    true,
			},
			"description": schema.StringAttribute{
				Description: "Credential description.",
				Computed:    true,
			},
			"requires_user_mapping": schema.BoolAttribute{
				Description: "Whether interactive sessions use the independent user credential source.",
				Computed:    true,
			},
			"credential_data": schema.DynamicAttribute{
				Description: "Credential data as a native Terraform object. Secret fields are masked as '****' by the API.",
				Computed:    true,
				Sensitive:   true,
			},
			"extra_data": schema.DynamicAttribute{
				Description: "Non-sensitive metadata as a native Terraform value.",
				Computed:    true,
			},
			"created_at": schema.StringAttribute{
				Description: "Creation timestamp.",
				Computed:    true,
			},
			"updated_at": schema.StringAttribute{
				Description: "Last update timestamp.",
				Computed:    true,
			},
		},
	}
	for _, prefix := range []string{"", "user_"} {
		resp.Schema.Attributes[prefix+"source"] = schema.StringAttribute{Computed: true, Description: "Credential source: local or external."}
		resp.Schema.Attributes[prefix+"external_vault_id"] = schema.Int64Attribute{Computed: true, Description: "Referenced external vault ID."}
		resp.Schema.Attributes[prefix+"external_vault_ref"] = schema.SingleNestedAttribute{Computed: true, Description: "Unresolved vault reference metadata.", Attributes: map[string]schema.Attribute{
			"base":   schema.DynamicAttribute{Computed: true, Description: "Vault-specific reference parameters."},
			"fields": schema.MapAttribute{Computed: true, ElementType: types.StringType, Description: "Credential fields mapped to vault keys."},
		}}
		resp.Schema.Attributes[prefix+"external_vault_name"] = schema.StringAttribute{Computed: true, Description: "Referenced vault name."}
		resp.Schema.Attributes[prefix+"external_vault_type"] = schema.StringAttribute{Computed: true, Description: "Referenced vault type."}
	}
	resp.Schema.Attributes["user_credential_data"] = schema.DynamicAttribute{Computed: true, Sensitive: true, Description: "Masked user literal fields. Secrets are never resolved by Terraform."}

}

func (d *CredentialDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected DataSource Configure Type", "Expected *client.Client")
		return
	}
	d.client = c
}

func (d *CredentialDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config CredentialDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	hasID := !config.ID.IsNull() && !config.ID.IsUnknown()

	var cred *client.CredentialResponse

	if hasID {
		var err error
		cred, err = d.client.GetCredential(ctx, config.ID.ValueInt64())
		if err != nil {
			resp.Diagnostics.AddError("Error reading credential", err.Error())
			return
		}
	} else {
		hasName := !config.Name.IsNull() && !config.Name.IsUnknown()
		hasType := !config.CredentialType.IsNull() && !config.CredentialType.IsUnknown()

		if !hasName && !hasType {
			resp.Diagnostics.AddError("Missing lookup criteria", "Specify 'id' for direct lookup, or at least one of 'name', 'credential_type' to search.")
			return
		}

		filter := client.CredentialListFilter{}
		if hasName {
			filter.Search = client.SearchPrefix(config.Name.ValueString())
		}
		if hasType {
			filter.CredentialType = config.CredentialType.ValueString()
		}

		creds, err := d.client.ListCredentials(ctx, filter)
		if err != nil {
			resp.Diagnostics.AddError("Error searching credentials", err.Error())
			return
		}

		// Apply exact name match if name was specified (API search is fuzzy)
		if hasName {
			name := config.Name.ValueString()
			var filtered []client.CredentialResponse
			for _, c := range creds {
				if c.Name == name {
					filtered = append(filtered, c)
				}
			}
			creds = filtered
		}

		if len(creds) == 0 {
			resp.Diagnostics.AddError("No matching credential found", "No credential matches the specified criteria.")
			return
		}
		if len(creds) > 1 {
			resp.Diagnostics.AddError("Multiple credentials found",
				fmt.Sprintf("Found %d credentials matching the criteria. Add more filters to narrow to exactly one.", len(creds)))
			return
		}
		// List returns the slim list shape; re-fetch the full object by id so
		// name lookups match id lookups (see auth_provider for the reference pattern).
		full, err := d.client.GetCredential(ctx, creds[0].ID)
		if err != nil {
			resp.Diagnostics.AddError("Error reading credential", err.Error())
			return
		}
		cred = full
	}

	config.ID = types.Int64Value(cred.ID)
	config.Name = types.StringValue(cred.Name)
	config.CredentialType = types.StringValue(cred.CredentialType)
	config.Description = stringPtrToValue(cred.Description)
	config.RequiresUserMapping = types.BoolValue(cred.RequiresUserMapping)
	credentialData, err := rawJSONToDynamic(cred.CredentialData)
	if err != nil {
		resp.Diagnostics.AddError("Error decoding credential_data", err.Error())
		return
	}
	config.CredentialData = credentialData
	config.Source = types.StringValue(cred.Source)
	if cred.Source == "" {
		config.Source = types.StringValue("local")
	}
	config.UserSource = types.StringValue(cred.UserSource)
	if cred.UserSource == "" {
		config.UserSource = types.StringValue("local")
	}
	config.ExternalVaultID = int64PtrToValue(cred.ExternalVaultID)
	config.UserExternalVaultID = int64PtrToValue(cred.UserExternalVaultID)
	config.ExternalVaultName = stringPtrToValue(cred.ExternalVaultName)
	config.ExternalVaultType = stringPtrToValue(cred.ExternalVaultType)
	config.UserExternalVaultName = stringPtrToValue(cred.UserExternalVaultName)
	config.UserExternalVaultType = stringPtrToValue(cred.UserExternalVaultType)
	config.ExternalVaultRef, err = credentialrefs.FromJSON(cred.ExternalVaultRef, config.ExternalVaultRef)
	if err != nil {
		resp.Diagnostics.AddError("Error decoding external_vault_ref", err.Error())
		return
	}
	config.UserExternalVaultRef, err = credentialrefs.FromJSON(cred.UserExternalVaultRef, config.UserExternalVaultRef)
	if err != nil {
		resp.Diagnostics.AddError("Error decoding user_external_vault_ref", err.Error())
		return
	}
	config.UserCredentialData, err = rawJSONToDynamic(cred.UserCredentialData)
	if err != nil {
		resp.Diagnostics.AddError("Error decoding user_credential_data", err.Error())
		return
	}

	extraData, err := rawJSONToDynamic(cred.ExtraData)
	if err != nil {
		resp.Diagnostics.AddError("Error decoding credential extra_data", err.Error())
		return
	}
	config.ExtraData = extraData
	config.CreatedAt = types.StringValue(cred.CreatedAt)
	config.UpdatedAt = types.StringValue(cred.UpdatedAt)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
