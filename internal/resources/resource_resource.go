package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
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
	"terraform-provider-mistershell/internal/tfjson"
)

var (
	_ resource.Resource                     = &NetworkResourceResource{}
	_ resource.ResourceWithConfigValidators = &NetworkResourceResource{}
	_ resource.ResourceWithImportState      = &NetworkResourceResource{}
)

type NetworkResourceResource struct {
	client *client.Client
}

type NetworkResourceResourceModel struct {
	CheckTaskID            types.Int64   `tfsdk:"check_task_id"`
	AcceptedTrustFields    types.Set     `tfsdk:"accepted_trust_fields"`
	AcceptedIdentity       types.Bool    `tfsdk:"accepted_identity"`
	HealthStatus           types.String  `tfsdk:"health_status"`
	LastHealthAt           types.String  `tfsdk:"last_health_at"`
	ID                     types.Int64   `tfsdk:"id"`
	Name                   types.String  `tfsdk:"name"`
	EngineID               types.String  `tfsdk:"engine_id"`
	ResourceType           types.String  `tfsdk:"resource_type"`
	ExternalID             types.String  `tfsdk:"external_id"`
	LocationID             types.Int64   `tfsdk:"location_id"`
	EffectiveLatitude      types.Float64 `tfsdk:"effective_latitude"`
	EffectiveLongitude     types.Float64 `tfsdk:"effective_longitude"`
	CoordinateSourceID     types.Int64   `tfsdk:"coordinate_source_location_id"`
	CoordinateSourceName   types.String  `tfsdk:"coordinate_source_location_name"`
	ConnectorData          types.Dynamic `tfsdk:"connector_data"`
	CredentialID           types.Int64   `tfsdk:"credential_id"`
	ExtraData              types.Dynamic `tfsdk:"extra_data"`
	IsEnabled              types.Bool    `tfsdk:"is_enabled"`
	SshgwAllowPersonalKey  types.Bool    `tfsdk:"sshgw_allow_personal_key"`
	TagIDs                 types.Set     `tfsdk:"tag_ids"`
	Tags                   types.List    `tfsdk:"tags"`
	ConnectorID            types.String  `tfsdk:"connector_id"`
	Status                 types.String  `tfsdk:"status"`
	CreatedAt              types.String  `tfsdk:"created_at"`
	UpdatedAt              types.String  `tfsdk:"updated_at"`
	LastConnectivityCheck  types.String  `tfsdk:"last_connectivity_check"`
	LastConnectivityResult types.Dynamic `tfsdk:"last_connectivity_result"`
	LastCollectionAt       types.String  `tfsdk:"last_collection_at"`
	NextCollectionAt       types.String  `tfsdk:"next_collection_at"`
}

func NewNetworkResourceResource() resource.Resource {
	return &NetworkResourceResource{}
}

func (r *NetworkResourceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_resource"
}

