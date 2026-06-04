package datasources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-mistershell/internal/client"
)

var _ datasource.DataSource = &RoleDataSource{}

type RoleDataSource struct {
	client *client.Client
}

type RoleDataSourceModel struct {
	ID               types.Int64  `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	Description      types.String `tfsdk:"description"`
	ScopeLocationIDs types.Set    `tfsdk:"scope_location_ids"`
	Permissions      types.Set    `tfsdk:"permissions"`
	CreatedAt        types.String `tfsdk:"created_at"`
	UpdatedAt        types.String `tfsdk:"updated_at"`
}

func NewRoleDataSource() datasource.DataSource {
	return &RoleDataSource{}
}

func (d *RoleDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_role"
}

func (d *RoleDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a MisterShell role. Look up by id for a direct fetch, or by name (exact match, must resolve to exactly one role).",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Description: "Role ID. Use for direct lookup.",
				Optional:    true,
				Computed:    true,
			},
			"name": schema.StringAttribute{
				Description: "Role name. Used as an exact-match search filter when id is not specified.",
				Optional:    true,
				Computed:    true,
			},
			"description": schema.StringAttribute{
				Description: "Role description.",
				Computed:    true,
			},
			"scope_location_ids": schema.SetAttribute{
				Description: "Set of location IDs that scope this role. Empty means all locations.",
				Computed:    true,
				ElementType: types.Int64Type,
			},
			"permissions": schema.SetAttribute{
				Description: "Set of permission names assigned to this role.",
				Computed:    true,
				ElementType: types.StringType,
			},
			"created_at": schema.StringAttribute{
				Description: "Creation timestamp.",
				Computed:    true,
			},
			"updated_at": schema.StringAttribute{
				Description: "Last update timestamp.",
				Computed:    true,
			},
		},
	}
}

func (d *RoleDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *RoleDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config RoleDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	hasID := !config.ID.IsNull() && !config.ID.IsUnknown()

	var role *client.RoleDetailResponse

	if hasID {
		var err error
		role, err = d.client.GetRole(ctx, config.ID.ValueInt64())
		if err != nil {
			resp.Diagnostics.AddError("Error reading role", err.Error())
			return
		}
	} else {
		hasName := !config.Name.IsNull() && !config.Name.IsUnknown()
		if !hasName {
			resp.Diagnostics.AddError("Missing lookup criteria", "Specify 'id' for direct lookup, or 'name' to search.")
			return
		}

		name := config.Name.ValueString()
		roles, err := d.client.ListRoles(ctx, client.RoleListFilter{Search: name})
		if err != nil {
			resp.Diagnostics.AddError("Error searching roles", err.Error())
			return
		}

		var matched []client.RoleDetailResponse
		for i := range roles {
			if roles[i].Name == name {
				matched = append(matched, roles[i])
			}
		}
		if len(matched) == 0 {
			resp.Diagnostics.AddError("No matching role found", "No role matches the specified name.")
			return
		}
		if len(matched) > 1 {
			resp.Diagnostics.AddError("Multiple roles found",
				fmt.Sprintf("Found %d roles matching the name. Role names should be unique.", len(matched)))
			return
		}
		role = &matched[0]
	}

	config.ID = types.Int64Value(role.ID)
	config.Name = types.StringValue(role.Name)
	config.Description = stringPtrToValue(role.Description)
	config.ScopeLocationIDs = int64SliceToSet(role.ScopeLocationIDs)
	config.Permissions = stringSliceToSet(role.Permissions)
	config.CreatedAt = types.StringValue(role.CreatedAt)
	config.UpdatedAt = types.StringValue(role.UpdatedAt)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
