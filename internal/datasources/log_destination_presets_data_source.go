package datasources

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/attr"
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
	ID      types.String `tfsdk:"id"`
	Presets types.List   `tfsdk:"presets"`
}

// logDestinationPresetObjectType is the object element type for the presets list.
var logDestinationPresetObjectType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"key":            types.StringType,
		"label":          types.StringType,
		"vendor":         types.StringType,
		"type":           types.StringType,
		"default_config": jsontypes.NormalizedType{},
	},
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
			"presets": schema.ListNestedAttribute{
				Description: "List of available log-destination presets.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"key": schema.StringAttribute{
							Description: "Preset key (e.g. custom_syslog, custom_webhook, splunk_hec, elastic, datadog).",
							Computed:    true,
						},
						"label": schema.StringAttribute{
							Description: "Human-readable preset label.",
							Computed:    true,
						},
						"vendor": schema.StringAttribute{
							Description: "Vendor name.",
							Computed:    true,
						},
						"type": schema.StringAttribute{
							Description: "Destination type (syslog or webhook).",
							Computed:    true,
						},
						"default_config": schema.StringAttribute{
							Description: "Ready-to-edit config template as JSON. Decode with jsondecode() or copy into a mistershell_log_destination config.",
							Computed:    true,
							CustomType:  jsontypes.NormalizedType{},
						},
					},
				},
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

	elems := make([]attr.Value, 0, len(presets))
	for _, p := range presets {
		obj, diags := types.ObjectValue(logDestinationPresetObjectType.AttrTypes, map[string]attr.Value{
			"key":            types.StringValue(p.Key),
			"label":          types.StringValue(p.Label),
			"vendor":         types.StringValue(p.Vendor),
			"type":           types.StringValue(p.Type),
			"default_config": rawJSONToNormalized(p.DefaultConfig),
		})
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		elems = append(elems, obj)
	}

	list, diags := types.ListValue(logDestinationPresetObjectType, elems)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	config.ID = types.StringValue("log_destination_presets")
	config.Presets = list

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