func (r *NetworkResourceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a MisterShell network resource through live onboarding. Creation requires a successful connection and may take several minutes. Set engine_id for automatic discovery (SSH falls back to Generic SSH) or resource_type for exact-type discovery. external_id is discovered and may be null.",
		Attributes: map[string]schema.Attribute{
			"check_task_id":         schema.Int64Attribute{Optional: true, Description: "Explicit completed edit-check task ID. Obtain and review a resource check for this exact draft using the same API principal before applying. A new ID is submitted once when changed; the backend validates ownership, draft, freshness and evidence. Update only; never automatically runs or trusts a check."},
			"accepted_trust_fields": schema.SetAttribute{Optional: true, ElementType: types.StringType, Description: "Explicit trust observations to accept from a new check_task_id. Only host_key_fingerprint is supported. Omit unless you reviewed and intend to accept the observed host key."},
			"accepted_identity":     schema.BoolAttribute{Optional: true, Description: "Explicitly accept the new external identity observed by a new check_task_id. Omit unless you reviewed the identity change."},
			"health_status":         schema.StringAttribute{Computed: true, Description: "Current resource health: healthy, degraded, critical, or unknown."},
			"last_health_at":        schema.StringAttribute{Computed: true, Description: "Timestamp of the latest resource health observation."},
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
			"engine_id": schema.StringAttribute{
				Description: "Connection engine used for automatic onboarding. Supply engine_id without resource_type to let MisterShell discover the resource type.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplaceIfConfigured(),
				},
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"resource_type": schema.StringAttribute{
				Description: "Optional exact installed resource type. When supplied, MisterShell must discover that type or onboarding fails.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplaceIfConfigured(),
				},
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"external_id": schema.StringAttribute{
				Description: "External identifier discovered by MisterShell. Null is valid for types such as Generic SSH.",
				Computed:    true,
			},
			"location_id": schema.Int64Attribute{
				Description: "Location ID where this resource resides.",
				Required:    true,
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
			"connector_data": schema.DynamicAttribute{
				Description: "Connection parameters as a native Terraform object. The accepted shape comes from the selected engine or resource type in MisterShell's installed type catalog.",
				Required:    true,
			},
			"credential_id": schema.Int64Attribute{
				Description: "Credential ID for connecting to this resource.",
				Required:    true,
			},
			"extra_data": schema.DynamicAttribute{
				Optional:    true,
				Description: "Resource metadata/configuration as a native Terraform object. Omit to read discovered metadata. Configured keys are managed; file_transfer_available is an observation and cannot be configured. Valid writable keys depend on the installed resource type.",
				Computed:    true,
			},
			"is_enabled": schema.BoolAttribute{
				Description: "Whether automatic snapshot collection is enabled for this resource. Defaults to true. Disabling it does not prevent manual sessions or operations.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
			},
			"sshgw_allow_personal_key": schema.BoolAttribute{
				Description: "Whether a user's own SSH key, registered in MisterShell under My Account > Account Security > My SSH keys, is accepted on its own in place of the account password and the second authentication factor (MFA) when that user opens a direct native SSH session to this resource through the MisterShell SSH gateway, so non-interactive automation can connect. The account must still be active, and the resource credential, session policy, session recording, and audit logging still apply; deleting the key or deactivating the user ends access. Defaults to false. Only resource types with a shell session mode accept true; MisterShell rejects it on graphical-only types with HTTP 400. Requires MisterShell >= 2.0.15: older backends reject any create or update that sends it with HTTP 422.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
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

func (r *NetworkResourceResource) ConfigValidators(_ context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		resourcevalidator.AtLeastOneOf(path.MatchRoot("engine_id"), path.MatchRoot("resource_type")),
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

	if !plan.CheckTaskID.IsNull() && !plan.CheckTaskID.IsUnknown() || plan.AcceptedIdentity.ValueBool() || len(plan.AcceptedTrustFields.Elements()) > 0 {
		resp.Diagnostics.AddError("Edit evidence requires an existing resource", "Create the resource first, then review a completed edit check and configure check_task_id to accept its evidence.")
		return
	}
	createCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	engineID, expectedTypeID, err := r.resolveOnboardingMode(createCtx, &plan)
	if err != nil {
		resp.Diagnostics.AddError("Invalid resource onboarding configuration", err.Error())
		return
	}
	connectorData, err := dynamicToRawJSON(plan.ConnectorData)
	if err != nil {
		resp.Diagnostics.AddError("Error encoding resource connector_data", err.Error())
		return
	}
	credential, err := r.client.GetCredential(createCtx, plan.CredentialID.ValueInt64())
	if err != nil && !client.IsForbidden(err) {
		resp.Diagnostics.AddError("Error reading onboarding credential", err.Error())
		return
	}
	// Only external platform credentials need name-based reference inputs.
	// Older backends reject this field and omit source in credential responses.
	resourceName := ""
	// Discovery itself checks credential access. Existing resource operators may
	// execute with a supplied credential ID without permission to read metadata.
	if credential != nil && credential.Source == "external" {
		resourceName = plan.Name.ValueString()
	}
	discovery, err := r.client.DiscoverNetworkResource(createCtx, client.DiscoveryTestInput{
		ResourceName:   resourceName,
		EngineID:       engineID,
		ExpectedTypeID: expectedTypeID,
		CredentialID:   plan.CredentialID.ValueInt64(),
		ConnectorData:  connectorData,
		LocationID:     plan.LocationID.ValueInt64(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error onboarding network resource", err.Error())
		return
	}

	input := client.NetworkResourceCreateInput{
		IsEnabled:      boolValueToPtr(plan.IsEnabled),
		Name:           plan.Name.ValueString(),
		ResourceType:   discovery.ResourceType,
		DiscoveryToken: discovery.DiscoveryToken,
		LocationID:     plan.LocationID.ValueInt64(),
		ConnectorData:  connectorData,
	}
	input.CredentialID = int64ValueToPtr(plan.CredentialID)
	// false is the server default and the field is unknown to MisterShell
	// < 2.0.15 (extra fields are rejected with HTTP 422), so it is only sent
	// when the user opts in; that keeps creates working on older backends.
	if plan.SshgwAllowPersonalKey.ValueBool() {
		input.SshgwAllowPersonalKey = boolValueToPtr(plan.SshgwAllowPersonalKey)
	}

	res, err := r.client.CreateNetworkResource(createCtx, input)
	if err != nil {
		resp.Diagnostics.AddError("Error creating network resource", err.Error())
		return
	}

	configuredExtraData := plan.ExtraData
	configuredConnectorData := plan.ConnectorData
	if err := mapNetworkResourceResponseToModel(res, &plan); err != nil {
		resp.Diagnostics.AddError("Error decoding network resource", err.Error())
		return
	}
	plan.EngineID = types.StringValue(discovery.EngineID)
	plan.ConnectorData = configuredConnectorData
	plan.Tags = types.ListNull(tagObjectType)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !configuredExtraData.IsNull() && !configuredExtraData.IsUnknown() {
		raw, err := dynamicToRawJSON(configuredExtraData)
		if err != nil {
			resp.Diagnostics.AddError("Error encoding resource extra_data", err.Error())
			return
		}
		updated, err := r.client.UpdateNetworkResource(createCtx, res.ID, client.NetworkResourceUpdateInput{ExtraData: raw})
		if err != nil {
			resp.Diagnostics.AddError("Error configuring resource extra_data", err.Error())
			return
		}
		plan.ExtraData = configuredExtraData
		if err := mapNetworkResourceResponseToModel(updated, &plan); err != nil {
			resp.Diagnostics.AddError("Error decoding resource", err.Error())
			return
		}
	}
	if err := r.reconcileTags(createCtx, res.ID, &plan); err != nil {
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

	if err := mapNetworkResourceResponseToModel(res, &state); err != nil {
		resp.Diagnostics.AddError("Error decoding network resource", err.Error())
		return
	}
	resourceType, err := r.client.FindResourceType(ctx, res.ResourceType)
	if err != nil {
		resp.Diagnostics.AddError("Error resolving network resource engine", err.Error())
		return
	}
	state.EngineID = types.StringValue(resourceType.EngineID)
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

	input, err := buildNetworkResourceUpdate(&plan, &state)
	if err != nil {
		resp.Diagnostics.AddError("Error encoding resource update", err.Error())
		return
	}

	res, err := r.client.UpdateNetworkResource(ctx, state.ID.ValueInt64(), input)
	if err != nil {
		resp.Diagnostics.AddError("Error updating network resource", err.Error())
		return
	}

	// Successful checks can enrich writable metadata with newly discovered values.
	// Reapply explicitly planned metadata so acceptance cannot silently replace
	// Terraform configuration. The backend deliberately invalidates the check's
	// applicability when this changes metadata; the accepted identity/pin remains.
	if !plan.ExtraData.IsNull() && !plan.ExtraData.IsUnknown() {
		refreshed, err := tfjson.Reconcile(plan.ExtraData, res.ExtraData, nil)
		if err != nil {
			resp.Diagnostics.AddError("Error decoding resource extra_data", err.Error())
			return
		}
		if !refreshed.Equal(plan.ExtraData) {
			// Preserve completed evidence in partial state before the follow-up write,
			// so a failed metadata PATCH does not replay a consumed/stale check on retry.
			partial := plan
			if err := mapNetworkResourceResponseToModel(res, &partial); err != nil {
				resp.Diagnostics.AddError("Error decoding network resource", err.Error())
				return
			}
			partial.EngineID = state.EngineID
			partial.Tags = state.Tags
			resp.Diagnostics.Append(resp.State.Set(ctx, &partial)...)
			if resp.Diagnostics.HasError() {
				return
			}
			raw, err := dynamicToRawJSON(plan.ExtraData)
			if err != nil {
				resp.Diagnostics.AddError("Error encoding resource extra_data", err.Error())
				return
			}
			res, err = r.client.UpdateNetworkResource(ctx, state.ID.ValueInt64(), client.NetworkResourceUpdateInput{ExtraData: raw})
			if err != nil {
				resp.Diagnostics.AddError("Error restoring configured resource extra_data", err.Error())
				return
			}
		}
	}

	configuredConnectorData := plan.ConnectorData
	if err := mapNetworkResourceResponseToModel(res, &plan); err != nil {
		resp.Diagnostics.AddError("Error decoding network resource", err.Error())
		return
	}
	plan.EngineID = state.EngineID
	plan.ConnectorData = configuredConnectorData
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

func mapNetworkResourceResponseToModel(res *client.NetworkResourceResponse, m *NetworkResourceResourceModel) error {
	m.ID = types.Int64Value(res.ID)
	m.Name = types.StringValue(res.Name)
	m.ResourceType = types.StringValue(res.ResourceType)
	m.ExternalID = stringPtrToValue(res.ExternalID)
	m.LocationID = int64PtrToValue(res.LocationID)
	if res.EffectiveCoordinates == nil {
		m.EffectiveLatitude = types.Float64Null()
		m.EffectiveLongitude = types.Float64Null()
		m.CoordinateSourceID = types.Int64Null()
		m.CoordinateSourceName = types.StringNull()
	} else {
		m.EffectiveLatitude = types.Float64Value(res.EffectiveCoordinates.Latitude)
		m.EffectiveLongitude = types.Float64Value(res.EffectiveCoordinates.Longitude)
		m.CoordinateSourceID = int64PtrToValue(res.EffectiveCoordinates.SourceLocationID)
		m.CoordinateSourceName = stringPtrToValue(res.EffectiveCoordinates.SourceLocationName)
	}
	m.CredentialID = int64PtrToValue(res.CredentialID)
	var err error
	m.ConnectorData, err = tfjson.Reconcile(m.ConnectorData, res.ConnectorData, nil)
	if err != nil {
		return fmt.Errorf("connector_data: %w", err)
	}
	m.HealthStatus = types.StringValue(res.HealthStatus)
	m.LastHealthAt = optStringPtrToValue(res.LastHealthAt)
	m.ExtraData, err = tfjson.Reconcile(m.ExtraData, res.ExtraData, nil)
	if err != nil {
		return fmt.Errorf("extra_data: %w", err)
	}
	m.IsEnabled = types.BoolValue(res.IsEnabled)
	m.SshgwAllowPersonalKey = types.BoolValue(res.SshgwAllowPersonalKey)
	m.ConnectorID = types.StringValue(res.ConnectorID)
	m.Status = types.StringValue(res.Status)
	m.CreatedAt = types.StringValue(res.CreatedAt)
	m.UpdatedAt = types.StringValue(res.UpdatedAt)
	m.LastConnectivityCheck = optStringPtrToValue(res.LastConnectivityCheck)
	m.LastConnectivityResult, err = rawJSONToDynamic(res.LastConnectivityResult)
	if err != nil {
		return fmt.Errorf("last_connectivity_result: %w", err)
	}
	m.LastCollectionAt = optStringPtrToValue(res.LastCollectionAt)
	m.NextCollectionAt = optStringPtrToValue(res.NextCollectionAt)
	return nil
}

func (r *NetworkResourceResource) resolveOnboardingMode(ctx context.Context, plan *NetworkResourceResourceModel) (string, *string, error) {
	hasEngine := !plan.EngineID.IsNull() && !plan.EngineID.IsUnknown()
	hasType := !plan.ResourceType.IsNull() && !plan.ResourceType.IsUnknown()
	if !hasEngine && !hasType {
		return "", nil, fmt.Errorf("at least one of engine_id or resource_type must be configured")
	}
	if !hasType {
		return plan.EngineID.ValueString(), nil, nil
	}

	typeID := plan.ResourceType.ValueString()
	resourceType, err := r.client.FindResourceType(ctx, typeID)
	if err != nil {
		return "", nil, err
	}
	if hasEngine && plan.EngineID.ValueString() != resourceType.EngineID {
		return "", nil, fmt.Errorf("resource type %q uses engine %q, not configured engine %q", typeID, resourceType.EngineID, plan.EngineID.ValueString())
	}
	return resourceType.EngineID, &typeID, nil
}

func buildNetworkResourceUpdate(plan, state *NetworkResourceResourceModel) (client.NetworkResourceUpdateInput, error) {
	input := client.NetworkResourceUpdateInput{}
	if !plan.ExtraData.IsNull() && !plan.ExtraData.IsUnknown() && !plan.ExtraData.Equal(state.ExtraData) {
		var err error
		input.ExtraData, err = dynamicToRawJSON(plan.ExtraData)
		if err != nil {
			return input, err
		}
	}
	evidenceChanged := !plan.CheckTaskID.Equal(state.CheckTaskID) && !plan.CheckTaskID.IsNull() && !plan.CheckTaskID.IsUnknown()
	if evidenceChanged {
		// Evidence binds to submitted inputs, including the name and omitted
		// defaults. Send the exact configured draft even when fields did not
		// change; the saved discovery provenance may differ from these inputs.
		input.Name = stringValueToPtr(plan.Name)
		input.LocationID = int64ValueToPtr(plan.LocationID)
		input.CredentialID = int64ValueToPtr(plan.CredentialID)
		var err error
		input.ConnectorData, err = dynamicToRawJSON(plan.ConnectorData)
		if err != nil {
			return input, err
		}
		input.CheckTaskID = int64ValueToPtr(plan.CheckTaskID)
		for _, v := range plan.AcceptedTrustFields.Elements() {
			if s, ok := v.(types.String); ok {
				input.AcceptedTrustFields = append(input.AcceptedTrustFields, s.ValueString())
			}
		}
		if plan.AcceptedIdentity.ValueBool() {
			input.AcceptedIdentity = boolValueToPtr(plan.AcceptedIdentity)
		}
	} else if (!plan.AcceptedTrustFields.Equal(state.AcceptedTrustFields) && len(plan.AcceptedTrustFields.Elements()) > 0) || (!plan.AcceptedIdentity.Equal(state.AcceptedIdentity) && plan.AcceptedIdentity.ValueBool()) {
		return input, fmt.Errorf("accepting trust or identity requires a new check_task_id; obtain and review a fresh edit check")
	}
	if !plan.Name.Equal(state.Name) {
		value := plan.Name.ValueString()
		input.Name = &value
	}
	if !plan.LocationID.Equal(state.LocationID) {
		value := plan.LocationID.ValueInt64()
		input.LocationID = &value
	}
	if !plan.ConnectorData.Equal(state.ConnectorData) {
		connectorData, err := dynamicToRawJSON(plan.ConnectorData)
		if err != nil {
			return client.NetworkResourceUpdateInput{}, fmt.Errorf("connector_data: %w", err)
		}
		input.ConnectorData = connectorData
	}
	if !plan.CredentialID.Equal(state.CredentialID) {
		input.CredentialID = int64ValueToPtr(plan.CredentialID)
	}
	if !plan.IsEnabled.Equal(state.IsEnabled) {
		value := plan.IsEnabled.ValueBool()
		input.IsEnabled = &value
	}
	if !plan.SshgwAllowPersonalKey.Equal(state.SshgwAllowPersonalKey) {
		value := plan.SshgwAllowPersonalKey.ValueBool()
		input.SshgwAllowPersonalKey = &value
	}
	return input, nil
}

func (r *NetworkResourceResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config NetworkResourceResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !config.CheckTaskID.IsNull() && !config.CheckTaskID.IsUnknown() && config.CheckTaskID.ValueInt64() < 1 {
		resp.Diagnostics.AddAttributeError(path.Root("check_task_id"), "Invalid check evidence", "check_task_id must be positive.")
	}
	accepts := config.AcceptedIdentity.ValueBool() || len(config.AcceptedTrustFields.Elements()) > 0
	if accepts && config.CheckTaskID.IsNull() {
		resp.Diagnostics.AddAttributeError(path.Root("check_task_id"), "Missing check evidence", "Explicit trust or identity acceptance requires check_task_id from a completed, reviewed resource edit check.")
	}
	for _, v := range config.AcceptedTrustFields.Elements() {
		if s, ok := v.(types.String); ok && !s.IsUnknown() && s.ValueString() != "host_key_fingerprint" {
			resp.Diagnostics.AddAttributeError(path.Root("accepted_trust_fields"), "Invalid trust field", "Only host_key_fingerprint can be explicitly accepted.")
		}
	}
	if !config.ExtraData.IsNull() && !config.ExtraData.IsUnknown() {
		raw, err := dynamicToRawJSON(config.ExtraData)
		if err != nil {
			return
		}
		var obj map[string]json.RawMessage
		if json.Unmarshal(raw, &obj) != nil || obj == nil {
			resp.Diagnostics.AddAttributeError(path.Root("extra_data"), "Invalid extra_data", "extra_data must be an object.")
			return
		}
		if _, ok := obj["file_transfer_available"]; ok {
			resp.Diagnostics.AddAttributeError(path.Root("extra_data"), "Read-only observation", "file_transfer_available is discovered by MisterShell and cannot be configured.")
		}
	}
}
