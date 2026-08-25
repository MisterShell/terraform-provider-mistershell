package datasources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-mistershell/internal/client"
)

var _ datasource.DataSource = &NetworkResourceDataSource{}

type NetworkResourceDataSource struct {
	client *client.Client
}

type NetworkResourceDataSourceModel struct {
	ID                     types.Int64   `tfsdk:"id"`
	Name                   types.String  `tfsdk:"name"`
	ResourceType           types.String  `tfsdk:"resource_type"`
	EngineID               types.String  `tfsdk:"engine_id"`
	ConnectorID            types.String  `tfsdk:"connector_id"`
	ExternalID             types.String  `tfsdk:"external_id"`
	ConnectorData          types.Dynamic `tfsdk:"connector_data"`
	CredentialID           types.Int64   `tfsdk:"credential_id"`
	LocationID             types.Int64   `tfsdk:"location_id"`
	EffectiveLatitude      types.Float64 `tfsdk:"effective_latitude"`
	EffectiveLongitude     types.Float64 `tfsdk:"effective_longitude"`
	CoordinateSourceID     types.Int64   `tfsdk:"coordinate_source_location_id"`
	CoordinateSourceName   types.String  `tfsdk:"coordinate_source_location_name"`
	Status                 types.String  `tfsdk:"status"`
	IsEnabled              types.Bool    `tfsdk:"is_enabled"`
	Tags                   types.List    `tfsdk:"tags"`
	ExtraData              types.Dynamic `tfsdk:"extra_data"`
	CreatedAt              types.String  `tfsdk:"created_at"`
	UpdatedAt              types.String  `tfsdk:"updated_at"`
	LastConnectivityCheck  types.String  `tfsdk:"last_connectivity_check"`
	LastConnectivityResult types.Dynamic `tfsdk:"last_connectivity_result"`
	LastCollectionAt       types.String  `tfsdk:"last_collection_at"`
	NextCollectionAt       types.String  `tfsdk:"next_collection_at"`
}

func NewNetworkResourceDataSource() datasource.DataSource {
	return &NetworkResourceDataSource{}
}

func (d *NetworkResourceDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_resource"
}

