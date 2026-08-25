package datasources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-mistershell/internal/client"
)

var _ datasource.DataSource = &AuthProviderDataSource{}

type AuthProviderDataSource struct {
	client *client.Client
}

type AuthProviderDataSourceModel struct {
	ID                 types.Int64   `tfsdk:"id"`
	Name               types.String  `tfsdk:"name"`
	Mnemonic           types.String  `tfsdk:"mnemonic"`
	ProviderType       types.String  `tfsdk:"provider_type"`
	IsEnabled          types.Bool    `tfsdk:"is_enabled"`
	DisplayOrder       types.Int64   `tfsdk:"display_order"`
	Config             types.Dynamic `tfsdk:"config"`
	GroupMappingsCount types.Int64   `tfsdk:"group_mappings_count"`
	SPEntityID         types.String  `tfsdk:"sp_entity_id"`
	SPACSURL           types.String  `tfsdk:"sp_acs_url"`
	OIDCRedirectURI    types.String  `tfsdk:"oidc_redirect_uri"`
	CreatedAt          types.String  `tfsdk:"created_at"`
	UpdatedAt          types.String  `tfsdk:"updated_at"`
}

func NewAuthProviderDataSource() datasource.DataSource {
	return &AuthProviderDataSource{}
}

func (d *AuthProviderDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_auth_provider"
}

func (d *AuthProviderDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a MisterShell external authentication provider. Look up by id or by exact name (names are unique). The config output reflects the server's config with secrets masked as '****'.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Description: "Auth provider ID. Use for direct lookup.",
				Optional:    true,
				Computed:    true,
			},
			"name": schema.StringAttribute{
				Description: "Provider name (exact match). Used as a lookup filter when id is not specified.",
				Optional:    true,
				Computed:    true,
			},
			"mnemonic": schema.StringAttribute{
				Description: "Stable URL token used by the provider's login and callback URLs.",
				Computed:    true,
			},
			"provider_type": schema.StringAttribute{
				Description: "Provider type (LDAP, OIDC, or SAML).",
				Computed:    true,
			},
			"is_enabled": schema.BoolAttribute{
				Description: "Whether the provider is enabled.",
				Computed:    true,
			},
			"display_order": schema.Int64Attribute{
				Description: "Login-page display order.",
				Computed:    true,
			},
			"config": schema.DynamicAttribute{
				Description: "Per-provider_type config as a native Terraform object. Secret fields are masked as '****' by the API.",
				Computed:    true,
				Sensitive:   true,
			},
			"group_mappings_count": schema.Int64Attribute{
				Description: "Number of external group->role mappings attached to this provider.",
				Computed:    true,
			},
			"sp_entity_id": schema.StringAttribute{
				Description: "SAML only (null for LDAP/OIDC): service-provider entity ID derived from app_base_url and mnemonic.",
				Computed:    true,
			},
			"sp_acs_url": schema.StringAttribute{
				Description: "SAML only (null for LDAP/OIDC): Assertion Consumer Service URL derived from app_base_url and mnemonic.",
				Computed:    true,
			},
			"oidc_redirect_uri": schema.StringAttribute{
				Description: "OIDC callback URL to register with the identity provider.",
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

func (d *AuthProviderDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *AuthProviderDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config AuthProviderDataSourceModel
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

	var id int64

	if hasID {
		id = config.ID.ValueInt64()
	} else {
		name := config.Name.ValueString()
		aps, err := d.client.ListAuthProviders(ctx, client.AuthProviderListFilter{Search: name})
		if err != nil {
			resp.Diagnostics.AddError("Error searching auth providers", err.Error())
			return
		}
		var filtered []client.AuthProviderResponse
		for _, ap := range aps {
			if ap.Name == name {
				filtered = append(filtered, ap)
			}
		}
		if len(filtered) == 0 {
			resp.Diagnostics.AddError("No matching auth provider found", "No auth provider matches the specified name.")
			return
		}
		if len(filtered) > 1 {
			resp.Diagnostics.AddError("Multiple auth providers found",
				fmt.Sprintf("Found %d auth providers named %q.", len(filtered), name))
			return
		}
		id = filtered[0].ID
	}

	detail, err := d.client.GetAuthProvider(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Error reading auth provider", err.Error())
		return
	}

	config.ID = types.Int64Value(detail.ID)
	config.Name = types.StringValue(detail.Name)
	config.Mnemonic = types.StringValue(detail.Mnemonic)
	config.ProviderType = types.StringValue(detail.ProviderType)
	config.IsEnabled = types.BoolValue(detail.IsEnabled)
	config.DisplayOrder = types.Int64Value(detail.DisplayOrder)
	nativeConfig, err := rawJSONToDynamic(detail.Config)
	if err != nil {
		resp.Diagnostics.AddError("Error decoding auth provider config", err.Error())
		return
	}
	config.Config = nativeConfig
	config.GroupMappingsCount = types.Int64Value(detail.GroupMappingsCount)
	config.SPEntityID = stringPtrToValue(detail.SPEntityID)
	config.SPACSURL = stringPtrToValue(detail.SPACSURL)
	config.OIDCRedirectURI = stringPtrToValue(detail.OIDCRedirectURI)
	config.CreatedAt = types.StringValue(detail.CreatedAt)
	config.UpdatedAt = types.StringValue(detail.UpdatedAt)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
