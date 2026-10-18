package provider

import (
	"context"
	"os"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-mistershell/internal/apimanaged"
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
		Description: "Terraform provider for managing MisterShell resources (locations, network resources, credentials, external vaults, tags, roles, permissions, log destinations, application settings, session-policy ACLs and rules, external authentication providers with group mappings, workers, and AI models, prompts, agents, and skills).",
		Attributes: map[string]schema.Attribute{
			"url": schema.StringAttribute{
				Description: "MisterShell base URL (e.g. https://mistershell.example.com). Can also be set with the MISTERSHELL_URL environment variable.",
				Optional:    true,
			},
			"api_key": schema.StringAttribute{
				Description: "MisterShell API key (apikey_ prefixed). Can also be set with the MISTERSHELL_API_KEY environment variable.",
				Optional:    true,
				Sensitive:   true,
			},
			"insecure": schema.BoolAttribute{
				Description: "Skip TLS certificate verification. Can also be set with MISTERSHELL_INSECURE (1 or true, case-insensitive). An explicit value overrides the environment variable. Defaults to false.",
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
	v := os.Getenv("MISTERSHELL_INSECURE")
	insecure := v == "1" || strings.EqualFold(v, "true")
	if !config.Insecure.IsNull() && !config.Insecure.IsUnknown() {
		insecure = config.Insecure.ValueBool()
	}

	httpClient := client.NewHTTPClient(insecure)
	c := client.NewClient(url, apiKey, httpClient)
	resp.DataSourceData = c
	resp.ResourceData = c
}

func (p *MisterShellProvider) Resources(_ context.Context) []func() resource.Resource {
	constructors := []func() resource.Resource{
		resources.NewLocationResource,
		resources.NewNetworkResourceResource,
		resources.NewCredentialResource,
		resources.NewExternalVaultResource,
		resources.NewTagResource,
		resources.NewRoleResource,
		resources.NewLogDestinationResource,
		resources.NewSettingResource,
		resources.NewSessionPolicyAclResource,
		resources.NewSessionPolicyRuleResource,
		resources.NewAuthProviderResource,
		resources.NewAuthProviderMappingResource,
		resources.NewUserResource,
		resources.NewUserRoleResource,
		resources.NewWorkerResource,
		resources.NewAIModelResource,
		resources.NewAIPromptResource,
		resources.NewAIAgentResource,
		resources.NewAISkillResource,
		resources.NewAIAgentOverrideResource,
		resources.NewAISkillOverrideResource,
		resources.NewAIGuardrailResource,
		resources.NewSensorRuleOverrideResource,
	}
	for _, definition := range apimanaged.Definitions() {
		constructors = append(constructors, func() resource.Resource { return apimanaged.NewResource(definition) })
	}
	return constructors
}

func (p *MisterShellProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	constructors := []func() datasource.DataSource{
		datasources.NewLocationDataSource,
		datasources.NewNetworkResourceDataSource,
		datasources.NewCredentialDataSource,
		datasources.NewCredentialTypesDataSource,
		datasources.NewExternalVaultTypesDataSource,
		datasources.NewExternalVaultDataSource,
		datasources.NewTagDataSource,
		datasources.NewTagsDataSource,
		datasources.NewRoleDataSource,
		datasources.NewUserDataSource,
		datasources.NewUsersDataSource,
		datasources.NewPermissionsDataSource,
		datasources.NewLogDestinationDataSource,
		datasources.NewLogDestinationPresetsDataSource,
		datasources.NewSettingDataSource,
		datasources.NewSessionPolicyAclDataSource,
		datasources.NewSessionPolicyRuleDataSource,
		datasources.NewAuthProviderDataSource,
		datasources.NewAuthProviderMappingDataSource,
		datasources.NewWorkerDataSource,
		datasources.NewAIModelDataSource,
		datasources.NewAIPromptDataSource,
		datasources.NewAIAgentDataSource,
		datasources.NewAISkillDataSource,
		datasources.NewAIToolDataSource,
		datasources.NewAIGuardrailDataSource,
		datasources.NewSensorRuleOverrideDataSource,
		datasources.NewAIPromptVariablesDataSource,
	}
	for _, definition := range apimanaged.Definitions() {
		constructors = append(constructors, func() datasource.DataSource { return apimanaged.NewDataSource(definition) })
	}
	return constructors
}
