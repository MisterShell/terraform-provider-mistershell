package resources

import (
	"context"
	"fmt"
	"strconv"
	"strings"

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
	ID             types.Int64  `tfsdk:"id"`
	Email          types.String `tfsdk:"email"`
	FirstName      types.String `tfsdk:"first_name"`
	LastName       types.String `tfsdk:"last_name"`
	Password       types.String `tfsdk:"password"`
	AuthType       types.String `tfsdk:"auth_type"`
	AuthProviderID types.Int64  `tfsdk:"auth_provider_id"`
	IsActive       types.Bool   `tfsdk:"is_active"`
	EmailVerified  types.Bool   `tfsdk:"email_verified"`
	Roles          types.List   `tfsdk:"roles"`
	Permissions    types.Set    `tfsdk:"permissions"`
	CreatedAt      types.String `tfsdk:"created_at"`
	UpdatedAt      types.String `tfsdk:"updated_at"`
}

func NewUserResource() resource.Resource {
	return &UserResource{}
}

func (r *UserResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

func (r *UserResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a MisterShell user. Role assignments are managed separately via mistershell_user_role; this resource exposes assigned roles read-only. password is required when creating a user, and may be omitted after import or to stop managing it. Only LOCAL users support password rotation. Import by integer ID.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Description: "User ID.",
				Computed:    true,
				PlanModifiers: []planmodifier.Int64{
					int64UseStateForUnknown(),
				},
			},
			"email": schema.StringAttribute{
				Description: "Canonical lowercase user email address (unique). Changing it forces replacement.",
				Validators:  []validator.String{canonicalEmailValidator{}},
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
				Description: "Required when creating any user. Omit after import or to stop password management. Changes rotate LOCAL passwords; external users cannot rotate passwords. Stored in state; never returned by the API.",
				Optional:    true,
				Validators:  []validator.String{stringvalidator.LengthAtLeast(8)},
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
			"auth_provider_id": schema.Int64Attribute{
				Description: "ID of the external authentication provider (mistershell_auth_provider) the user signs in through; null for LOCAL users and for users not yet linked to a provider. Read-only; null on MisterShell < 2.0.9.",
				Computed:    true,
				PlanModifiers: []planmodifier.Int64{
					int64UseStateForUnknown(),
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
						"id":                  schema.Int64Attribute{Computed: true, Description: "Role ID."},
						"name":                schema.StringAttribute{Computed: true, Description: "Role name."},
						"description":         schema.StringAttribute{Computed: true, Description: "Role description."},
						"is_builtin":          schema.BoolAttribute{Computed: true, Description: "Whether this is a built-in role."},
						"location_scope_mode": schema.StringAttribute{Computed: true, Description: "Role location scope mode."},
						"scope_location_ids":  schema.SetAttribute{Computed: true, ElementType: types.Int64Type, Description: "Role location scope roots."},
					},
				},
			},
			"permissions": schema.SetAttribute{
				Description: "Flattened permission names granted through assigned roles (read-only).",
				Computed:    true,
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

	if plan.Password.IsNull() || plan.Password.IsUnknown() {
		resp.Diagnostics.AddAttributeError(path.Root("password"), "Password required for creation", "The API requires a password when creating any user, including external users. Imported users may omit password.")
		return
	}

	input := client.UserCreateInput{
		Email:     plan.Email.ValueString(),
		FirstName: plan.FirstName.ValueString(),
		LastName:  plan.LastName.ValueString(),
		AuthType:  plan.AuthType.ValueString(),
		IsActive:  plan.IsActive.ValueBool(),
	}
	input.Password = plan.Password.ValueString()

	created, err := r.client.CreateUser(ctx, input)
	if err != nil {
		resp.Diagnostics.AddError("Error creating user", err.Error())
		return
	}

	mapUserBaseResponseToModel(created, &plan)
	plan.Roles = types.ListValueMust(roleObjectType, nil)
	plan.Permissions = types.SetValueMust(types.StringType, nil)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
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

	rotatePassword := !plan.Password.IsNull() && !plan.Password.IsUnknown() && !plan.Password.Equal(state.Password)
	if rotatePassword && state.AuthType.ValueString() != "LOCAL" {
		resp.Diagnostics.AddAttributeError(path.Root("password"), "Password rotation requires LOCAL authentication", "External users cannot rotate passwords. Omit password to leave it unmanaged.")
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
	if rotatePassword {
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

// The backend canonicalizes email to lowercase; rejecting noncanonical input
// avoids changing a configured immutable attribute during apply.
type canonicalEmailValidator struct{}

func (canonicalEmailValidator) Description(context.Context) string {
	return "Email must be lowercase without surrounding whitespace."
}
func (v canonicalEmailValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}
func (v canonicalEmailValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	email := req.ConfigValue.ValueString()
	if email != strings.ToLower(strings.TrimSpace(email)) {
		resp.Diagnostics.AddAttributeError(req.Path, "Noncanonical email", v.Description(ctx))
	}
}
