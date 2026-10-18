package resources

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-mistershell/internal/client"
)

var (
	_ resource.Resource                = &RoleResource{}
	_ resource.ResourceWithImportState = &RoleResource{}
)

type RoleResource struct {
	client *client.Client
}

type RoleResourceModel struct {
	IsBuiltin         types.Bool   `tfsdk:"is_builtin"`
	ID                types.Int64  `tfsdk:"id"`
	Name              types.String `tfsdk:"name"`
	Description       types.String `tfsdk:"description"`
	LocationScopeMode types.String `tfsdk:"location_scope_mode"`
	ScopeLocationIDs  types.Set    `tfsdk:"scope_location_ids"`
	Permissions       types.Set    `tfsdk:"permissions"`
	CreatedAt         types.String `tfsdk:"created_at"`
	UpdatedAt         types.String `tfsdk:"updated_at"`
}

func NewRoleResource() resource.Resource {
	return &RoleResource{}
}

func (r *RoleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_role"
}

func (r *RoleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a MisterShell role. A role groups a set of permission names and an optional location scope. Permission names are validated by the backend (no static enum). The superuser permission '*.*.*' cannot be combined with other permissions or with a location scope.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Description: "Role ID.",
				Computed:    true,
				PlanModifiers: []planmodifier.Int64{
					int64UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Role name (1-100 characters).",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, 100),
				},
			},
			"description": schema.StringAttribute{
				Description: "Role description (up to 500 characters).",
				Optional:    true,
				Validators: []validator.String{
					stringvalidator.LengthAtMost(500),
				},
			},
			"location_scope_mode": schema.StringAttribute{
				Description: "Location scope mode: global applies everywhere; restricted applies only beneath scope_location_ids.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("global"),
				Validators: []validator.String{
					stringvalidator.OneOf("global", "restricted"),
				},
			},
			"scope_location_ids": schema.SetAttribute{
				Description: "Location scope roots. Required and non-empty when location_scope_mode is restricted; empty for global roles.",
				Optional:    true,
				ElementType: types.Int64Type,
			},
			"is_builtin": schema.BoolAttribute{Computed: true, Description: "Whether this is a built-in role."},
			"permissions": schema.SetAttribute{
				Description: "Complete permission set assigned to this role. Omitting this attribute removes all grants. The backend validates names and superuser rules.",
				Optional:    true,
				ElementType: types.StringType,
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

func (r *RoleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *RoleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan RoleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	input := client.RoleCreateInput{
		Permissions:       stringSetToSlice(plan.Permissions),
		Name:              plan.Name.ValueString(),
		LocationScopeMode: plan.LocationScopeMode.ValueString(),
	}
	if !plan.Description.IsNull() {
		v := plan.Description.ValueString()
		input.Description = &v
	}
	if !plan.ScopeLocationIDs.IsNull() && !plan.ScopeLocationIDs.IsUnknown() {
		input.ScopeLocationIDs = int64SetToSlice(plan.ScopeLocationIDs)
	}

	role, err := r.client.CreateRole(ctx, input)
	if err != nil {
		resp.Diagnostics.AddError("Error creating role", err.Error())
		return
	}

	mapRoleBaseResponseToModel(role, &plan)
	if plan.Permissions.IsUnknown() {
		plan.Permissions = stringSliceToSet(nil)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.finalizeState(ctx, role.ID, &plan); err != nil {
		resp.Diagnostics.AddError("Error reading role after create", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *RoleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state RoleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	role, err := r.client.GetRole(ctx, state.ID.ValueInt64())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading role", err.Error())
		return
	}

	mapRoleResponseToModel(role, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *RoleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan RoleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state RoleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := state.ID.ValueInt64()

	input := client.RoleUpdateInput{}
	if !plan.Name.Equal(state.Name) {
		v := plan.Name.ValueString()
		input.Name = &v
	}
	input.Description = stringValueToPtr(plan.Description)
	if !plan.LocationScopeMode.Equal(state.LocationScopeMode) {
		value := plan.LocationScopeMode.ValueString()
		input.LocationScopeMode = &value
	}
	if !plan.ScopeLocationIDs.Equal(state.ScopeLocationIDs) {
		ids := int64SetToSlice(plan.ScopeLocationIDs)
		if ids == nil {
			ids = []int64{}
		}
		input.ScopeLocationIDs = &ids
	}

	if !plan.Permissions.IsUnknown() && !plan.Permissions.Equal(state.Permissions) {
		permissions := stringSetToSlice(plan.Permissions)
		if permissions == nil {
			permissions = []string{}
		}
		input.Permissions = &permissions
	}
	if _, err := r.client.UpdateRole(ctx, id, input); err != nil {
		resp.Diagnostics.AddError("Error updating role", err.Error())
		return
	}

	if err := r.finalizeState(ctx, id, &plan); err != nil {
		resp.Diagnostics.AddError("Error reading role after update", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *RoleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state RoleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteRole(ctx, state.ID.ValueInt64()); err != nil {
		resp.Diagnostics.AddError("Error deleting role", err.Error())
	}
}

func (r *RoleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("Expected integer ID, got: %s", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), types.Int64Value(id))...)
}

// finalizeState re-GETs the role (RoleDetailResponse, has permissions) to populate model state.
func (r *RoleResource) finalizeState(ctx context.Context, id int64, m *RoleResourceModel) error {
	role, err := r.client.GetRole(ctx, id)
	if err != nil {
		return err
	}
	mapRoleResponseToModel(role, m)
	return nil
}

func mapRoleResponseToModel(role *client.RoleDetailResponse, m *RoleResourceModel) {
	mapRoleBaseResponseToModel(&role.RoleResponse, m)
	if m.Permissions.IsNull() && len(role.Permissions) == 0 {
		m.Permissions = types.SetNull(types.StringType)
	} else {
		m.Permissions = stringSliceToSet(role.Permissions)
	}
}

func mapRoleBaseResponseToModel(role *client.RoleResponse, m *RoleResourceModel) {
	m.IsBuiltin = types.BoolValue(role.IsBuiltin)
	m.ID = types.Int64Value(role.ID)
	m.Name = types.StringValue(role.Name)
	m.Description = stringPtrToValue(role.Description)
	m.LocationScopeMode = types.StringValue(role.LocationScopeMode)
	// scope_location_ids is Optional (not Computed): when the config omits it the
	// server returns an empty list (= all locations); keep state null to match config.
	if (m.ScopeLocationIDs.IsNull() || m.ScopeLocationIDs.IsUnknown()) && len(role.ScopeLocationIDs) == 0 {
		m.ScopeLocationIDs = types.SetNull(types.Int64Type)
	} else {
		m.ScopeLocationIDs = int64SliceToSet(role.ScopeLocationIDs)
	}
	m.CreatedAt = types.StringValue(role.CreatedAt)
	m.UpdatedAt = types.StringValue(role.UpdatedAt)
}
