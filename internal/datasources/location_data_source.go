package datasources

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-mistershell/internal/client"
)

var _ datasource.DataSource = &LocationDataSource{}

type LocationDataSource struct {
	client *client.Client
}

type LocationDataSourceModel struct {
	ID          types.Int64          `tfsdk:"id"`
	Name        types.String         `tfsdk:"name"`
	Kind        types.String         `tfsdk:"kind"`
	Description types.String         `tfsdk:"description"`
	ParentID    types.Int64          `tfsdk:"parent_id"`
	Latitude    types.Float64        `tfsdk:"latitude"`
	Longitude   types.Float64        `tfsdk:"longitude"`
	ExtraData   jsontypes.Normalized `tfsdk:"extra_data"`
	CreatedAt   types.String         `tfsdk:"created_at"`
	UpdatedAt   types.String         `tfsdk:"updated_at"`
}

func NewLocationDataSource() datasource.DataSource {
	return &LocationDataSource{}
}

func (d *LocationDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_location"
}

func (d *LocationDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a MisterShell location by ID.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Description: "Location ID to look up.",
				Required:    true,
			},
			"name": schema.StringAttribute{
				Description: "Location name.",
				Computed:    true,
			},
			"kind": schema.StringAttribute{
				Description: "Location type (geo or org).",
				Computed:    true,
			},
			"description": schema.StringAttribute{
				Description: "Location description.",
				Computed:    true,
			},
			"parent_id": schema.Int64Attribute{
				Description: "Parent location ID.",
				Computed:    true,
			},
			"latitude": schema.Float64Attribute{
				Description: "Geographic latitude.",
				Computed:    true,
			},
			"longitude": schema.Float64Attribute{
				Description: "Geographic longitude.",
				Computed:    true,
			},
			"extra_data": schema.StringAttribute{
				Description: "Additional metadata as JSON.",
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

func (d *LocationDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *LocationDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config LocationDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	loc, err := d.client.GetLocation(ctx, config.ID.ValueInt64())
	if err != nil {
		resp.Diagnostics.AddError("Error reading location", err.Error())
		return
	}

	config.Name = types.StringValue(loc.Name)
	config.Kind = types.StringValue(loc.Kind)
	config.Description = stringPtrToValue(loc.Description)
	config.ParentID = int64PtrToValue(loc.ParentID)
	config.Latitude = float64PtrToValue(loc.Latitude)
	config.Longitude = float64PtrToValue(loc.Longitude)
	config.ExtraData = rawJSONToNormalized(loc.ExtraData)
	config.CreatedAt = types.StringValue(loc.CreatedAt)
	config.UpdatedAt = types.StringValue(loc.UpdatedAt)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
