package provider

import (
	"context"
	"crypto/tls"
	"net/http"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-mistershell/internal/client"
	"terraform-provider-mistershell/internal/datasources"
	"terraform-provider-mistershell/internal/resources"
)

var _ provider.Provider = &MisterShellProvider{}

type MisterShellProvider struct {
	version string
}

type MisterShellProviderModel struct {
	URL      types.String `tfsdk:"url"`
	APIKey   types.String `tfsdk:"api_key"`
	Insecure types.Bool   `tfsdk:"insecure"`
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &MisterShellProvider{version: version}
	}
}

func (p *MisterShellProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "mistershell"
	resp.Version = p.version
}

func (p *MisterShellProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Terraform provider for managing MisterShell resources (locations, network resources, credentials, tags, roles, permissions, log destinations, application settings, session-policy ACLs and rules, and external authentication providers with group mappings).",
		Attributes: map[string]schema.Attribute{
			"url": schema.StringAttribute{
				Description: "MisterShell base URL (e.g. https://mistershell.example.com). Can also be set with the MISTERSHELL_URL environment variable.",
				Optional:    true,
			},
			"api_key": schema.StringAttribute{
				Description: "MisterShell API key (yami_ prefixed). Can also be set with the MISTERSHELL_API_KEY environment variable.",
				Optional:    true,
				Sensitive:   true,
			},
			"insecure": schema.BoolAttribute{
				Description: "Skip TLS certificate verification. Use for self-signed certificates. Defaults to false.",
				Optional:    true,
			},
		},
	}
}

func (p *MisterShellProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config MisterShellProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Resolve URL from config or environment
	url := os.Getenv("MISTERSHELL_URL")
	if !config.URL.IsNull() && !config.URL.IsUnknown() {
		url = config.URL.ValueString()
	}
	if url == "" {
		resp.Diagnostics.AddError(
			"Missing MisterShell URL",
			"Set the 'url' provider attribute or the MISTERSHELL_URL environment variable.",
		)
	}

	// Resolve API key from config or environment
	apiKey := os.Getenv("MISTERSHELL_API_KEY")
	if !config.APIKey.IsNull() && !config.APIKey.IsUnknown() {
		apiKey = config.APIKey.ValueString()
	}
	if apiKey == "" {
		resp.Diagnostics.AddError(
			"Missing MisterShell API Key",
			"Set the 'api_key' provider attribute or the MISTERSHELL_API_KEY environment variable.",
		)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	// Build HTTP client
	insecure := false
	if !config.Insecure.IsNull() && !config.Insecure.IsUnknown() {
		insecure = config.Insecure.ValueBool()
	}

	httpClient := &http.Client{}
	if insecure {
		httpClient.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		}
	}

	c := client.NewClient(url, apiKey, httpClient)
	resp.DataSourceData = c
	resp.ResourceData = c
}

func (p *MisterShellProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		resources.NewLocationResource,
		resources.NewNetworkResourceResource,
		resources.NewCredentialResource,
		resources.NewTagResource,
		resources.NewRoleResource,
		resources.NewLogDestinationResource,
		resources.NewSettingResource,
		resources.NewSessionPolicyAclResource,
		resources.NewSessionPolicyRuleResource,
		resources.NewAuthProviderResource,
		resources.NewAuthProviderMappingResource,
	}
}

func (p *MisterShellProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		datasources.NewLocationDataSource,
		datasources.NewNetworkResourceDataSource,
		datasources.NewCredentialDataSource,
		datasources.NewTagDataSource,
		datasources.NewRoleDataSource,
		datasources.NewPermissionsDataSource,
		datasources.NewLogDestinationDataSource,
		datasources.NewLogDestinationPresetsDataSource,
		datasources.NewSettingDataSource,
		datasources.NewSessionPolicyAclDataSource,
		datasources.NewSessionPolicyRuleDataSource,
		datasources.NewAuthProviderDataSource,
	}
}
