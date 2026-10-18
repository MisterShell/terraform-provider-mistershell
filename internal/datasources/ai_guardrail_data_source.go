package datasources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-mistershell/internal/client"
)

type AIGuardrailDataSource struct{ client *client.Client }
type AIGuardrailDataSourceModel struct {
	ID               types.String `tfsdk:"id"`
	ResourceType     types.String `tfsdk:"resource_type"`
	AllowlistEnabled types.Bool   `tfsdk:"allowlist_enabled"`
	DisplayName      types.String `tfsdk:"display_name"`
	Description      types.String `tfsdk:"description"`
}

func NewAIGuardrailDataSource() datasource.DataSource { return &AIGuardrailDataSource{} }
func (d *AIGuardrailDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ai_guardrail"
}
func (d *AIGuardrailDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{Description: "Reads the effective AI command allowlist toggle for a registered resource type.", Attributes: map[string]schema.Attribute{"id": schema.StringAttribute{Computed: true}, "resource_type": schema.StringAttribute{Required: true}, "allowlist_enabled": schema.BoolAttribute{Computed: true}, "display_name": schema.StringAttribute{Computed: true}, "description": schema.StringAttribute{Computed: true}}}
}
func (d *AIGuardrailDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *AIGuardrailDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state AIGuardrailDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rows, err := d.client.ListAIGuardrails(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading AI guardrails", err.Error())
		return
	}
	for _, row := range rows {
		if row.ResourceType == state.ResourceType.ValueString() {
			state.ID = types.StringValue(row.ResourceType)
			state.AllowlistEnabled = types.BoolValue(row.AllowlistEnabled)
			state.DisplayName = types.StringValue(row.DisplayName)
			state.Description = types.StringValue(row.Description)
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
	}
	resp.Diagnostics.AddError("AI guardrail not found", fmt.Sprintf("No guardrail exists for resource type %q", state.ResourceType.ValueString()))
}
