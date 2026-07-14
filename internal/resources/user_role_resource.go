package resources

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-mistershell/internal/client"
)

var (
	_ resource.Resource                = &UserRoleResource{}
	_ resource.ResourceWithImportState = &UserRoleResource{}
)

type UserRoleResource struct {
	client *client.Client
}

type UserRoleResourceModel struct {
	ID       types.String `tfsdk:"id"`
	UserID   types.Int64  `tfsdk:"user_id"`
	RoleID   types.Int64  `tfsdk:"role_id"`
	RoleName types.String `tfsdk:"role_name"`
}

func NewUserRoleResource() resource.Resource {
	return &UserRoleResource{}
}

func (r *UserRoleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user_role"
}

func (r *UserRoleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Assigns one MisterShell role to one user. (user_id, role_id) is unique. Both fields force replacement on change. Import format is \"<user_id>:<role_id>\".",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Synthetic edge ID in the form \"<user_id>:<role_id>\".",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"user_id": schema.Int64Attribute{
				Description: "ID of the user. Cannot be changed after creation.",
				Required:    true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"role_id": schema.Int64Attribute{
				Description: "ID of the role to assign. Cannot be changed after creation.",
				Required:    true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"role_name": schema.StringAttribute{
				Description: "Name of the assigned role (read-only).",
				Computed:    true,
			},
		},
	}
}

func (r *UserRoleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// edgeID builds the synthetic state ID.
func edgeID(userID, roleID int64) string {
	return fmt.Sprintf("%d:%d", userID, roleID)
}

// findRole returns the role with the given id from the user's assigned roles,
// or nil if the edge is absent.
func findRole(user *client.UserDetailResponse, roleID int64) *client.RoleResponse {
	for i := range user.Roles {
		if user.Roles[i].ID == roleID {
			return &user.Roles[i]
		}
	}
	return nil
}

func (r *UserRoleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan UserRoleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	userID := plan.UserID.ValueInt64()
	roleID := plan.RoleID.ValueInt64()

	if err := r.client.AddUserRole(ctx, userID, roleID); err != nil {
		resp.Diagnostics.AddError("Error assigning role to user", err.Error())
		return
	}

	user, err := r.client.GetUser(ctx, userID)
	if err != nil {
		resp.Diagnostics.AddError("Error reading user after assignment", err.Error())
		return
	}
	role := findRole(user, roleID)
	if role == nil {
		resp.Diagnostics.AddError("Role assignment not reflected", fmt.Sprintf("Role %d not present on user %d after assignment.", roleID, userID))
		return
	}

	plan.ID = types.StringValue(edgeID(userID, roleID))
	plan.RoleName = types.StringValue(role.Name)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *UserRoleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state UserRoleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	userID := state.UserID.ValueInt64()
	roleID := state.RoleID.ValueInt64()

	user, err := r.client.GetUser(ctx, userID)
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading user", err.Error())
		return
	}
	role := findRole(user, roleID)
	if role == nil {
		// Edge removed out of band.
		resp.State.RemoveResource(ctx)
		return
	}

	state.ID = types.StringValue(edgeID(userID, roleID))
	state.RoleName = types.StringValue(role.Name)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is unreachable: user_id and role_id both RequiresReplace, and there are
// no other mutable attributes. Implemented to satisfy the interface.
func (r *UserRoleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan UserRoleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *UserRoleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state UserRoleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.RemoveUserRole(ctx, state.UserID.ValueInt64(), state.RoleID.ValueInt64())
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error unassigning role from user", err.Error())
	}
}

func (r *UserRoleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, ":", 2)
	if len(parts) != 2 {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("Expected \"<user_id>:<role_id>\", got: %s", req.ID))
		return
	}
	userID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("Expected integer user_id, got: %s", parts[0]))
		return
	}
	roleID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("Expected integer role_id, got: %s", parts[1]))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("user_id"), types.Int64Value(userID))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("role_id"), types.Int64Value(roleID))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), types.StringValue(edgeID(userID, roleID)))...)
}
