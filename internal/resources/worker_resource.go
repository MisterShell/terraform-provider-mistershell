package resources

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-mistershell/internal/client"
)

var (
	_ resource.Resource                = &WorkerResource{}
	_ resource.ResourceWithImportState = &WorkerResource{}
)

type WorkerResource struct {
	client *client.Client
}

type WorkerResourceModel struct {
	ID               types.Int64          `tfsdk:"id"`
	Name             types.String         `tfsdk:"name"`
	Description      types.String         `tfsdk:"description"`
	LocationID       types.Int64          `tfsdk:"location_id"`
	Config           jsontypes.Normalized `tfsdk:"config"`
	ConfigSchema     jsontypes.Normalized `tfsdk:"config_schema"`
	IsEnabled        types.Bool           `tfsdk:"is_enabled"`
	Token            types.String         `tfsdk:"token"`
	Status           types.String         `tfsdk:"status"`
	IsDefault        types.Bool           `tfsdk:"is_default"`
	ActiveTaskCount  types.Int64          `tfsdk:"active_task_count"`
	TotalTaskCount   types.Int64          `tfsdk:"total_task_count"`
	LastHeartbeatAt  types.String         `tfsdk:"last_heartbeat_at"`
	ConnectedAt      types.String         `tfsdk:"connected_at"`
	PresenceIP       types.String         `tfsdk:"presence_ip"`
	PresenceIPSource types.String         `tfsdk:"presence_ip_source"`
	ConfigVersion    types.String         `tfsdk:"config_version"`
	CreatedAt        types.String         `tfsdk:"created_at"`
	UpdatedAt        types.String         `tfsdk:"updated_at"`
}

func NewWorkerResource() resource.Resource {
	return &WorkerResource{}
}

func (r *WorkerResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_worker"
}

func (r *WorkerResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This resource manages user-created workers. The bootstrap default
		// worker (is_default=true) is provisioned by the backend and cannot be
		// edited or deleted — the API returns 403 — so it must not be managed
		// here. The worker authentication token is returned only once, at
		// creation, and can never be read back afterwards.
		Description: "Manages a MisterShell worker — a task-handler agent bound to a location. The config and config_schema attributes are opaque JSON blobs (the worker's task-handler config and its JSON Schema). The bootstrap default worker (is_default=true) is provisioned by the backend and cannot be edited or deleted (the API returns 403); do not manage it with this resource. The authentication token is returned only once at creation and is never readable again.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Description: "Worker ID.",
				Computed:    true,
				PlanModifiers: []planmodifier.Int64{
					int64UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Worker name (1-255 characters).",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, 255),
				},
			},
			"description": schema.StringAttribute{
				Description: "Worker description.",
				Optional:    true,
			},
			"location_id": schema.Int64Attribute{
				Description: "ID of the location this worker belongs to. Updatable.",
				Required:    true,
			},
			"config": schema.StringAttribute{
				Description: "Opaque worker task-handler config as JSON. Use jsonencode() in HCL.",
				Optional:    true,
				CustomType:  jsontypes.NormalizedType{},
			},
			"config_schema": schema.StringAttribute{
				Description: "JSON Schema used to validate the worker config, as JSON. Use jsonencode() in HCL.",
				Optional:    true,
				CustomType:  jsontypes.NormalizedType{},
			},
			"is_enabled": schema.BoolAttribute{
				Description: "Whether the worker is enabled. Defaults to true.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
			},
			"token": schema.StringAttribute{
				Description: "Worker authentication token. Returned only once at creation and never readable again; not populated on import.",
				Computed:    true,
				Sensitive:   true,
				PlanModifiers: []planmodifier.String{
					stringUseStateForUnknown(),
				},
			},
			"status": schema.StringAttribute{
				Description: "Worker status.",
				Computed:    true,
			},
			"is_default": schema.BoolAttribute{
				Description: "Whether this is the bootstrap default worker. Default workers cannot be edited or deleted via the API.",
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
			"config_version": schema.StringAttribute{
				Description: "Version identifier of the currently applied worker config.",
				Computed:    true,
			},
			"created_at": schema.StringAttribute{
				Description: "Creation timestamp.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"updated_at": schema.StringAttribute{
				Description: "Last update timestamp.",
				Computed:    true,
			},
		},
	}
}

func (r *WorkerResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Resource Configure Type", "Expected *client.Client")
		return
	}
	r.client = c
}

