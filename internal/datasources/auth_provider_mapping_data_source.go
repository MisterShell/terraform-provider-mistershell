package datasources

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"terraform-provider-mistershell/internal/client"
)

type AuthProviderMappingDataSource struct{ client *client.Client }
type AuthProviderMappingDataSourceModel struct {
	ID            types.Int64  `tfsdk:"id"`
	ProviderID    types.Int64  `tfsdk:"provider_id"`
	ExternalGroup types.String `tfsdk:"external_group"`
	RoleID        types.Int64  `tfsdk:"role_id"`
	RoleName      types.String `tfsdk:"role_name"`
	CreatedAt     types.String `tfsdk:"created_at"`
	UpdatedAt     types.String `tfsdk:"updated_at"`
}

func NewAuthProviderMappingDataSource() datasource.DataSource {
	return &AuthProviderMappingDataSource{}
}
func (d *AuthProviderMappingDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_auth_provider_mapping"
}
func (d *AuthProviderMappingDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{Description: "Reads an external group to role mapping under an authentication provider. Specify provider_id and either id or an exact external_group.", Attributes: map[string]schema.Attribute{
		"provider_id":    schema.Int64Attribute{Required: true, Description: "Parent authentication provider ID."},
		"id":             schema.Int64Attribute{Optional: true, Computed: true, Description: "Mapping ID for direct lookup."},
		"external_group": schema.StringAttribute{Optional: true, Computed: true, Description: "Exact external group identifier for lookup when id is omitted."},
		"role_id":        schema.Int64Attribute{Computed: true, Description: "Granted role ID."},
		"role_name":      schema.StringAttribute{Computed: true, Description: "Granted role name."},
		"created_at":     schema.StringAttribute{Computed: true, Description: "Creation timestamp."},
		"updated_at":     schema.StringAttribute{Computed: true, Description: "Last update timestamp."},
	}}
}
func (d *AuthProviderMappingDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *AuthProviderMappingDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config AuthProviderMappingDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var mapping *client.GroupMappingResponse
	var err error
	if !config.ID.IsNull() && !config.ID.IsUnknown() {
		mapping, err = d.client.GetGroupMapping(ctx, config.ProviderID.ValueInt64(), config.ID.ValueInt64())
	} else {
		if config.ExternalGroup.IsNull() || config.ExternalGroup.IsUnknown() {
			resp.Diagnostics.AddError("Missing lookup criteria", "Specify id or external_group together with provider_id.")
			return
		}
		var mappings []client.GroupMappingResponse
		mappings, err = d.client.ListGroupMappings(ctx, config.ProviderID.ValueInt64())
		if err == nil {
			matches := 0
			for i := range mappings {
				if mappings[i].ExternalGroup == config.ExternalGroup.ValueString() {
					mapping = &mappings[i]
					matches++
				}
			}
			if matches != 1 {
				resp.Diagnostics.AddError("Mapping lookup failed", fmt.Sprintf("Expected one mapping for the external group; found %d.", matches))
				return
			}
		}
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading auth provider mapping", err.Error())
		return
	}
	config.ID = types.Int64Value(mapping.ID)
	config.ProviderID = types.Int64Value(mapping.AuthProviderID)
	config.ExternalGroup = types.StringValue(mapping.ExternalGroup)
	config.RoleID = types.Int64Value(mapping.RoleID)
	config.RoleName = types.StringValue(mapping.RoleName)
	config.CreatedAt = types.StringValue(mapping.CreatedAt)
	config.UpdatedAt = types.StringValue(mapping.UpdatedAt)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
