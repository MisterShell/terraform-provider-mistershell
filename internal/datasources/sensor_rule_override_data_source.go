package datasources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-mistershell/internal/client"
)

type SensorRuleOverrideDataSource struct{ client *client.Client }
type SensorRuleOverrideDataSourceModel struct {
	ID               types.String `tfsdk:"id"`
	GID              types.Int64  `tfsdk:"gid"`
	SID              types.Int64  `tfsdk:"sid"`
	Action           types.String `tfsdk:"action"`
	DefaultEnabled   types.Bool   `tfsdk:"default_enabled"`
	EffectiveEnabled types.Bool   `tfsdk:"effective_enabled"`
	Pending          types.Bool   `tfsdk:"pending"`
	Message          types.String `tfsdk:"message"`
	Raw              types.String `tfsdk:"raw"`
}

func NewSensorRuleOverrideDataSource() datasource.DataSource { return &SensorRuleOverrideDataSource{} }
func (d *SensorRuleOverrideDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sensor_rule_override"
}
func (d *SensorRuleOverrideDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{Description: "Reads the staged override and default state of a sensor catalog rule. A null action means no override. Effective state includes staged changes and does not confirm sensor deployment.", Attributes: map[string]schema.Attribute{
		"id": schema.StringAttribute{Computed: true}, "gid": schema.Int64Attribute{Required: true, Validators: []validator.Int64{int64validator.AtLeast(1)}}, "sid": schema.Int64Attribute{Required: true, Validators: []validator.Int64{int64validator.AtLeast(1)}}, "action": schema.StringAttribute{Computed: true}, "default_enabled": schema.BoolAttribute{Computed: true}, "effective_enabled": schema.BoolAttribute{Computed: true}, "pending": schema.BoolAttribute{Computed: true}, "message": schema.StringAttribute{Computed: true}, "raw": schema.StringAttribute{Computed: true},
	}}
}
func (d *SensorRuleOverrideDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *SensorRuleOverrideDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state SensorRuleOverrideDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	row, err := d.client.GetSensorCatalogRule(ctx, state.GID.ValueInt64(), state.SID.ValueInt64())
	if err != nil {
		resp.Diagnostics.AddError("Error reading sensor rule override", err.Error())
		return
	}
	state.ID = types.StringValue(fmt.Sprintf("%d:%d", row.GID, row.SID))
	state.Action = stringPtrToValue(row.OverrideAction)
	state.DefaultEnabled = types.BoolValue(row.DefaultEnabled)
	state.EffectiveEnabled = types.BoolValue(row.EffectiveEnabled)
	state.Pending = types.BoolValue(row.Pending)
	state.Message = stringPtrToValue(row.Message)
	state.Raw = types.StringValue(row.Raw)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
