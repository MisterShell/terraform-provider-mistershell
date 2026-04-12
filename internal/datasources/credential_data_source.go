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
		Description: "Reads a MisterShell credential by ID or by unique name. Note: secret fields in credential_data are masked as '****' by the API.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Description: "Credential ID. Specify either id or name.",
				Optional:    true,
				Computed:    true,
			},
			"name": schema.StringAttribute{
				Description: "Credential name (unique). Specify either id or name.",
				Optional:    true,
				Computed:    true,
			},
			"credential_type": schema.StringAttribute{
				Description: "Credential type.",
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
	hasName := !config.Name.IsNull() && !config.Name.IsUnknown()

	if !hasID && !hasName {
		resp.Diagnostics.AddError("Missing lookup key", "Specify either 'id' or 'name' to look up a credential.")
		return
	}
	if hasID && hasName {
		resp.Diagnostics.AddError("Ambiguous lookup", "Specify either 'id' or 'name', not both.")
		return
	}

	var cred *client.CredentialResponse
	var err error

	if hasID {
		cred, err = d.client.GetCredential(ctx, config.ID.ValueInt64())
	} else {
		// Look up by name: search and find exact match.
		name := config.Name.ValueString()
		creds, searchErr := d.client.ListCredentials(ctx, name)
		if searchErr != nil {
			resp.Diagnostics.AddError("Error searching credentials", searchErr.Error())
			return
		}
		for i := range creds {
			if creds[i].Name == name {
				cred = &creds[i]
				break
			}
		}
		if cred == nil {
			err = fmt.Errorf("credential with name %q not found", name)
		}
	}

	if err != nil {
		resp.Diagnostics.AddError("Error reading credential", err.Error())
		return
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
