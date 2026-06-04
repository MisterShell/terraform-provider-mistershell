package datasources

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-mistershell/internal/client"
)

var _ datasource.DataSource = &PermissionsDataSource{}

type PermissionsDataSource struct {
	client *client.Client
}

type PermissionsDataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	Module      types.String `tfsdk:"module"`
	Search      types.String `tfsdk:"search"`
	Modules     types.Set    `tfsdk:"modules"`
	Permissions types.List   `tfsdk:"permissions"`
}

// permissionObjectType is the object element type for the permissions list.
var permissionObjectType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"name":        types.StringType,
		"description": types.StringType,
		"module":      types.StringType,
		"action":      types.StringType,
	},
}

func NewPermissionsDataSource() datasource.DataSource {
	return &PermissionsDataSource{}
}

func (d *PermissionsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_permissions"
}

func (d *PermissionsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads the MisterShell permission registry (read-only). Permissions are a fixed, source-defined registry and cannot be created via Terraform. Filter by module and/or search.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Synthetic identifier for this data source.",
				Computed:    true,
			},
			"module": schema.StringAttribute{
				Description: "Filter permissions by module (server-side).",
				Optional:    true,
			},
			"search": schema.StringAttribute{
				Description: "Fuzzy search filter (server-side).",
				Optional:    true,
			},
			"modules": schema.SetAttribute{
				Description: "Set of all unique permission module names.",
				Computed:    true,
				ElementType: types.StringType,
			},
			"permissions": schema.ListNestedAttribute{
				Description: "List of permissions matching the filters.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Description: "Permission name (e.g. 'app.tags.read').",
							Computed:    true,
						},
						"description": schema.StringAttribute{
							Description: "Permission description.",
							Computed:    true,
						},
						"module": schema.StringAttribute{
							Description: "Module the permission belongs to.",
							Computed:    true,
						},
						"action": schema.StringAttribute{
							Description: "Action the permission grants.",
							Computed:    true,
						},
					},
				},
			},
		},
	}
}

func (d *PermissionsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *PermissionsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config PermissionsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	filter := client.PermissionListFilter{}
	if !config.Module.IsNull() && !config.Module.IsUnknown() {
		filter.Module = config.Module.ValueString()
	}
	if !config.Search.IsNull() && !config.Search.IsUnknown() {
		filter.Search = config.Search.ValueString()
	}

	perms, err := d.client.ListPermissions(ctx, filter)
	if err != nil {
		resp.Diagnostics.AddError("Error reading permissions", err.Error())
		return
	}

	modules, err := d.client.ListPermissionModules(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading permission modules", err.Error())
		return
	}

	permElems := make([]attr.Value, 0, len(perms))
	for _, p := range perms {
		obj, diags := types.ObjectValue(permissionObjectType.AttrTypes, map[string]attr.Value{
			"name":        types.StringValue(p.Name),
			"description": types.StringValue(p.Description),
			"module":      types.StringValue(p.Module),
			"action":      types.StringValue(p.Action),
		})
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		permElems = append(permElems, obj)
	}

	permList, diags := types.ListValue(permissionObjectType, permElems)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	config.ID = types.StringValue("permissions")
	config.Modules = stringSliceToSet(modules)
	config.Permissions = permList

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
