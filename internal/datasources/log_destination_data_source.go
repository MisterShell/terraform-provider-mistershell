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

var _ datasource.DataSource = &LogDestinationDataSource{}

type LogDestinationDataSource struct {
	client *client.Client
}

type LogDestinationDataSourceModel struct {
	ID          types.Int64          `tfsdk:"id"`
	Name        types.String         `tfsdk:"name"`
	Enabled     types.Bool           `tfsdk:"enabled"`
	Type        types.String         `tfsdk:"type"`
	Streams     types.Set            `tfsdk:"streams"`
	MinSeverity types.String         `tfsdk:"min_severity"`
	Config      jsontypes.Normalized `tfsdk:"config"`
	CreatedAt   types.String         `tfsdk:"created_at"`
	UpdatedAt   types.String         `tfsdk:"updated_at"`
}

func NewLogDestinationDataSource() datasource.DataSource {
	return &LogDestinationDataSource{}
}

func (d *LogDestinationDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_log_destination"
}

func (d *LogDestinationDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a MisterShell log destination. Look up by id for a direct fetch, or by name (exact match). Note: webhook auth secrets in config are masked as '****' by the API.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Description: "Log destination ID. Use for direct lookup.",
				Optional:    true,
				Computed:    true,
			},
			"name": schema.StringAttribute{
				Description: "Log destination name. Used as an exact-match search filter when id is not specified.",
				Optional:    true,
				Computed:    true,
			},
			"enabled": schema.BoolAttribute{
				Description: "Whether the destination is enabled.",
				Computed:    true,
			},
			"type": schema.StringAttribute{
				Description: "Destination type (syslog or webhook).",
				Computed:    true,
			},
			"streams": schema.SetAttribute{
				Description: "Log streams forwarded (security, policy, api, app).",
				Computed:    true,
				ElementType: types.StringType,
			},
			"min_severity": schema.StringAttribute{
				Description: "Minimum severity forwarded.",
				Computed:    true,
			},
			"config": schema.StringAttribute{
				Description: "Per-type config as JSON. Webhook auth secrets are masked as '****' by the API.",
				Computed:    true,
				Sensitive:   true,
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

func (d *LogDestinationDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *LogDestinationDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config LogDestinationDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	hasID := !config.ID.IsNull() && !config.ID.IsUnknown()

	var dest *client.LogDestinationResponse

	if hasID {
		var err error
		dest, err = d.client.GetLogDestination(ctx, config.ID.ValueInt64())
		if err != nil {
			resp.Diagnostics.AddError("Error reading log destination", err.Error())
			return
		}
	} else {
		hasName := !config.Name.IsNull() && !config.Name.IsUnknown()
		if !hasName {
			resp.Diagnostics.AddError("Missing lookup criteria", "Specify 'id' for direct lookup, or 'name' to search.")
			return
		}

		dests, err := d.client.ListLogDestinations(ctx, client.LogDestinationListFilter{Search: config.Name.ValueString()})
		if err != nil {
			resp.Diagnostics.AddError("Error searching log destinations", err.Error())
			return
		}

		name := config.Name.ValueString()
		var filtered []client.LogDestinationResponse
		for _, dst := range dests {
			if dst.Name == name {
				filtered = append(filtered, dst)
			}
		}

		if len(filtered) == 0 {
			resp.Diagnostics.AddError("No matching log destination found", "No log destination matches the specified name.")
			return
		}
		if len(filtered) > 1 {
			resp.Diagnostics.AddError("Multiple log destinations found",
				fmt.Sprintf("Found %d log destinations matching name %q.", len(filtered), name))
			return
		}
		dest = &filtered[0]
	}

	config.ID = types.Int64Value(dest.ID)
	config.Name = types.StringValue(dest.Name)
	config.Enabled = types.BoolValue(dest.Enabled)
	config.Type = types.StringValue(dest.Type)
	config.Streams = stringSliceToSet(dest.Streams)
	config.MinSeverity = types.StringValue(dest.MinSeverity)
	config.Config = rawJSONToNormalized(dest.Config)
	config.CreatedAt = types.StringValue(dest.CreatedAt)
	config.UpdatedAt = types.StringValue(dest.UpdatedAt)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
