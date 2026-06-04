package resources

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-mistershell/internal/client"
)

var (
	_ resource.Resource                = &LogDestinationResource{}
	_ resource.ResourceWithImportState = &LogDestinationResource{}
)

type LogDestinationResource struct {
	client *client.Client
}

type LogDestinationResourceModel struct {
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

func NewLogDestinationResource() resource.Resource {
	return &LogDestinationResource{}
}

func (r *LogDestinationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_log_destination"
}

func (r *LogDestinationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a MisterShell log destination, forwarding security/policy/api/app log streams to a syslog server or webhook endpoint. The config attribute is an opaque per-type JSON blob (syslog or webhook); webhook auth secrets are masked by the API and stored from config (not read back).",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Description: "Log destination ID.",
				Computed:    true,
				PlanModifiers: []planmodifier.Int64{
					int64UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Log destination name (1-128 characters).",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, 128),
				},
			},
			"enabled": schema.BoolAttribute{
				Description: "Whether the destination is enabled. Defaults to true.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
			},
			"type": schema.StringAttribute{
				Description: "Destination type. One of: syslog, webhook. Cannot be changed after creation (the config shape is type-coupled).",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.OneOf(client.SupportedLogDestinationTypes...),
				},
			},
			"streams": schema.SetAttribute{
				Description: "Log streams to forward. At least one of: security, policy, api, app.",
				Required:    true,
				ElementType: types.StringType,
				Validators: []validator.Set{
					setvalidator.SizeAtLeast(1),
					setvalidator.ValueStringsAre(stringvalidator.OneOf(client.SupportedLogStreams...)),
				},
			},
			"min_severity": schema.StringAttribute{
				Description: "Minimum severity to forward. One of: info, low, medium, high, critical. Defaults to info.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("info"),
				Validators: []validator.String{
					stringvalidator.OneOf(client.SupportedLogSeverities...),
				},
			},
			"config": schema.StringAttribute{
				Description: "Per-type config as JSON. Use jsonencode() in HCL. For type=syslog: host, port, protocol, format, facility, tls_verify. For type=webhook: url, method, headers, auth, body_format, timeout_seconds, tls_verify. Webhook auth secrets (bearer token, basic password, header value) are masked as '****' by the API and stored from config, not read back. See the config field tables on this page.",
				Required:    true,
				Sensitive:   true,
				CustomType:  jsontypes.NormalizedType{},
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

func (r *LogDestinationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *LogDestinationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan LogDestinationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	input := client.LogDestinationCreateInput{
		Name:        plan.Name.ValueString(),
		Type:        plan.Type.ValueString(),
		Streams:     stringSetToSlice(plan.Streams),
		MinSeverity: plan.MinSeverity.ValueString(),
		Config:      normalizedToRawJSON(plan.Config),
	}
	if !plan.Enabled.IsNull() && !plan.Enabled.IsUnknown() {
		v := plan.Enabled.ValueBool()
		input.Enabled = &v
	}

	dest, err := r.client.CreateLogDestination(ctx, input)
	if err != nil {
		resp.Diagnostics.AddError("Error creating log destination", err.Error())
		return
	}

	// Preserve config from plan: the server reorders keys and injects per-type
	// defaults, so the response config differs byte-wise from the Required config.
	savedConfig := plan.Config
	mapLogDestinationResponseToModel(dest, &plan)
	plan.Config = savedConfig
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *LogDestinationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state LogDestinationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	dest, err := r.client.GetLogDestination(ctx, state.ID.ValueInt64())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading log destination", err.Error())
		return
	}

	// Preserve the config already in state (the user's value). The server both
	// reorders config keys and injects per-type defaults (e.g. syslog facility,
	// tls_verify), so reflecting the server config would differ byte-wise from
	// the planned/required config and break Terraform's apply-consistency
	// contract for this Required+Sensitive attribute ("inconsistent values for
	// sensitive attribute"). webhook auth secrets are additionally masked ****.
	// Both cases therefore use the stored-from-config rule (like credential_data).
	savedConfig := state.Config
	mapLogDestinationResponseToModel(dest, &state)
	state.Config = savedConfig
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *LogDestinationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan LogDestinationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state LogDestinationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Full-replace PUT — send the whole planned object verbatim.
	input := client.LogDestinationUpdateInput{
		Name:        plan.Name.ValueString(),
		Type:        plan.Type.ValueString(),
		Streams:     stringSetToSlice(plan.Streams),
		MinSeverity: plan.MinSeverity.ValueString(),
		Config:      normalizedToRawJSON(plan.Config),
	}
	if !plan.Enabled.IsNull() && !plan.Enabled.IsUnknown() {
		v := plan.Enabled.ValueBool()
		input.Enabled = &v
	}

	dest, err := r.client.UpdateLogDestination(ctx, state.ID.ValueInt64(), input)
	if err != nil {
		resp.Diagnostics.AddError("Error updating log destination", err.Error())
		return
	}

	// Preserve config from plan (server reorders/enriches; webhook masks secrets).
	savedConfig := plan.Config
	mapLogDestinationResponseToModel(dest, &plan)
	plan.Config = savedConfig
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *LogDestinationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state LogDestinationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteLogDestination(ctx, state.ID.ValueInt64()); err != nil {
		resp.Diagnostics.AddError("Error deleting log destination", err.Error())
	}
}

func (r *LogDestinationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("Expected integer ID, got: %s", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), types.Int64Value(id))...)
}

// mapLogDestinationResponseToModel maps the API response to the Terraform model.
// config is set by the caller per the syslog/webhook secret rules.
func mapLogDestinationResponseToModel(dest *client.LogDestinationResponse, m *LogDestinationResourceModel) {
	m.ID = types.Int64Value(dest.ID)
	m.Name = types.StringValue(dest.Name)
	m.Enabled = types.BoolValue(dest.Enabled)
	m.Type = types.StringValue(dest.Type)
	m.Streams = stringSliceToSet(dest.Streams)
	m.MinSeverity = types.StringValue(dest.MinSeverity)
	m.CreatedAt = types.StringValue(dest.CreatedAt)
	m.UpdatedAt = types.StringValue(dest.UpdatedAt)
}
