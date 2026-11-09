package canaryextras

import (
	"context"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"terraform-provider-mistershell/internal/client"
	"terraform-provider-mistershell/internal/tfjson"
)

type CatalogDataSource struct{ client *client.Client }
type CatalogModel struct {
	ID      types.String  `tfsdk:"id"`
	Catalog types.Dynamic `tfsdk:"catalog"`
}

func NewCanaryCatalogDataSource() datasource.DataSource { return &CatalogDataSource{} }
func (d *CatalogDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_canary_catalog"
}
func (d *CatalogDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{Description: "Reads the Canary service catalog, including version, service definitions, settings schemas and prerequisites. Requires MisterShell 2.1 or newer.", Attributes: map[string]schema.Attribute{"id": schema.StringAttribute{Computed: true, Description: "Catalog identity."}, "catalog": schema.DynamicAttribute{Computed: true, Description: "Native catalog object containing schema_version, catalog_version, services, settings_schemas and prerequisites."}}}
}
func (d *CatalogDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	var ok bool
	d.client, ok = req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Invalid provider data", "Expected MisterShell API client.")
	}
}
func (d *CatalogDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	raw, err := d.client.ConfigurationRequest(ctx, "GET", "/api/v1/canaries/catalog", nil)
	if err != nil {
		resp.Diagnostics.AddError("Cannot read Canary catalog", err.Error())
		return
	}
	v, err := tfjson.FromRawJSON(raw)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Canary catalog", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, CatalogModel{ID: types.StringValue("catalog"), Catalog: v})...)
}
