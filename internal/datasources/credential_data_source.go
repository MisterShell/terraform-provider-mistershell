package datasources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-mistershell/internal/client"
)

var _ datasource.DataSource = &CredentialDataSource{}

type CredentialDataSource struct {
	client *client.Client
}

type CredentialDataSourceModel struct {
	ID                  types.Int64          `tfsdk:"id"`
	Name                types.String         `tfsdk:"name"`
	CredentialType      types.String         `tfsdk:"credential_type"`
	Description         types.String         `tfsdk:"description"`
	RequiresUserMapping types.Bool           `tfsdk:"requires_user_mapping"`
	CredentialData      jsontypes.Normalized `tfsdk:"credential_data"`
	ExtraData           jsontypes.Normalized `tfsdk:"extra_data"`
	CreatedAt           types.String         `tfsdk:"created_at"`
	UpdatedAt           types.String         `tfsdk:"updated_at"`
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
				Description: "Credential type filter (ssh_password, ssh_key, aws_credentials, azure_service_principal, kubeconfig, rdp_password, db_password).",
				Optional:    true,
				Computed:    true,
			},
			"description": schema.StringAttribute{
				Description: "Credential description.",
				Computed:    true,
			},
			"requires_user_mapping": schema.BoolAttribute{
				Description: "Whether users must fill their own credential copy.",
				Computed:    true,
			},
			"credential_data": schema.StringAttribute{
				Description: "Credential data as JSON. Secret fields are masked as '****' by the API.",
				Computed:    true,
				Sensitive:   true,
				CustomType:  jsontypes.NormalizedType{},
			},
			"extra_data": schema.StringAttribute{
				Description: "Non-sensitive metadata as JSON.",
				Computed:    true,
				CustomType:  jsontypes.NormalizedType{},
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
			filter.Search = config.Name.ValueString()
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
		cred = &creds[0]
	}

	config.ID = types.Int64Value(cred.ID)
	config.Name = types.StringValue(cred.Name)
	config.CredentialType = types.StringValue(cred.CredentialType)
	config.Description = stringPtrToValue(cred.Description)
	config.RequiresUserMapping = types.BoolValue(cred.RequiresUserMapping)
	config.CredentialData = rawJSONToNormalized(cred.CredentialData)
	config.ExtraData = rawJSONToNormalized(cred.ExtraData)
	config.CreatedAt = types.StringValue(cred.CreatedAt)
	config.UpdatedAt = types.StringValue(cred.UpdatedAt)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