func (d *NetworkResourceDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a MisterShell network resource. Look up by id or search by name/resource_type/location_id/status. The engine is resolved from the target instance's installed type catalog.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Description: "Resource ID. Use for direct lookup.",
				Optional:    true,
				Computed:    true,
			},
			"name": schema.StringAttribute{
				Description: "Resource name. Used as a search filter when id is not specified.",
				Optional:    true,
				Computed:    true,
			},
			"resource_type": schema.StringAttribute{
				Description: "Installed resource type filter. Query MisterShell's runtime resource catalog for current values.",
				Optional:    true,
				Computed:    true,
			},
			"engine_id": schema.StringAttribute{
				Description: "Connection engine inferred from the installed resource-type catalog.",
				Computed:    true,
			},
			"connector_id": schema.StringAttribute{
				Description: "Connector type.",
				Computed:    true,
			},
			"external_id": schema.StringAttribute{
				Description: "External identifier.",
				Computed:    true,
			},
			"connector_data": schema.DynamicAttribute{
				Description: "Connection parameters as a native Terraform object; the shape is defined by the installed type catalog.",
				Computed:    true,
			},
			"credential_id": schema.Int64Attribute{
				Description: "Credential ID.",
				Computed:    true,
			},
			"location_id": schema.Int64Attribute{
				Description: "Location ID filter.",
				Optional:    true,
				Computed:    true,
			},
			"effective_latitude": schema.Float64Attribute{
				Description: "Effective latitude resolved from the resource's location or its nearest ancestor with valid coordinates. Null when no location in that path has valid coordinates.",
				Computed:    true,
			},
			"effective_longitude": schema.Float64Attribute{
				Description: "Effective longitude resolved from the resource's location or its nearest ancestor with valid coordinates. Null when no location in that path has valid coordinates.",
				Computed:    true,
			},
			"coordinate_source_location_id": schema.Int64Attribute{
				Description: "Location ID that supplies the effective coordinates. Null when no effective coordinates exist or the source location is outside the caller's visible location scope.",
				Computed:    true,
			},
			"coordinate_source_location_name": schema.StringAttribute{
				Description: "Location name that supplies the effective coordinates. Null when no effective coordinates exist or the source location is outside the caller's visible location scope.",
				Computed:    true,
			},
			"status": schema.StringAttribute{
				Description: "Connectivity status filter (unknown, verified, unreachable, auth_failed, error, identity_mismatch, snapshot_truncated).",
				Optional:    true,
				Computed:    true,
			},
			"is_enabled": schema.BoolAttribute{
				Description: "Whether the resource is enabled.",
				Computed:    true,
			},
			"tags": schema.ListNestedAttribute{
				Description: "Tags assigned to this resource, as objects with id, name, color, and description.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":          schema.Int64Attribute{Computed: true, Description: "Tag ID."},
						"name":        schema.StringAttribute{Computed: true, Description: "Tag name."},
						"color":       schema.StringAttribute{Computed: true, Description: "Tag color."},
						"description": schema.StringAttribute{Computed: true, Description: "Tag description."},
					},
				},
			},
			"extra_data": schema.DynamicAttribute{
				Description: "Discovered metadata as a native Terraform value.",
				Computed:    true,
			},
			"created_at": schema.StringAttribute{
				Description: "Creation timestamp.",
				Computed:    true,
			},
			"updated_at": schema.StringAttribute{
				Description: "Last update timestamp.",
				Computed:    true,
			},
			"last_connectivity_check": schema.StringAttribute{
				Description: "Timestamp of last connectivity check.",
				Computed:    true,
			},
			"last_connectivity_result": schema.DynamicAttribute{
				Description: "Structured result of the most recent connectivity check (read-only).",
				Computed:    true,
			},
			"last_collection_at": schema.StringAttribute{
				Description: "Timestamp of last data collection.",
				Computed:    true,
			},
			"next_collection_at": schema.StringAttribute{
				Description: "Scheduled time for next data collection.",
				Computed:    true,
			},
		},
	}
}

