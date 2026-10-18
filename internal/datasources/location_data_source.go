package datasources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-mistershell/internal/client"
)

var _ datasource.DataSource = &LocationDataSource{}

type LocationDataSource struct {
	client *client.Client
}

type LocationDataSourceModel struct {
	ID            types.Int64   `tfsdk:"id"`
	Name          types.String  `tfsdk:"name"`
	Kind          types.String  `tfsdk:"kind"`
	Description   types.String  `tfsdk:"description"`
	ParentID      types.Int64   `tfsdk:"parent_id"`
	Latitude      types.Float64 `tfsdk:"latitude"`
	Longitude     types.Float64 `tfsdk:"longitude"`
	ExtraData     types.Dynamic `tfsdk:"extra_data"`
	ResourceCount types.Int64   `tfsdk:"resource_count"`
	Path          types.String  `tfsdk:"path"`
	CreatedAt     types.String  `tfsdk:"created_at"`
	UpdatedAt     types.String  `tfsdk:"updated_at"`
}

func NewLocationDataSource() datasource.DataSource {
	return &LocationDataSource{}
}

func (d *LocationDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_location"
}

func (d *LocationDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a MisterShell location. Look up by id for a direct fetch, or use name/kind/parent_id to search. Search filters must match exactly one location.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Description: "Location ID. Use for direct lookup.",
				Optional:    true,
				Computed:    true,
			},
			"name": schema.StringAttribute{
				Description: "Location name. Used as a search filter when id is not specified.",
				Optional:    true,
				Computed:    true,
			},
			"kind": schema.StringAttribute{
				Description: "Location type filter: 'geo' or 'org'.",
				Optional:    true,
				Computed:    true,
				Validators: []validator.String{
					stringvalidator.OneOf("geo", "org"),
				},
			},
			"description": schema.StringAttribute{
				Description: "Location description.",
				Computed:    true,
			},
			"parent_id": schema.Int64Attribute{
				Description: "Parent location ID. Used as a search filter when id is not specified.",
				Optional:    true,
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
			"extra_data": schema.DynamicAttribute{
				Description: "Additional metadata as a native Terraform value.",
				Computed:    true,
			},
			"resource_count": schema.Int64Attribute{Description: "Number of resources directly assigned to this location.", Computed: true},
			"path":           schema.StringAttribute{Description: "Resolved hierarchical location path.", Computed: true},
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

	hasID := !config.ID.IsNull() && !config.ID.IsUnknown()

	var loc *client.LocationResponse

	if hasID {
		// Direct lookup by ID
		var err error
		loc, err = d.client.GetLocation(ctx, config.ID.ValueInt64())
		if err != nil {
			resp.Diagnostics.AddError("Error reading location", err.Error())
			return
		}
	} else {
		// Search with filters
		hasName := !config.Name.IsNull() && !config.Name.IsUnknown()
		hasKind := !config.Kind.IsNull() && !config.Kind.IsUnknown()
		hasParentID := !config.ParentID.IsNull() && !config.ParentID.IsUnknown()

		if !hasName && !hasKind && !hasParentID {
			resp.Diagnostics.AddError("Missing lookup criteria", "Specify 'id' for direct lookup, or at least one of 'name', 'kind', 'parent_id' to search.")
			return
		}

		filter := client.LocationListFilter{}
		if hasName {
			filter.Search = client.SearchPrefix(config.Name.ValueString())
		}
		if hasParentID {
			v := config.ParentID.ValueInt64()
			filter.ParentID = &v
		}

		locs, err := d.client.ListLocations(ctx, filter)
		if err != nil {
			resp.Diagnostics.AddError("Error searching locations", err.Error())
			return
		}

		name, kind := "", ""
		if hasName {
			name = config.Name.ValueString()
		}
		if hasKind {
			kind = config.Kind.ValueString()
		}
		locs = filterLocationsExact(locs, name, kind)

		if len(locs) == 0 {
			resp.Diagnostics.AddError("No matching location found", "No location matches the specified criteria.")
			return
		}
		if len(locs) > 1 {
			resp.Diagnostics.AddError("Multiple locations found",
				fmt.Sprintf("Found %d locations matching the criteria. Add more filters to narrow to exactly one.", len(locs)))
			return
		}
		loc = &locs[0]
	}

	config.ID = types.Int64Value(loc.ID)
	config.Name = types.StringValue(loc.Name)
	config.Kind = types.StringValue(loc.Kind)
	config.Description = stringPtrToValue(loc.Description)
	config.ParentID = int64PtrToValue(loc.ParentID)
	config.Latitude = float64PtrToValue(loc.Latitude)
	config.Longitude = float64PtrToValue(loc.Longitude)
	extraData, err := rawJSONToDynamic(loc.ExtraData)
	if err != nil {
		resp.Diagnostics.AddError("Error decoding location extra_data", err.Error())
		return
	}
	config.ExtraData = extraData
	config.ResourceCount = int64PtrToValue(loc.ResourceCount)
	config.Path = stringPtrToValue(loc.Path)
	config.CreatedAt = types.StringValue(loc.CreatedAt)
	config.UpdatedAt = types.StringValue(loc.UpdatedAt)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

func filterLocationsExact(locations []client.LocationResponse, name, kind string) []client.LocationResponse {
	filtered := make([]client.LocationResponse, 0, len(locations))
	for _, location := range locations {
		if name != "" && location.Name != name {
			continue
		}
		if kind != "" && location.Kind != kind {
			continue
		}
		filtered = append(filtered, location)
	}
	return filtered
}