func (r *WorkerResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan WorkerResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	input := client.WorkerCreateInput{
		Name:         plan.Name.ValueString(),
		Description:  stringValueToPtr(plan.Description),
		LocationID:   plan.LocationID.ValueInt64(),
		Config:       normalizedToRawJSON(plan.Config),
		ConfigSchema: normalizedToRawJSON(plan.ConfigSchema),
	}

	worker, err := r.client.CreateWorker(ctx, input)
	if err != nil {
		resp.Diagnostics.AddError("Error creating worker", err.Error())
		return
	}

	// The token is returned only on the create response; persist it into state.
	token := types.StringValue(worker.Token)
	// Preserve config/config_schema from plan: the server may reorder keys and
	// inject defaults, so the response JSON differs byte-wise from the planned
	// value, which would break Terraform's apply-consistency contract.
	savedConfig := plan.Config
	savedConfigSchema := plan.ConfigSchema
	mapWorkerResponseToModel(worker, &plan)
	plan.Token = token
	plan.Config = savedConfig
	plan.ConfigSchema = savedConfigSchema
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *WorkerResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state WorkerResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	worker, err := r.client.GetWorker(ctx, state.ID.ValueInt64())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading worker", err.Error())
		return
	}

	// The server GET does not return the token and may reorder config; preserve
	// token and config/config_schema from prior state.
	savedToken := state.Token
	savedConfig := state.Config
	savedConfigSchema := state.ConfigSchema
	mapWorkerResponseToModel(worker, &state)
	state.Token = savedToken
	state.Config = savedConfig
	state.ConfigSchema = savedConfigSchema
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *WorkerResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan WorkerResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state WorkerResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := plan.Name.ValueString()
	locationID := plan.LocationID.ValueInt64()
	isEnabled := plan.IsEnabled.ValueBool()
	input := client.WorkerUpdateInput{
		Name:         &name,
		Description:  stringValueToPtr(plan.Description),
		LocationID:   &locationID,
		Config:       normalizedToRawJSON(plan.Config),
		ConfigSchema: normalizedToRawJSON(plan.ConfigSchema),
		IsEnabled:    &isEnabled,
	}

	worker, err := r.client.UpdateWorker(ctx, state.ID.ValueInt64(), input)
	if err != nil {
		resp.Diagnostics.AddError("Error updating worker", err.Error())
		return
	}

	// Preserve token (not returned by update) and config/config_schema from plan
	// (server reorders/enriches).
	savedToken := state.Token
	savedConfig := plan.Config
	savedConfigSchema := plan.ConfigSchema
	mapWorkerResponseToModel(worker, &plan)
	plan.Token = savedToken
	plan.Config = savedConfig
	plan.ConfigSchema = savedConfigSchema
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *WorkerResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state WorkerResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// If the targeted worker is the bootstrap default (is_default=true), the API
	// returns 403; let that error surface to the user.
	if err := r.client.DeleteWorker(ctx, state.ID.ValueInt64()); err != nil {
		resp.Diagnostics.AddError("Error deleting worker", err.Error())
	}
}

func (r *WorkerResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("Expected integer ID, got: %s", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), types.Int64Value(id))...)
}

// mapWorkerResponseToModel maps the API response to the Terraform model. The
// token and config/config_schema are set by the caller per their preserve rules.
func mapWorkerResponseToModel(worker *client.WorkerResponse, m *WorkerResourceModel) {
	m.ID = types.Int64Value(worker.ID)
	m.Name = types.StringValue(worker.Name)
	m.Description = stringPtrToValue(worker.Description)
	m.LocationID = types.Int64Value(worker.LocationID)
	m.IsEnabled = types.BoolValue(worker.IsEnabled)
	m.Status = types.StringValue(worker.Status)
	m.IsDefault = types.BoolValue(worker.IsDefault)
	m.ActiveTaskCount = types.Int64Value(worker.ActiveTaskCount)
	m.TotalTaskCount = types.Int64Value(worker.TotalTaskCount)
	m.LastHeartbeatAt = optStringPtrToValue(worker.LastHeartbeatAt)
	m.ConnectedAt = optStringPtrToValue(worker.ConnectedAt)
	m.PresenceIP = optStringPtrToValue(worker.PresenceIP)
	m.PresenceIPSource = optStringPtrToValue(worker.PresenceIPSource)
	m.ConfigVersion = optStringPtrToValue(worker.ConfigVersion)
	m.CreatedAt = types.StringValue(worker.CreatedAt)
	m.UpdatedAt = types.StringValue(worker.UpdatedAt)
}
