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

var _ datasource.DataSource = &AIAgentDataSource{}

type AIAgentDataSource struct {
	client *client.Client
}

type AIAgentDataSourceModel struct {
	ID             types.Int64          `tfsdk:"id"`
	Name           types.String         `tfsdk:"name"`
	Type           types.String         `tfsdk:"type"`
	Description    types.String         `tfsdk:"description"`
	ModelID        types.Int64          `tfsdk:"model_id"`
	SystemPromptID types.Int64          `tfsdk:"system_prompt_id"`
	Config         jsontypes.Normalized `tfsdk:"config"`
	IsBuiltin      types.Bool           `tfsdk:"is_builtin"`
	IsFunctional   types.Bool           `tfsdk:"is_functional"`
	ToolIDs        types.Set            `tfsdk:"tool_ids"`
	CreatedAt      types.String         `tfsdk:"created_at"`
	UpdatedAt      types.String         `tfsdk:"updated_at"`
}

func NewAIAgentDataSource() datasource.DataSource {
	return &AIAgentDataSource{}
}

func (d *AIAgentDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ai_agent"
}

func (d *AIAgentDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a MisterShell AI agent (user-defined or builtin). Look up by id for a direct fetch, or by name (exact match).",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Description: "AI agent ID. Use for direct lookup.",
				Optional:    true,
				Computed:    true,
			},
			"name": schema.StringAttribute{
				Description: "Agent name. Used as an exact-match search filter when id is not specified.",
				Optional:    true,
				Computed:    true,
			},
			"type": schema.StringAttribute{
				Description: "Agent type (chat, background, or a builtin_* type).",
				Computed:    true,
			},
			"description": schema.StringAttribute{
				Description: "Agent description.",
				Computed:    true,
			},
			"model_id": schema.Int64Attribute{
				Description: "FK to the AI model used by the agent (null means the default model).",
				Computed:    true,
			},
			"system_prompt_id": schema.Int64Attribute{
				Description: "FK to the AI prompt providing the agent's system prompt.",
				Computed:    true,
			},
			"config": schema.StringAttribute{
				Description: "Agent config as JSON.",
				Computed:    true,
				CustomType:  jsontypes.NormalizedType{},
			},
			"is_builtin": schema.BoolAttribute{
				Description: "Whether the agent is a builtin agent managed by MisterShell.",
				Computed:    true,
			},
			"is_functional": schema.BoolAttribute{
				Description: "Whether the agent is currently functional (model and prompt resolve).",
				Computed:    true,
			},
			"tool_ids": schema.SetAttribute{
				Description: "Tool IDs the agent may use; empty means ALL tools are allowed.",
				Computed:    true,
				ElementType: types.Int64Type,
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

func (d *AIAgentDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *AIAgentDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config AIAgentDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	hasID := !config.ID.IsNull() && !config.ID.IsUnknown()

	var agent *client.AIAgentResponse

	if hasID {
		var err error
		agent, err = d.client.GetAIAgent(ctx, config.ID.ValueInt64())
		if err != nil {
			resp.Diagnostics.AddError("Error reading AI agent", err.Error())
			return
		}
	} else {
		hasName := !config.Name.IsNull() && !config.Name.IsUnknown()
		if !hasName {
			resp.Diagnostics.AddError("Missing lookup criteria", "Specify 'id' for direct lookup, or 'name' to search.")
			return
		}

		agents, err := d.client.ListAIAgents(ctx, client.AIAgentListFilter{Search: config.Name.ValueString()})
		if err != nil {
			resp.Diagnostics.AddError("Error searching AI agents", err.Error())
			return
		}

		name := config.Name.ValueString()
		var filtered []client.AIAgentResponse
		for _, a := range agents {
			if a.Name == name {
				filtered = append(filtered, a)
			}
		}

		if len(filtered) == 0 {
			resp.Diagnostics.AddError("No matching AI agent found", "No AI agent matches the specified name.")
			return
		}
		if len(filtered) > 1 {
			resp.Diagnostics.AddError("Multiple AI agents found",
				fmt.Sprintf("Found %d AI agents matching name %q.", len(filtered), name))
			return
		}
		agent = &filtered[0]
	}

	config.ID = types.Int64Value(agent.ID)
	config.Name = types.StringValue(agent.Name)
	config.Type = types.StringValue(agent.Type)
	config.Description = stringPtrToValue(agent.Description)
	config.ModelID = int64PtrToValue(agent.ModelID)
	config.SystemPromptID = int64PtrToValue(agent.SystemPromptID)
	config.Config = rawJSONToNormalized(agent.Config)
	config.IsBuiltin = types.BoolValue(agent.IsBuiltin)
	config.IsFunctional = types.BoolValue(agent.IsFunctional)
	config.ToolIDs = int64SliceToSet(agent.ToolIDs)
	config.CreatedAt = types.StringValue(agent.CreatedAt)
	config.UpdatedAt = types.StringValue(agent.UpdatedAt)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
