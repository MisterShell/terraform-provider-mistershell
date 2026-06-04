package datasources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-mistershell/internal/client"
)

var _ datasource.DataSource = &SessionPolicyAclDataSource{}

type SessionPolicyAclDataSource struct {
	client *client.Client
}

type SessionPolicyAclDataSourceModel struct {
	ID          types.Int64  `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Patterns    types.List   `tfsdk:"patterns"`
	Enabled     types.Bool   `tfsdk:"enabled"`
	IsBuiltin   types.Bool   `tfsdk:"is_builtin"`
	CreatedAt   types.String `tfsdk:"created_at"`
	UpdatedAt   types.String `tfsdk:"updated_at"`
}

func NewSessionPolicyAclDataSource() datasource.DataSource {
	return &SessionPolicyAclDataSource{}
}

func (d *SessionPolicyAclDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_session_policy_acl"
}

func (d *SessionPolicyAclDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a MisterShell session-policy command ACL. Look up by id or by exact name (must match exactly one). Reads resolve through the list endpoint (there is no GET-single for ACLs).",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Description: "ACL ID. Use for direct lookup.",
				Optional:    true,
				Computed:    true,
			},
			"name": schema.StringAttribute{
				Description: "ACL name (exact match). Used as a lookup filter when id is not specified.",
				Optional:    true,
				Computed:    true,
			},
			"description": schema.StringAttribute{
				Description: "ACL description.",
				Computed:    true,
			},
			"patterns": schema.ListNestedAttribute{
				Description: "Ordered list of command-match patterns.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"pattern": schema.StringAttribute{
							Description: "The match pattern.",
							Computed:    true,
						},
						"type": schema.StringAttribute{
							Description: "Pattern type (glob or regex).",
							Computed:    true,
						},
					},
				},
			},
			"enabled": schema.BoolAttribute{
				Description: "Whether the ACL is enabled.",
				Computed:    true,
			},
			"is_builtin": schema.BoolAttribute{
				Description: "Whether this is a built-in ACL.",
				Computed:    true,
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

func (d *SessionPolicyAclDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *SessionPolicyAclDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config SessionPolicyAclDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	hasID := !config.ID.IsNull() && !config.ID.IsUnknown()
	hasName := !config.Name.IsNull() && !config.Name.IsUnknown()

	if !hasID && !hasName {
		resp.Diagnostics.AddError("Missing lookup criteria", "Specify 'id' or 'name'.")
		return
	}

	var acl *client.AclResponse

	if hasID {
		var err error
		acl, err = d.client.GetAcl(ctx, config.ID.ValueInt64())
		if err != nil {
			resp.Diagnostics.AddError("Error reading ACL", err.Error())
			return
		}
	} else {
		name := config.Name.ValueString()
		acls, err := d.client.ListAcls(ctx, client.AclListFilter{Search: name})
		if err != nil {
			resp.Diagnostics.AddError("Error searching ACLs", err.Error())
			return
		}
		var filtered []client.AclResponse
		for _, a := range acls {
			if a.Name == name {
				filtered = append(filtered, a)
			}
		}
		if len(filtered) == 0 {
			resp.Diagnostics.AddError("No matching ACL found", "No ACL matches the specified name.")
			return
		}
		if len(filtered) > 1 {
			resp.Diagnostics.AddError("Multiple ACLs found",
				fmt.Sprintf("Found %d ACLs named %q. Use id for a unique lookup.", len(filtered), name))
			return
		}
		acl = &filtered[0]
	}

	config.ID = types.Int64Value(acl.ID)
	config.Name = types.StringValue(acl.Name)
	config.Description = types.StringValue(acl.Description)
	config.Patterns = aclPatternsToList(acl.Patterns)
	config.Enabled = types.BoolValue(acl.Enabled)
	config.IsBuiltin = types.BoolValue(acl.IsBuiltin)
	config.CreatedAt = types.StringValue(acl.CreatedAt)
	config.UpdatedAt = types.StringValue(acl.UpdatedAt)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
