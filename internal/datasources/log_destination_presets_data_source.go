package datasources

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-mistershell/internal/client"
)

var _ datasource.DataSource = &LogDestinationPresetsDataSource{}

type LogDestinationPresetsDataSource struct {
	client *client.Client
}

type LogDestinationPresetsDataSourceModel struct {
	ID      types.String  `tfsdk:"id"`
	Presets types.Dynamic `tfsdk:"presets"`
}

func NewLogDestinationPresetsDataSource() datasource.DataSource {
	return &LogDestinationPresetsDataSource{}
}

func (d *LogDestinationPresetsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_log_destination_presets"
}

func (d *LogDestinationPresetsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads the MisterShell log-destination presets (read-only). Presets are code-owned per-type/vendor config templates (key, label, vendor, type, default_config). Copy a preset's default_config into a mistershell_log_destination's config to author a destination quickly.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Synthetic identifier for this data source.",
				Computed:    true,
			},
			"presets": schema.DynamicAttribute{
				Description: "Native Terraform sequence of log-destination preset objects. Each object contains key, label, vendor, type, and a native default_config object.",
				Computed:    true,
			},
		},
	}
}

func (d *LogDestinationPresetsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *LogDestinationPresetsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config LogDestinationPresetsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	presets, err := d.client.ListLogDestinationPresets(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading log destination presets", err.Error())
		return
	}

	nativePresets, err := logDestinationPresetsToDynamic(presets)
	if err != nil {
		resp.Diagnostics.AddError("Error decoding log destination presets", err.Error())
		return
	}

	config.ID = types.StringValue("log_destination_presets")
	config.Presets = nativePresets

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

func logDestinationPresetsToDynamic(presets []client.LogDestinationPreset) (types.Dynamic, error) {
	if presets == nil {
		presets = []client.LogDestinationPreset{}
	}
	rawPresets, err := json.Marshal(presets)
	if err != nil {
		return types.DynamicNull(), err
	}
	return rawJSONToDynamic(rawPresets)
}
