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

var _ datasource.DataSource = &NetworkResourceDataSource{}

type NetworkResourceDataSource struct {
	client *client.Client
}

type NetworkResourceDataSourceModel struct {
	ID                    types.Int64          `tfsdk:"id"`
	Name                  types.String         `tfsdk:"name"`
	ResourceType          types.String         `tfsdk:"resource_type"`
	ConnectorID           types.String         `tfsdk:"connector_id"`
	ExternalID            types.String         `tfsdk:"external_id"`
	ConnectorData         jsontypes.Normalized `tfsdk:"connector_data"`
	CredentialID          types.Int64          `tfsdk:"credential_id"`
	LocationID            types.Int64          `tfsdk:"location_id"`
	Status                types.String         `tfsdk:"status"`
	HealthStatus          types.String         `tfsdk:"health_status"`
	IsEnabled             types.Bool           `tfsdk:"is_enabled"`
	Tags                  types.List           `tfsdk:"tags"`
	ExtraData             jsontypes.Normalized `tfsdk:"extra_data"`
	CreatedAt             types.String         `tfsdk:"created_at"`
	UpdatedAt             types.String         `tfsdk:"updated_at"`
	LastConnectivityCheck types.String         `tfsdk:"last_connectivity_check"`
	LastCollectionAt      types.String         `tfsdk:"last_collection_at"`
	NextCollectionAt      types.String         `tfsdk:"next_collection_at"`
	LastSnapshotAt        types.String         `tfsdk:"last_snapshot_at"`
	LastHealthAt          types.String         `tfsdk:"last_health_at"`
}

func NewNetworkResourceDataSource() datasource.DataSource {
	return &NetworkResourceDataSource{}
}

func (d *NetworkResourceDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_resource"
}

func (d *NetworkResourceDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a MisterShell network resource. Look up by id for a direct fetch, or use name/resource_type/location_id/status/health_status to search. Search filters must match exactly one resource.",
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
				Description: "Resource type filter (e.g. cisco_iosxe, linux, aws_account).",
				Optional:    true,
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
			"connector_data": schema.StringAttribute{
				Description: "Connection parameters as JSON. Fields vary by resource_type; see the valid resource_type values table on this page.",
				Computed:    true,
				CustomType:  jsontypes.NormalizedType{},
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
			"status": schema.StringAttribute{
				Description: "Connectivity status filter (unknown, verified, unreachable, auth_failed, error, identity_mismatch, snapshot_truncated).",
				Optional:    true,
				Computed:    true,
			},
			"health_status": schema.StringAttribute{
				Description: "Health status filter (healthy, degraded, critical, unknown).",
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
			"extra_data": schema.StringAttribute{
				Description: "Discovered metadata as JSON.",
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
			"last_connectivity_check": schema.StringAttribute{
				Description: "Timestamp of last connectivity check.",
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
			"last_snapshot_at": schema.StringAttribute{
				Description: "Timestamp of last configuration snapshot.",
				Computed:    true,
			},
			"last_health_at": schema.StringAttribute{
				Description: "Timestamp of last health check.",
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
		hasHealthStatus := !config.HealthStatus.IsNull() && !config.HealthStatus.IsUnknown()

		if !hasName && !hasType && !hasLocationID && !hasStatus && !hasHealthStatus {
			resp.Diagnostics.AddError("Missing lookup criteria",
				"Specify 'id' for direct lookup, or at least one of 'name', 'resource_type', 'location_id', 'status', 'health_status' to search.")
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
		if hasHealthStatus {
			filter.HealthStatus = config.HealthStatus.ValueString()
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
		res = &results[0]
	}

	config.ID = types.Int64Value(res.ID)
	config.Name = types.StringValue(res.Name)
	config.ResourceType = types.StringValue(res.ResourceType)
	config.ConnectorID = types.StringValue(res.ConnectorID)
	config.ExternalID = types.StringValue(res.ExternalID)
	config.ConnectorData = rawJSONToNormalized(res.ConnectorData)
	config.CredentialID = int64PtrToValue(res.CredentialID)
	config.LocationID = int64PtrToValue(res.LocationID)
	config.Status = types.StringValue(res.Status)
	config.HealthStatus = types.StringValue(res.HealthStatus)
	config.IsEnabled = types.BoolValue(res.IsEnabled)

	tags, terr := d.client.GetResourceTags(ctx, res.ID)
	if terr != nil {
		resp.Diagnostics.AddError("Error reading resource tags", terr.Error())
		return
	}
	config.Tags = tagsToList(tags)

	config.ExtraData = rawJSONToNormalized(res.ExtraData)
	config.CreatedAt = types.StringValue(res.CreatedAt)
	config.UpdatedAt = types.StringValue(res.UpdatedAt)
	config.LastConnectivityCheck = optStringPtrToValue(res.LastConnectivityCheck)
	config.LastCollectionAt = optStringPtrToValue(res.LastCollectionAt)
	config.NextCollectionAt = optStringPtrToValue(res.NextCollectionAt)
	config.LastSnapshotAt = optStringPtrToValue(res.LastSnapshotAt)
	config.LastHealthAt = optStringPtrToValue(res.LastHealthAt)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
