package datasources

import (
	"context"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"terraform-provider-mistershell/internal/client"
)

type TypeCatalogDataSource struct {
	name, path string
	client     *client.Client
}
type TypeCatalogModel struct {
	ID    types.String  `tfsdk:"id"`
	Types types.Dynamic `tfsdk:"types"`
}

func NewCredentialTypesDataSource() datasource.DataSource {
	return &TypeCatalogDataSource{name: "credential_types", path: "/api/v1/credentials/types"}
}
func NewExternalVaultTypesDataSource() datasource.DataSource {
	return &TypeCatalogDataSource{name: "external_vault_types", path: "/api/v1/external-vaults/types"}
}
func (d *TypeCatalogDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + d.name
}
func (d *TypeCatalogDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{Description: "Reads installed MisterShell type definitions and their configuration/reference JSON schemas.", Attributes: map[string]schema.Attribute{"id": schema.StringAttribute{Computed: true, Description: "Catalog identity."}, "types": schema.DynamicAttribute{Computed: true, Description: "Native list of installed type definitions. Credential entries include type, display_name, description and schema. Vault entries include vault_type, display_name, config_schema, base_ref_schema, secret_config_keys, supports_subject_identity and field_ref_hint."}}}
}
func (d *TypeCatalogDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	var ok bool
	d.client, ok = req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Invalid provider data", "Expected MisterShell API client.")
	}
}
func (d *TypeCatalogDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	raw, err := d.client.ConfigurationRequest(ctx, "GET", d.path, nil)
	if err != nil {
		resp.Diagnostics.AddError("Error reading type catalog", err.Error())
		return
	}
	value, err := rawJSONToDynamic(raw)
	if err != nil {
		resp.Diagnostics.AddError("Error decoding type catalog", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, TypeCatalogModel{ID: types.StringValue("catalog"), Types: value})...)
}
