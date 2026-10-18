package datasources

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-mistershell/internal/client"
)

type AIPromptVariablesDataSource struct{ client *client.Client }
type AIPromptVariablesDataSourceModel struct {
	ID        types.String  `tfsdk:"id"`
	Universal types.Dynamic `tfsdk:"universal"`
	Agents    types.Dynamic `tfsdk:"agents"`
}

func NewAIPromptVariablesDataSource() datasource.DataSource { return &AIPromptVariablesDataSource{} }
func (d *AIPromptVariablesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ai_prompt_variables"
}
func (d *AIPromptVariablesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{Description: "Reads the prompt template variable catalog, including universal variables and variables for each built-in agent.", Attributes: map[string]schema.Attribute{"id": schema.StringAttribute{Computed: true}, "universal": schema.DynamicAttribute{Computed: true, Description: "Universal variable objects with name, description and value_type."}, "agents": schema.DynamicAttribute{Computed: true, Description: "Agent blocks with agent_name, agent_capability, agent_type and variables."}}}
}
func (d *AIPromptVariablesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", "Expected *client.Client")
		return
	}
	d.client = c
}
func (d *AIPromptVariablesDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	raw, err := d.client.GetAIPromptVariables(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading AI prompt variables", err.Error())
		return
	}
	var rows struct {
		Universal json.RawMessage `json:"universal"`
		Agents    json.RawMessage `json:"agents"`
	}
	if err = json.Unmarshal(raw, &rows); err != nil {
		resp.Diagnostics.AddError("Error decoding AI prompt variables", err.Error())
		return
	}
	state := AIPromptVariablesDataSourceModel{ID: types.StringValue("catalog")}
	state.Universal, err = rawJSONToDynamic(rows.Universal)
	if err != nil {
		resp.Diagnostics.AddError("Error decoding universal variables", err.Error())
		return
	}
	state.Agents, err = rawJSONToDynamic(rows.Agents)
	if err != nil {
		resp.Diagnostics.AddError("Error decoding agent variables", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
