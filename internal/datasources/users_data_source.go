package datasources

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-mistershell/internal/client"
)

var _ datasource.DataSource = &UsersDataSource{}

type UsersDataSource struct {
	client *client.Client
}

type UsersDataSourceModel struct {
	Search types.String `tfsdk:"search"`
	Users  types.List   `tfsdk:"users"`
}

func NewUsersDataSource() datasource.DataSource {
	return &UsersDataSource{}
}

func (d *UsersDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_users"
}

// usersCatalogAttrTypes is the element object type for the catalog `users` list.
var usersCatalogAttrTypes = map[string]attr.Type{
	"id":               types.Int64Type,
	"email":            types.StringType,
	"first_name":       types.StringType,
	"last_name":        types.StringType,
	"auth_type":        types.StringType,
	"auth_provider_id": types.Int64Type,
	"is_active":        types.BoolType,
	"email_verified":   types.BoolType,
}

var usersCatalogObjectType = types.ObjectType{AttrTypes: usersCatalogAttrTypes}

func (d *UsersDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Lists MisterShell users. With no filter it returns every user; set `search` to a substring to narrow the list (server-side fuzzy match on name/email). Use the singular `mistershell_user` data source to look up exactly one user (and its roles).",
		Attributes: map[string]schema.Attribute{
			"search": schema.StringAttribute{
				Description: "Optional case-insensitive substring filter (name/email). Omit to return all users.",
				Optional:    true,
			},
			"users": schema.ListNestedAttribute{
				Description: "All matching users.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":               schema.Int64Attribute{Computed: true, Description: "User ID."},
						"email":            schema.StringAttribute{Computed: true, Description: "User email."},
						"first_name":       schema.StringAttribute{Computed: true, Description: "User first name."},
						"last_name":        schema.StringAttribute{Computed: true, Description: "User last name."},
						"auth_type":        schema.StringAttribute{Computed: true, Description: "Authentication type."},
						"auth_provider_id": schema.Int64Attribute{Computed: true, Description: "ID of the external authentication provider the user signs in through; null for LOCAL users (and on MisterShell < 2.0.9)."},
						"is_active":        schema.BoolAttribute{Computed: true, Description: "Whether the account is active."},
						"email_verified":   schema.BoolAttribute{Computed: true, Description: "Whether the email is verified."},
					},
				},
			},
		},
	}
}

func (d *UsersDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *UsersDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config UsersDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	filter := client.UserListFilter{}
	if !config.Search.IsNull() && !config.Search.IsUnknown() {
		filter.Search = config.Search.ValueString()
	}

	items, err := d.client.ListUsers(ctx, filter)
	if err != nil {
		resp.Diagnostics.AddError("Error listing users", err.Error())
		return
	}

	elems := make([]attr.Value, 0, len(items))
	for _, it := range items {
		obj := types.ObjectValueMust(usersCatalogAttrTypes, map[string]attr.Value{
			"id":               types.Int64Value(it.ID),
			"email":            types.StringValue(it.Email),
			"first_name":       types.StringValue(it.FirstName),
			"last_name":        types.StringValue(it.LastName),
			"auth_type":        types.StringValue(it.AuthType),
			"auth_provider_id": int64PtrToValue(it.AuthProviderID),
			"is_active":        types.BoolValue(it.IsActive),
			"email_verified":   types.BoolValue(it.EmailVerified),
		})
		elems = append(elems, obj)
	}
	config.Users = types.ListValueMust(usersCatalogObjectType, elems)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
