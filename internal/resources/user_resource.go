package resources

import (
	"context"
	"fmt"
	"strconv"

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
	_ resource.Resource                = &UserResource{}
	_ resource.ResourceWithImportState = &UserResource{}
)

type UserResource struct {
	client *client.Client
}

type UserResourceModel struct {
	ID            types.Int64  `tfsdk:"id"`
	Email         types.String `tfsdk:"email"`
	FirstName     types.String `tfsdk:"first_name"`
	LastName      types.String `tfsdk:"last_name"`
	Password      types.String `tfsdk:"password"`
	AuthType      types.String `tfsdk:"auth_type"`
	IsActive      types.Bool   `tfsdk:"is_active"`
	EmailVerified types.Bool   `tfsdk:"email_verified"`
	Roles         types.List   `tfsdk:"roles"`
	CreatedAt     types.String `tfsdk:"created_at"`
	UpdatedAt     types.String `tfsdk:"updated_at"`
}

func NewUserResource() resource.Resource {
	return &UserResource{}
}

func (r *UserResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

func (r *UserResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a MisterShell user. Role assignments are managed separately via mistershell_user_role; this resource exposes assigned roles read-only. password is required for all auth types (the backend hashes it and enforces its own password policy); changing it rotates the password via the admin reset endpoint. Import by integer ID.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Description: "User ID.",
				Computed:    true,
				PlanModifiers: []planmodifier.Int64{
					int64UseStateForUnknown(),
				},
			},
			"email": schema.StringAttribute{
				Description: "User email address (unique). Changing it forces replacement.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"first_name": schema.StringAttribute{
				Description: "User first name.",
				Required:    true,
			},
			"last_name": schema.StringAttribute{
				Description: "User last name.",
				Required:    true,
			},
			"password": schema.StringAttribute{
				Description: "User password. Required for all auth types: the backend hashes it and enforces its own password policy (complexity rules apply to LOCAL users only). Stored in state (sensitive); the API never returns it. Changing this value rotates the password via the admin reset endpoint.",
				Required:    true,
				Sensitive:   true,
			},
			"auth_type": schema.StringAttribute{
				Description: "Authentication type. One of LOCAL, LDAP, OIDC, SAML. Defaults to LOCAL. A password is required for all types; complexity rules are enforced for LOCAL only.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("LOCAL"),
				Validators: []validator.String{
					// Hand-maintained: AuthType is not an OpenAPI enum, so it is
					// not generated into types_gen.go.
					stringvalidator.OneOf("LOCAL", "LDAP", "OIDC", "SAML"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"is_active": schema.BoolAttribute{
				Description: "Whether the user account is active. Defaults to true.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
			},
			"email_verified": schema.BoolAttribute{
				Description: "Whether the user's email is verified (read-only).",
				Computed:    true,
			},
			"roles": schema.ListNestedAttribute{
				Description: "Roles assigned to this user, as objects with id, name, and description (read-only; manage assignments via mistershell_user_role).",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":          schema.Int64Attribute{Computed: true, Description: "Role ID."},
						"name":        schema.StringAttribute{Computed: true, Description: "Role name."},
						"description": schema.StringAttribute{Computed: true, Description: "Role description."},
					},
				},
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

func (r *UserResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *UserResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan UserResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	input := client.UserCreateInput{
		Email:     plan.Email.ValueString(),
		FirstName: plan.FirstName.ValueString(),
		LastName:  plan.LastName.ValueString(),
		AuthType:  plan.AuthType.ValueString(),
	}
	input.Password = plan.Password.ValueString()

	created, err := r.client.CreateUser(ctx, input)
	if err != nil {
		resp.Diagnostics.AddError("Error creating user", err.Error())
		return
	}

	// UserCreate has no is_active field; if the plan set it to a non-default,
	// apply it with a follow-up PATCH.
	if !plan.IsActive.IsNull() && !plan.IsActive.IsUnknown() && plan.IsActive.ValueBool() != created.IsActive {
		active := plan.IsActive.ValueBool()
		if _, err := r.client.UpdateUser(ctx, created.ID, client.UserUpdateInput{IsActive: &active}); err != nil {
			resp.Diagnostics.AddError("Error setting user is_active", err.Error())
			return
		}
	}

	full, err := r.client.GetUser(ctx, created.ID)
	if err != nil {
		resp.Diagnostics.AddError("Error reading user after create", err.Error())
		return
	}
	mapUserResponseToModel(full, &plan)
	// Preserve the configured password in state (API never returns it).
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *UserResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state UserResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	user, err := r.client.GetUser(ctx, state.ID.ValueInt64())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading user", err.Error())
		return
	}

	// mapUserResponseToModel does not touch Password; state keeps the prior value.
	mapUserResponseToModel(user, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *UserResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan UserResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state UserResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := state.ID.ValueInt64()

	input := client.UserUpdateInput{}
	if !plan.FirstName.Equal(state.FirstName) {
		v := plan.FirstName.ValueString()
		input.FirstName = &v
	}
	if !plan.LastName.Equal(state.LastName) {
		v := plan.LastName.ValueString()
		input.LastName = &v
	}
	if !plan.IsActive.Equal(state.IsActive) {
		v := plan.IsActive.ValueBool()
		input.IsActive = &v
	}
	if input.FirstName != nil || input.LastName != nil || input.IsActive != nil {
		if _, err := r.client.UpdateUser(ctx, id, input); err != nil {
			resp.Diagnostics.AddError("Error updating user", err.Error())
			return
		}
	}

	// Rotate password when it changed.
	if !plan.Password.Equal(state.Password) {
		if err := r.client.ResetUserPassword(ctx, id, plan.Password.ValueString()); err != nil {
			resp.Diagnostics.AddError("Error resetting user password", err.Error())
			return
		}
	}

	full, err := r.client.GetUser(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Error reading user after update", err.Error())
		return
	}
	mapUserResponseToModel(full, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *UserResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state UserResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteUser(ctx, state.ID.ValueInt64()); err != nil {
		resp.Diagnostics.AddError("Error deleting user", err.Error())
	}
}

func (r *UserResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("Expected integer ID, got: %s", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), types.Int64Value(id))...)
}
