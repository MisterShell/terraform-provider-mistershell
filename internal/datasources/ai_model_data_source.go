package datasources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-mistershell/internal/client"
)

var _ datasource.DataSource = &AIModelDataSource{}

type AIModelDataSource struct {
	client *client.Client
}

type AIModelDataSourceModel struct {
	ID        types.Int64          `tfsdk:"id"`
	Name      types.String         `tfsdk:"name"`
	Provider  types.String         `tfsdk:"model_provider"`
	ModelID   types.String         `tfsdk:"model_id"`
	Config    jsontypes.Normalized `tfsdk:"config"`
	IsDefault types.Bool           `tfsdk:"is_default"`
	CreatedAt types.String         `tfsdk:"created_at"`
	UpdatedAt types.String         `tfsdk:"updated_at"`
}

func NewAIModelDataSource() datasource.DataSource {
	return &AIModelDataSource{}
}

func (d *AIModelDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ai_model"
}

func (d *AIModelDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a MisterShell AI model. Look up by id for a direct fetch, or by name (exact match). Note: provider secrets in config (e.g. api_key) are masked as '***' by the API.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Description: "AI model ID. Use for direct lookup.",
				Optional:    true,
				Computed:    true,
			},
			"name": schema.StringAttribute{
				Description: "AI model name. Used as an exact-match search filter when id is not specified.",
				Optional:    true,
				Computed:    true,
			},
			"model_provider": schema.StringAttribute{
				Description: "AI provider backing this model (anthropic, azure_openai, bedrock, cohere, google, mistral, ollama, openai, openrouter, xai).",
				Computed:    true,
			},
			"model_id": schema.StringAttribute{
				Description: "Provider-specific model identifier.",
				Computed:    true,
			},
			"config": schema.StringAttribute{
				Description: "Per-provider config as JSON. Provider secrets (e.g. api_key) are masked as '***' by the API.",
				Computed:    true,
				Sensitive:   true,
				CustomType:  jsontypes.NormalizedType{},
			},
			"is_default": schema.BoolAttribute{
				Description: "Whether this is the default AI model.",
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

func (d *AIModelDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *AIModelDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config AIModelDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	hasID := !config.ID.IsNull() && !config.ID.IsUnknown()

	var model *client.AIModelResponse

	if hasID {
		var err error
		model, err = d.client.GetAIModel(ctx, config.ID.ValueInt64())
		if err != nil {
			resp.Diagnostics.AddError("Error reading AI model", err.Error())
			return
		}
	} else {
		hasName := !config.Name.IsNull() && !config.Name.IsUnknown()
		if !hasName {
			resp.Diagnostics.AddError("Missing lookup criteria", "Specify 'id' for direct lookup, or 'name' to search.")
			return
		}

		models, err := d.client.ListAIModels(ctx, client.AIModelListFilter{Search: config.Name.ValueString()})
		if err != nil {
			resp.Diagnostics.AddError("Error searching AI models", err.Error())
			return
		}

		name := config.Name.ValueString()
		var filtered []client.AIModelResponse
		for _, m := range models {
			if m.Name == name {
				filtered = append(filtered, m)
			}
		}

		if len(filtered) == 0 {
			resp.Diagnostics.AddError("No matching AI model found", "No AI model matches the specified name.")
			return
		}
		if len(filtered) > 1 {
			resp.Diagnostics.AddError("Multiple AI models found",
				fmt.Sprintf("Found %d AI models matching name %q.", len(filtered), name))
			return
		}
		model = &filtered[0]
	}

	config.ID = types.Int64Value(model.ID)
	config.Name = types.StringValue(model.Name)
	config.Provider = types.StringValue(model.Provider)
	config.ModelID = types.StringValue(model.ModelID)
	config.Config = rawJSONToNormalized(model.Config)
	config.IsDefault = types.BoolValue(model.IsDefault)
	config.CreatedAt = types.StringValue(model.CreatedAt)
	config.UpdatedAt = types.StringValue(model.UpdatedAt)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
