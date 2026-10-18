package datasources

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-mistershell/internal/client"
)

var _ datasource.DataSource = &UserDataSource{}

type UserDataSource struct {
	client *client.Client
}

type UserDataSourceModel struct {
	ID             types.Int64  `tfsdk:"id"`
	Email          types.String `tfsdk:"email"`
	FirstName      types.String `tfsdk:"first_name"`
	LastName       types.String `tfsdk:"last_name"`
	AuthType       types.String `tfsdk:"auth_type"`
	AuthProviderID types.Int64  `tfsdk:"auth_provider_id"`
	IsActive       types.Bool   `tfsdk:"is_active"`
	EmailVerified  types.Bool   `tfsdk:"email_verified"`
	Roles          types.List   `tfsdk:"roles"`
	Permissions    types.Set    `tfsdk:"permissions"`
	CreatedAt      types.String `tfsdk:"created_at"`
	UpdatedAt      types.String `tfsdk:"updated_at"`
}

func NewUserDataSource() datasource.DataSource {
	return &UserDataSource{}
}

func (d *UserDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

func (d *UserDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a MisterShell user. Look up by id for a direct fetch, or by email (exact match, must resolve to exactly one user).",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Description: "User ID. Use for direct lookup.",
				Optional:    true,
				Computed:    true,
			},
			"email": schema.StringAttribute{
				Description: "User email. Used as an exact-match search filter when id is not specified.",
				Optional:    true,
				Computed:    true,
			},
			"first_name":       schema.StringAttribute{Description: "User first name.", Computed: true},
			"last_name":        schema.StringAttribute{Description: "User last name.", Computed: true},
			"auth_type":        schema.StringAttribute{Description: "Authentication type (LOCAL, LDAP, OIDC, SAML).", Computed: true},
			"auth_provider_id": schema.Int64Attribute{Description: "ID of the external authentication provider the user signs in through; null for LOCAL users (and on MisterShell < 2.0.9).", Computed: true},
			"is_active":        schema.BoolAttribute{Description: "Whether the account is active.", Computed: true},
			"email_verified": schema.BoolAttribute{
				Description: "Whether the user's email is verified.",
				Computed:    true,
			},
			"roles": schema.ListNestedAttribute{
				Description: "Roles assigned to this user, as objects with id, name, and description.",
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
				Description: "Flattened permission names granted through assigned roles.",
				Computed:    true,
				ElementType: types.StringType,
			},
			"created_at": schema.StringAttribute{Description: "Creation timestamp.", Computed: true},
			"updated_at": schema.StringAttribute{Description: "Last update timestamp.", Computed: true},
		},
	}
}

func (d *UserDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *UserDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config UserDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	hasID := !config.ID.IsNull() && !config.ID.IsUnknown()

	var user *client.UserDetailResponse
	var err error

	if hasID {
		user, err = d.client.GetUser(ctx, config.ID.ValueInt64())
		if err != nil {
			resp.Diagnostics.AddError("Error reading user", err.Error())
			return
		}
	} else {
		hasEmail := !config.Email.IsNull() && !config.Email.IsUnknown()
		if !hasEmail {
			resp.Diagnostics.AddError("Missing lookup criteria", "Specify 'id' for direct lookup, or 'email' to search.")
			return
		}
		email := strings.ToLower(strings.TrimSpace(config.Email.ValueString()))
		users, lerr := d.client.ListUsers(ctx, client.UserListFilter{Search: client.SearchPrefix(email)})
		if lerr != nil {
			resp.Diagnostics.AddError("Error searching users", lerr.Error())
			return
		}
		var matched []client.UserDetailResponse
		for i := range users {
			if strings.EqualFold(users[i].Email, email) {
				matched = append(matched, users[i])
			}
		}
		if len(matched) == 0 {
			resp.Diagnostics.AddError("No matching user found", "No user matches the specified email.")
			return
		}
		if len(matched) > 1 {
			resp.Diagnostics.AddError("Multiple users found",
				fmt.Sprintf("Found %d users matching the email. Emails should be unique.", len(matched)))
			return
		}
		user, err = d.client.GetUser(ctx, matched[0].ID)
		if err != nil {
			resp.Diagnostics.AddError("Error reading user", err.Error())
			return
		}
	}

	config.ID = types.Int64Value(user.ID)
	config.Email = types.StringValue(user.Email)
	config.FirstName = types.StringValue(user.FirstName)
	config.LastName = types.StringValue(user.LastName)
	config.AuthType = types.StringValue(user.AuthType)
	config.AuthProviderID = int64PtrToValue(user.AuthProviderID)
	config.IsActive = types.BoolValue(user.IsActive)
	config.EmailVerified = types.BoolValue(user.EmailVerified)
	config.Roles = rolesToList(user.Roles)
	config.Permissions = stringSliceToSet(user.Permissions)
	config.CreatedAt = types.StringValue(user.CreatedAt)
	config.UpdatedAt = types.StringValue(user.UpdatedAt)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
