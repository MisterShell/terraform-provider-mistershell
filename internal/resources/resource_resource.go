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
	_ resource.Resource                = &NetworkResourceResource{}
	_ resource.ResourceWithImportState = &NetworkResourceResource{}
)

type NetworkResourceResource struct {
	client *client.Client
}

type NetworkResourceResourceModel struct {
	ID                    types.Int64          `tfsdk:"id"`
	Name                  types.String         `tfsdk:"name"`
	ResourceType          types.String         `tfsdk:"resource_type"`
	ExternalID            types.String         `tfsdk:"external_id"`
	LocationID            types.Int64          `tfsdk:"location_id"`
	ConnectorData         jsontypes.Normalized `tfsdk:"connector_data"`
	CredentialID          types.Int64          `tfsdk:"credential_id"`
	ExtraData             jsontypes.Normalized `tfsdk:"extra_data"`
	IsEnabled             types.Bool           `tfsdk:"is_enabled"`
	TagIDs                types.Set            `tfsdk:"tag_ids"`
	Tags                  types.List           `tfsdk:"tags"`
	ConnectorID           types.String         `tfsdk:"connector_id"`
	Status                types.String         `tfsdk:"status"`
	HealthStatus          types.String         `tfsdk:"health_status"`
	CreatedAt             types.String         `tfsdk:"created_at"`
	UpdatedAt             types.String         `tfsdk:"updated_at"`
	LastConnectivityCheck types.String         `tfsdk:"last_connectivity_check"`
	LastCollectionAt      types.String         `tfsdk:"last_collection_at"`
	NextCollectionAt      types.String         `tfsdk:"next_collection_at"`
	LastSnapshotAt        types.String         `tfsdk:"last_snapshot_at"`
	LastHealthAt          types.String         `tfsdk:"last_health_at"`
}

func NewNetworkResourceResource() resource.Resource {
	return &NetworkResourceResource{}
}

func (r *NetworkResourceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_resource"
}