func (d *NetworkResourceDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *NetworkResourceDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config NetworkResourceDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	hasID := !config.ID.IsNull() && !config.ID.IsUnknown()

	var res *client.NetworkResourceResponse

	if hasID {
		var err error
		res, err = d.client.GetNetworkResource(ctx, config.ID.ValueInt64())
		if err != nil {
			resp.Diagnostics.AddError("Error reading network resource", err.Error())
			return
		}
	} else {
		hasName := !config.Name.IsNull() && !config.Name.IsUnknown()
		hasType := !config.ResourceType.IsNull() && !config.ResourceType.IsUnknown()
		hasLocationID := !config.LocationID.IsNull() && !config.LocationID.IsUnknown()
		hasStatus := !config.Status.IsNull() && !config.Status.IsUnknown()

		if !hasName && !hasType && !hasLocationID && !hasStatus {
			resp.Diagnostics.AddError("Missing lookup criteria",
				"Specify 'id' for direct lookup, or at least one of 'name', 'resource_type', 'location_id', 'status' to search.")
			return
		}

		filter := client.NetworkResourceListFilter{}
		if hasName {
			filter.Search = config.Name.ValueString()
		}
		if hasType {
			filter.ResourceType = config.ResourceType.ValueString()
		}
		if hasLocationID {
			v := config.LocationID.ValueInt64()
			filter.LocationID = &v
		}
		if hasStatus {
			filter.Status = config.Status.ValueString()
		}

		results, err := d.client.ListNetworkResources(ctx, filter)
		if err != nil {
			resp.Diagnostics.AddError("Error searching network resources", err.Error())
			return
		}

		// Apply exact name match if name was specified (API search is fuzzy)
		if hasName {
			name := config.Name.ValueString()
			var filtered []client.NetworkResourceResponse
			for _, r := range results {
				if r.Name == name {
					filtered = append(filtered, r)
				}
			}
			results = filtered
		}

		if len(results) == 0 {
			resp.Diagnostics.AddError("No matching resource found", "No network resource matches the specified criteria.")
			return
		}
		if len(results) > 1 {
			resp.Diagnostics.AddError("Multiple resources found",
				fmt.Sprintf("Found %d resources matching the criteria. Add more filters to narrow to exactly one.", len(results)))
			return
		}
		// List returns the slim list shape; re-fetch the full object by id so
		// name lookups match id lookups (see auth_provider for the reference pattern).
		full, err := d.client.GetNetworkResource(ctx, results[0].ID)
		if err != nil {
			resp.Diagnostics.AddError("Error reading network resource", err.Error())
			return
		}
		res = full
	}

	config.ID = types.Int64Value(res.ID)
	config.Name = types.StringValue(res.Name)
	config.ResourceType = types.StringValue(res.ResourceType)
	resourceType, err := d.client.FindResourceType(ctx, res.ResourceType)
	if err != nil {
		resp.Diagnostics.AddError("Error resolving resource engine", err.Error())
		return
	}
	config.EngineID = types.StringValue(resourceType.EngineID)
	config.ConnectorID = types.StringValue(res.ConnectorID)
	config.ExternalID = stringPtrToValue(res.ExternalID)
	config.ConnectorData, err = rawJSONToDynamic(res.ConnectorData)
	if err != nil {
		resp.Diagnostics.AddError("Error decoding resource connector_data", err.Error())
		return
	}
	config.CredentialID = int64PtrToValue(res.CredentialID)
	config.LocationID = int64PtrToValue(res.LocationID)
	mapNetworkResourceCoordinatesToDataSourceModel(res.EffectiveCoordinates, &config)
	config.Status = types.StringValue(res.Status)
	config.IsEnabled = types.BoolValue(res.IsEnabled)

	tags, terr := d.client.GetResourceTags(ctx, res.ID)
	if terr != nil {
		resp.Diagnostics.AddError("Error reading resource tags", terr.Error())
		return
	}
	config.Tags = tagsToList(tags)

	config.ExtraData, err = rawJSONToDynamic(res.ExtraData)
	if err != nil {
		resp.Diagnostics.AddError("Error decoding resource extra_data", err.Error())
		return
	}
	config.CreatedAt = types.StringValue(res.CreatedAt)
	config.UpdatedAt = types.StringValue(res.UpdatedAt)
	config.LastConnectivityCheck = optStringPtrToValue(res.LastConnectivityCheck)
	config.LastConnectivityResult, err = rawJSONToDynamic(res.LastConnectivityResult)
	if err != nil {
		resp.Diagnostics.AddError("Error decoding resource last_connectivity_result", err.Error())
		return
	}
	config.LastCollectionAt = optStringPtrToValue(res.LastCollectionAt)
	config.NextCollectionAt = optStringPtrToValue(res.NextCollectionAt)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

func mapNetworkResourceCoordinatesToDataSourceModel(
	coordinates *client.EffectiveCoordinates,
	model *NetworkResourceDataSourceModel,
) {
	if coordinates == nil {
		model.EffectiveLatitude = types.Float64Null()
		model.EffectiveLongitude = types.Float64Null()
		model.CoordinateSourceID = types.Int64Null()
		model.CoordinateSourceName = types.StringNull()
		return
	}

	model.EffectiveLatitude = types.Float64Value(coordinates.Latitude)
	model.EffectiveLongitude = types.Float64Value(coordinates.Longitude)
	model.CoordinateSourceID = int64PtrToValue(coordinates.SourceLocationID)
	model.CoordinateSourceName = stringPtrToValue(coordinates.SourceLocationName)
}
