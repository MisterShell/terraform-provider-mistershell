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

var _ datasource.DataSource = &AIPromptDataSource{}

type AIPromptDataSource struct {
	client *client.Client
}

type AIPromptDataSourceModel struct {
	ID             types.Int64          `tfsdk:"id"`
	Name           types.String         `tfsdk:"name"`
	Type           types.String         `tfsdk:"type"`
	Content        types.String         `tfsdk:"content"`
	Description    types.String         `tfsdk:"description"`
	VariableSchema jsontypes.Normalized `tfsdk:"variable_schema"`
	CreatedAt      types.String         `tfsdk:"created_at"`
	UpdatedAt      types.String         `tfsdk:"updated_at"`
}

func NewAIPromptDataSource() datasource.DataSource {
	return &AIPromptDataSource{}
}

func (d *AIPromptDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ai_prompt"
}

func (d *AIPromptDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a MisterShell AI prompt. Look up by id for a direct fetch, or by name (exact match). Both custom (type=\"user\") and builtin (type=\"system\") prompts are readable here.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Description: "AI prompt ID. Use for direct lookup.",
				Optional:    true,
				Computed:    true,
			},
			"name": schema.StringAttribute{
				Description: "AI prompt name. Used as an exact-match search filter when id is not specified.",
				Optional:    true,
				Computed:    true,
			},
			"type": schema.StringAttribute{
				Description: "Prompt type. Either \"user\" (custom) or \"system\" (builtin/read-only).",
				Computed:    true,
			},
			"content": schema.StringAttribute{
				Description: "The prompt text. May contain Jinja2 template variables such as {{resource_id}}.",
				Computed:    true,
			},
			"description": schema.StringAttribute{
				Description: "Human-readable description of the prompt.",
				Computed:    true,
			},
			"variable_schema": schema.StringAttribute{
				Description: "JSON Schema documenting the template variables used in content.",
				Computed:    true,
				CustomType:  jsontypes.NormalizedType{},
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

func (d *AIPromptDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *AIPromptDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config AIPromptDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	hasID := !config.ID.IsNull() && !config.ID.IsUnknown()

	var prompt *client.AIPromptResponse

	if hasID {
		var err error
		prompt, err = d.client.GetAIPrompt(ctx, config.ID.ValueInt64())
		if err != nil {
			resp.Diagnostics.AddError("Error reading AI prompt", err.Error())
			return
		}
	} else {
		hasName := !config.Name.IsNull() && !config.Name.IsUnknown()
		if !hasName {
			resp.Diagnostics.AddError("Missing lookup criteria", "Specify 'id' for direct lookup, or 'name' to search.")
			return
		}

		prompts, err := d.client.ListAIPrompts(ctx, client.AIPromptListFilter{Search: config.Name.ValueString()})
		if err != nil {
			resp.Diagnostics.AddError("Error searching AI prompts", err.Error())
			return
		}

		name := config.Name.ValueString()
		var filtered []client.AIPromptResponse
		for _, p := range prompts {
			if p.Name == name {
				filtered = append(filtered, p)
			}
		}

		if len(filtered) == 0 {
			resp.Diagnostics.AddError("No matching AI prompt found", "No AI prompt matches the specified name.")
			return
		}
		if len(filtered) > 1 {
			resp.Diagnostics.AddError("Multiple AI prompts found",
				fmt.Sprintf("Found %d AI prompts matching name %q.", len(filtered), name))
			return
		}
		prompt = &filtered[0]
	}

	config.ID = types.Int64Value(prompt.ID)
	config.Name = types.StringValue(prompt.Name)
	config.Type = types.StringValue(prompt.Type)
	config.Content = types.StringValue(prompt.Content)
	config.Description = stringPtrToValue(prompt.Description)
	config.VariableSchema = rawJSONToNormalized(prompt.VariableSchema)
	config.CreatedAt = types.StringValue(prompt.CreatedAt)
	config.UpdatedAt = types.StringValue(prompt.UpdatedAt)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