func (r *NetworkResourceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a MisterShell network resource (device, cloud account, Kubernetes cluster).",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Description: "Resource ID.",
				Computed:    true,
				PlanModifiers: []planmodifier.Int64{
					int64UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Resource display name.",
				Required:    true,
			},
			"resource_type": schema.StringAttribute{
				Description: "What the resource is (e.g. cisco_iosxe, linux, aws_account). Cannot be changed after creation.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.OneOf(client.SupportedResourceTypes...),
				},
			},
			"external_id": schema.StringAttribute{
				Description: "Unique external identifier for the resource (hostname, account ID, etc.).",
				Required:    true,
			},
			"location_id": schema.Int64Attribute{
				Description: "Location ID where this resource resides.",
				Required:    true,
			},
			"connector_data": schema.StringAttribute{
				Description: "Type-specific connection parameters as JSON. Use jsonencode() in HCL. Fields vary by resource_type (e.g. host/port for SSH types, rdp_port/nla_required for windows and generic_rdp, engine/host/port for database); see the valid resource_type values table on this page for the fields per type.",
				Optional:    true,
				CustomType:  jsontypes.NormalizedType{},
			},
			"credential_id": schema.Int64Attribute{
				Description: "Credential ID for connecting to this resource.",
				Optional:    true,
			},
			"extra_data": schema.StringAttribute{
				Description: "Discovered metadata as JSON. Auto-populated by MisterShell from connectivity checks.",
				Computed:    true,
				CustomType:  jsontypes.NormalizedType{},
			},
			"is_enabled": schema.BoolAttribute{
				Description: "Whether the resource is enabled for operations.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
			},
			"tag_ids": schema.SetAttribute{
				Description: "Set of tag IDs to assign to this resource (reference them as mistershell_tag.<name>.id). When set, the provider manages the resource's tags **exclusively** (it adds and removes tags to match this set exactly; `[]` clears all tags). When **omitted/null**, the provider does not manage tags at all — leave it unset if you assign this resource's tags from the tag side via mistershell_tag.resource_ids. Own each tag↔resource edge from one side only.",
				Optional:    true,
				ElementType: types.Int64Type,
			},
			"tags": schema.ListNestedAttribute{
				Description: "The tags currently assigned to this resource (read-back), as objects with id, name, color, and description. Always reflects server state regardless of whether tag_ids is managed.",
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
			"connector_id": schema.StringAttribute{
				Description: "Connector type, derived by the server from resource_type.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"status": schema.StringAttribute{
				Description: "Connectivity status (unknown, verified, unreachable, auth_failed, error, identity_mismatch, snapshot_truncated).",
				Computed:    true,
			},
			"health_status": schema.StringAttribute{
				Description: "Health status (healthy, degraded, critical, unknown).",
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

func (r *NetworkResourceResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *NetworkResourceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan NetworkResourceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	input := client.NetworkResourceCreateInput{
		Name:          plan.Name.ValueString(),
		ResourceType:  plan.ResourceType.ValueString(),
		ExternalID:    plan.ExternalID.ValueString(),
		LocationID:    plan.LocationID.ValueInt64(),
		ConnectorData: normalizedToRawJSON(plan.ConnectorData),
	}
	input.CredentialID = int64ValueToPtr(plan.CredentialID)

	res, err := r.client.CreateNetworkResource(ctx, input)
	if err != nil {
		resp.Diagnostics.AddError("Error creating network resource", err.Error())
		return
	}

	// Preserve connector_data from plan — the API enriches it with server-side defaults.
	mapNetworkResourceResponseToModel(res, &plan)
	if err := r.reconcileTags(ctx, res.ID, &plan); err != nil {
		resp.Diagnostics.AddError("Error setting resource tags", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *NetworkResourceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state NetworkResourceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	res, err := r.client.GetNetworkResource(ctx, state.ID.ValueInt64())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading network resource", err.Error())
		return
	}

	// Preserve connector_data from state — the API enriches it with server-side defaults
	// which would cause perpetual diffs.
	savedConnectorData := state.ConnectorData
	mapNetworkResourceResponseToModel(res, &state)
	state.ConnectorData = savedConnectorData
	if err := r.readTags(ctx, state.ID.ValueInt64(), &state); err != nil {
		resp.Diagnostics.AddError("Error reading resource tags", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *NetworkResourceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan NetworkResourceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state NetworkResourceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	input := client.NetworkResourceUpdateInput{}
	if !plan.Name.Equal(state.Name) {
		v := plan.Name.ValueString()
		input.Name = &v
	}
	if !plan.ExternalID.Equal(state.ExternalID) {
		v := plan.ExternalID.ValueString()
		input.ExternalID = &v
	}
	if !plan.LocationID.Equal(state.LocationID) {
		v := plan.LocationID.ValueInt64()
		input.LocationID = &v
	}
	input.ConnectorData = normalizedToRawJSON(plan.ConnectorData)
	input.CredentialID = int64ValueToPtr(plan.CredentialID)
	if !plan.IsEnabled.Equal(state.IsEnabled) {
		v := plan.IsEnabled.ValueBool()
		input.IsEnabled = &v
	}

	res, err := r.client.UpdateNetworkResource(ctx, state.ID.ValueInt64(), input)
	if err != nil {
		resp.Diagnostics.AddError("Error updating network resource", err.Error())
		return
	}

	mapNetworkResourceResponseToModel(res, &plan)
	if err := r.reconcileTags(ctx, state.ID.ValueInt64(), &plan); err != nil {
		resp.Diagnostics.AddError("Error updating resource tags", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *NetworkResourceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state NetworkResourceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteNetworkResource(ctx, state.ID.ValueInt64()); err != nil {
		resp.Diagnostics.AddError("Error deleting network resource", err.Error())
	}
}

func (r *NetworkResourceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("Expected integer ID, got: %s", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), types.Int64Value(id))...)
}

// reconcileTags is used by Create/Update. When tag_ids is set (managed), it
// replaces the resource's tag set exactly via PUT and reflects the server result
// into both tag_ids and the computed tags. When tag_ids is null (unmanaged), it
// does NOT modify tags — it only reads the current set into the computed tags and
// leaves tag_ids null.
func (r *NetworkResourceResource) reconcileTags(ctx context.Context, resourceID int64, m *NetworkResourceResourceModel) error {
	managed := !m.TagIDs.IsNull() && !m.TagIDs.IsUnknown()
	var (
		tags []client.TagResponse
		err  error
	)
	if managed {
		tags, err = r.client.SetResourceTags(ctx, resourceID, int64SetToSlice(m.TagIDs))
	} else {
		tags, err = r.client.GetResourceTags(ctx, resourceID)
	}
	if err != nil {
		return err
	}
	m.Tags = tagsToList(tags)
	if managed {
		m.TagIDs = tagIDsToSet(tags)
	} else {
		m.TagIDs = types.SetNull(types.Int64Type)
	}
	return nil
}

// readTags is used by Read. It always refreshes the computed tags from the
// server; it refreshes tag_ids from the server only when tag_ids is already
// managed (non-null), so drift on managed tags is detected while an unmanaged
// resource keeps tag_ids null.
func (r *NetworkResourceResource) readTags(ctx context.Context, resourceID int64, m *NetworkResourceResourceModel) error {
	tags, err := r.client.GetResourceTags(ctx, resourceID)
	if err != nil {
		return err
	}
	m.Tags = tagsToList(tags)
	if !m.TagIDs.IsNull() {
		m.TagIDs = tagIDsToSet(tags)
	}
	return nil
}

func mapNetworkResourceResponseToModel(res *client.NetworkResourceResponse, m *NetworkResourceResourceModel) {
	m.ID = types.Int64Value(res.ID)
	m.Name = types.StringValue(res.Name)
	m.ResourceType = types.StringValue(res.ResourceType)
	m.ExternalID = types.StringValue(res.ExternalID)
	m.LocationID = int64PtrToValue(res.LocationID)
	// ConnectorData intentionally NOT set here — preserved from plan/state by the caller
	// because the API enriches it with server-side defaults (e.g. strict_host_key).
	m.CredentialID = int64PtrToValue(res.CredentialID)
	m.ExtraData = rawJSONToNormalized(res.ExtraData)
	m.IsEnabled = types.BoolValue(res.IsEnabled)
	m.ConnectorID = types.StringValue(res.ConnectorID)
	m.Status = types.StringValue(res.Status)
	m.HealthStatus = types.StringValue(res.HealthStatus)
	m.CreatedAt = types.StringValue(res.CreatedAt)
	m.UpdatedAt = types.StringValue(res.UpdatedAt)
	m.LastConnectivityCheck = optStringPtrToValue(res.LastConnectivityCheck)
	m.LastCollectionAt = optStringPtrToValue(res.LastCollectionAt)
	m.NextCollectionAt = optStringPtrToValue(res.NextCollectionAt)
	m.LastSnapshotAt = optStringPtrToValue(res.LastSnapshotAt)
	m.LastHealthAt = optStringPtrToValue(res.LastHealthAt)
}
