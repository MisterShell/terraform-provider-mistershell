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

var _ datasource.DataSource = &WorkerDataSource{}

type WorkerDataSource struct {
	client *client.Client
}

type WorkerDataSourceModel struct {
	ID               types.Int64          `tfsdk:"id"`
	Name             types.String         `tfsdk:"name"`
	Description      types.String         `tfsdk:"description"`
	LocationID       types.Int64          `tfsdk:"location_id"`
	Status           types.String         `tfsdk:"status"`
	IsDefault        types.Bool           `tfsdk:"is_default"`
	IsEnabled        types.Bool           `tfsdk:"is_enabled"`
	ActiveTaskCount  types.Int64          `tfsdk:"active_task_count"`
	TotalTaskCount   types.Int64          `tfsdk:"total_task_count"`
	LastHeartbeatAt  types.String         `tfsdk:"last_heartbeat_at"`
	ConnectedAt      types.String         `tfsdk:"connected_at"`
	PresenceIP       types.String         `tfsdk:"presence_ip"`
	PresenceIPSource types.String         `tfsdk:"presence_ip_source"`
	Config           jsontypes.Normalized `tfsdk:"config"`
	ConfigVersion    types.String         `tfsdk:"config_version"`
	ConfigSchema     jsontypes.Normalized `tfsdk:"config_schema"`
	CreatedAt        types.String         `tfsdk:"created_at"`
	UpdatedAt        types.String         `tfsdk:"updated_at"`
}

func NewWorkerDataSource() datasource.DataSource {
	return &WorkerDataSource{}
}

func (d *WorkerDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_worker"
}

func (d *WorkerDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a MisterShell worker. Look up by id for a direct fetch, or by name (exact match). The authentication token is not returned by reads.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Description: "Worker ID. Use for direct lookup.",
				Optional:    true,
				Computed:    true,
			},
			"name": schema.StringAttribute{
				Description: "Worker name. Used as an exact-match search filter when id is not specified.",
				Optional:    true,
				Computed:    true,
			},
			"description": schema.StringAttribute{
				Description: "Worker description.",
				Computed:    true,
			},
			"location_id": schema.Int64Attribute{
				Description: "ID of the location this worker belongs to.",
				Computed:    true,
			},
			"status": schema.StringAttribute{
				Description: "Worker status.",
				Computed:    true,
			},
			"is_default": schema.BoolAttribute{
				Description: "Whether this is the bootstrap default worker.",
				Computed:    true,
			},
			"is_enabled": schema.BoolAttribute{
				Description: "Whether the worker is enabled.",
				Computed:    true,
			},
			"active_task_count": schema.Int64Attribute{
				Description: "Number of currently active tasks assigned to the worker.",
				Computed:    true,
			},
			"total_task_count": schema.Int64Attribute{
				Description: "Total number of tasks ever assigned to the worker.",
				Computed:    true,
			},
			"last_heartbeat_at": schema.StringAttribute{
				Description: "Timestamp of the worker's last heartbeat.",
				Computed:    true,
			},
			"connected_at": schema.StringAttribute{
				Description: "Timestamp at which the worker connected.",
				Computed:    true,
			},
			"presence_ip": schema.StringAttribute{
				Description: "IP address from which the worker is connected.",
				Computed:    true,
			},
			"presence_ip_source": schema.StringAttribute{
				Description: "Source of the presence IP determination.",
				Computed:    true,
			},
			"config": schema.StringAttribute{
				Description: "Opaque worker task-handler config as JSON.",
				Computed:    true,
				CustomType:  jsontypes.NormalizedType{},
			},
			"config_version": schema.StringAttribute{
				Description: "Version identifier of the currently applied worker config.",
				Computed:    true,
			},
			"config_schema": schema.StringAttribute{
				Description: "JSON Schema used to validate the worker config, as JSON.",
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

func (d *WorkerDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *WorkerDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config WorkerDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	hasID := !config.ID.IsNull() && !config.ID.IsUnknown()

	var worker *client.WorkerResponse

	if hasID {
		var err error
		worker, err = d.client.GetWorker(ctx, config.ID.ValueInt64())
		if err != nil {
			resp.Diagnostics.AddError("Error reading worker", err.Error())
			return
		}
	} else {
		hasName := !config.Name.IsNull() && !config.Name.IsUnknown()
		if !hasName {
			resp.Diagnostics.AddError("Missing lookup criteria", "Specify 'id' for direct lookup, or 'name' to search.")
			return
		}

		workers, err := d.client.ListWorkers(ctx, client.WorkerListFilter{Search: config.Name.ValueString()})
		if err != nil {
			resp.Diagnostics.AddError("Error searching workers", err.Error())
			return
		}

		name := config.Name.ValueString()
		var filtered []client.WorkerResponse
		for _, w := range workers {
			if w.Name == name {
				filtered = append(filtered, w)
			}
		}

		if len(filtered) == 0 {
			resp.Diagnostics.AddError("No matching worker found", "No worker matches the specified name.")
			return
		}
		if len(filtered) > 1 {
			resp.Diagnostics.AddError("Multiple workers found",
				fmt.Sprintf("Found %d workers matching name %q.", len(filtered), name))
			return
		}
		worker = &filtered[0]
	}

	config.ID = types.Int64Value(worker.ID)
	config.Name = types.StringValue(worker.Name)
	config.Description = stringPtrToValue(worker.Description)
	config.LocationID = types.Int64Value(worker.LocationID)
	config.Status = types.StringValue(worker.Status)
	config.IsDefault = types.BoolValue(worker.IsDefault)
	config.IsEnabled = types.BoolValue(worker.IsEnabled)
	config.ActiveTaskCount = types.Int64Value(worker.ActiveTaskCount)
	config.TotalTaskCount = types.Int64Value(worker.TotalTaskCount)
	config.LastHeartbeatAt = optStringPtrToValue(worker.LastHeartbeatAt)
	config.ConnectedAt = optStringPtrToValue(worker.ConnectedAt)
	config.PresenceIP = optStringPtrToValue(worker.PresenceIP)
	config.PresenceIPSource = optStringPtrToValue(worker.PresenceIPSource)
	config.Config = rawJSONToNormalized(worker.Config)
	config.ConfigVersion = optStringPtrToValue(worker.ConfigVersion)
	config.ConfigSchema = rawJSONToNormalized(worker.ConfigSchema)
	config.CreatedAt = types.StringValue(worker.CreatedAt)
	config.UpdatedAt = types.StringValue(worker.UpdatedAt)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
