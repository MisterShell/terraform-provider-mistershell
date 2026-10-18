package datasources

import (
	"context"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"terraform-provider-mistershell/internal/client"
)

var _ datasource.DataSource = &ExternalVaultDataSource{}

type ExternalVaultDataSource struct{ client *client.Client }
type ExternalVaultDataSourceModel struct {
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

func NewExternalVaultDataSource() datasource.DataSource { return &ExternalVaultDataSource{} }
func (d *ExternalVaultDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_external_vault"
}
func (d *ExternalVaultDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{Description: "Reads an external vault by ID or exact name/mnemonic. Exposes masked configuration and metadata only; no checks, authentication or secret resolution are performed.", Attributes: map[string]schema.Attribute{
		"id":                schema.Int64Attribute{Computed: true, Description: "External vault ID for direct lookup.", Optional: true},
		"name":              schema.StringAttribute{Computed: true, Description: "Exact vault name. When combined with another selector, all must match.", Optional: true},
		"mnemonic":          schema.StringAttribute{Computed: true, Description: "Exact immutable vault mnemonic.", Optional: true},
		"description":       schema.StringAttribute{Computed: true, Description: "Vault description."},
		"vault_type":        schema.StringAttribute{Computed: true, Description: "Vault provider type."},
		"is_enabled":        schema.BoolAttribute{Computed: true, Description: "Whether the vault is enabled."},
		"cache_ttl_seconds": schema.Int64Attribute{Computed: true, Description: "Secret cache lifetime in seconds."},
		"last_check_at":     schema.StringAttribute{Computed: true, Description: "Last check timestamp; Terraform never initiates checks."},
		"last_check_status": schema.StringAttribute{Computed: true, Description: "Last check status."},
		"last_check_error":  schema.StringAttribute{Computed: true, Description: "Last check error."},
		"credential_count":  schema.Int64Attribute{Computed: true, Description: "Number of credentials referencing this vault."},
		"created_at":        schema.StringAttribute{Computed: true, Description: "Creation timestamp."},
		"updated_at":        schema.StringAttribute{Computed: true, Description: "Last update timestamp."},
		"config":            schema.DynamicAttribute{Computed: true, Sensitive: true, Description: "Native vault configuration with secret values masked by the API."},
	}}
}
func (d *ExternalVaultDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *ExternalVaultDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var m ExternalVaultDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	hasID := !m.ID.IsNull() && !m.ID.IsUnknown()
	hasName := !m.Name.IsNull() && !m.Name.IsUnknown()
	hasMnemonic := !m.Mnemonic.IsNull() && !m.Mnemonic.IsUnknown()
	if !hasID && !hasName && !hasMnemonic {
		resp.Diagnostics.AddError("Missing lookup criteria", "Specify id, exact name or exact mnemonic.")
		return
	}
	var v *client.ExternalVaultResponse
	var err error
	if hasID {
		if m.ID.ValueInt64() <= 0 {
			resp.Diagnostics.AddError("Invalid vault ID", "id must be positive.")
			return
		}
		v, err = d.client.GetExternalVault(ctx, m.ID.ValueInt64())
	} else {
		v, err = d.client.FindExternalVault(ctx, m.Name.ValueString(), m.Mnemonic.ValueString())
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading external vault", err.Error())
		return
	}
	if (hasName && v.Name != m.Name.ValueString()) || (hasMnemonic && v.Mnemonic != m.Mnemonic.ValueString()) {
		resp.Diagnostics.AddError("Lookup criteria mismatch", "The external vault does not match all specified lookup criteria.")
		return
	}
	config, err := rawJSONToDynamic(v.Config)
	if err != nil {
		resp.Diagnostics.AddError("Error decoding vault config", err.Error())
		return
	}
	m.Config = config
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
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
