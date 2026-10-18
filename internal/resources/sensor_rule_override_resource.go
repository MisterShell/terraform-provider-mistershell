package resources

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-mistershell/internal/client"
)

type SensorRuleOverrideResource struct{ client *client.Client }
type SensorRuleOverrideModel struct {
	ID               types.String `tfsdk:"id"`
	GID              types.Int64  `tfsdk:"gid"`
	SID              types.Int64  `tfsdk:"sid"`
	Action           types.String `tfsdk:"action"`
	DefaultEnabled   types.Bool   `tfsdk:"default_enabled"`
	EffectiveEnabled types.Bool   `tfsdk:"effective_enabled"`
	Pending          types.Bool   `tfsdk:"pending"`
	Message          types.String `tfsdk:"message"`
}

func NewSensorRuleOverrideResource() resource.Resource { return &SensorRuleOverrideResource{} }
func (r *SensorRuleOverrideResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sensor_rule_override"
}
func (r *SensorRuleOverrideResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{Description: "Stages an enable/disable override for one sensor catalog rule. The provider does not publish these changes. Operators or scheduled backend updates in auto mode can activate staged changes. Destroy stages a reset to the rule default. This resource never invokes the global apply operation.", Attributes: map[string]schema.Attribute{
		"id":                schema.StringAttribute{Computed: true, Description: "Rule identity as gid:sid."},
		"gid":               schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(1), Validators: []validator.Int64{int64validator.AtLeast(1)}, PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()}},
		"sid":               schema.Int64Attribute{Required: true, Validators: []validator.Int64{int64validator.AtLeast(1)}, PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()}},
		"action":            schema.StringAttribute{Required: true, Validators: []validator.String{stringvalidator.OneOf("enable", "disable")}, Description: "Staged rule action: enable or disable."},
		"default_enabled":   schema.BoolAttribute{Computed: true, Description: "Rule enabled state without an override."},
		"effective_enabled": schema.BoolAttribute{Computed: true, Description: "Enabled state including the staged override; this is not confirmation of sensor deployment."},
		"pending":           schema.BoolAttribute{Computed: true, Description: "Whether this override differs from the active ruleset snapshot."},
		"message":           schema.StringAttribute{Computed: true, Description: "Rule message from the active catalog."},
	}}
}
func (r *SensorRuleOverrideResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", "Expected *client.Client")
		return
	}
	r.client = c
}
func mapSensorRuleOverride(row *client.SensorCatalogRule, m *SensorRuleOverrideModel) {
	m.ID = types.StringValue(fmt.Sprintf("%d:%d", row.GID, row.SID))
	m.GID = types.Int64Value(row.GID)
	m.SID = types.Int64Value(row.SID)
	m.Action = stringPtrToValue(row.OverrideAction)
	m.DefaultEnabled = types.BoolValue(row.DefaultEnabled)
	m.EffectiveEnabled = types.BoolValue(row.EffectiveEnabled)
	m.Pending = types.BoolValue(row.Pending)
	m.Message = stringPtrToValue(row.Message)
}
func (r *SensorRuleOverrideResource) stage(ctx context.Context, plan *SensorRuleOverrideModel, creating bool) error {
	// Validate catalog membership before writing: the bulk endpoint accepts IDs
	// not present in the catalog, which cannot be refreshed or imported.
	existing, err := r.client.GetSensorCatalogRule(ctx, plan.GID.ValueInt64(), plan.SID.ValueInt64())
	if err != nil {
		return err
	}
	if creating && existing.OverrideAction != nil {
		return fmt.Errorf("sensor rule %d:%d already has an override; import it with terraform import before managing it", plan.GID.ValueInt64(), plan.SID.ValueInt64())
	}
	if err := r.client.SetSensorRuleOverride(ctx, plan.GID.ValueInt64(), plan.SID.ValueInt64(), plan.Action.ValueString()); err != nil {
		return err
	}
	row, err := r.client.GetSensorCatalogRule(ctx, plan.GID.ValueInt64(), plan.SID.ValueInt64())
	if err != nil {
		return err
	}
	mapSensorRuleOverride(row, plan)
	return nil
}
func (r *SensorRuleOverrideResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan SensorRuleOverrideModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.stage(ctx, &plan, true); err != nil {
		resp.Diagnostics.AddError("Error staging sensor rule override", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}
func (r *SensorRuleOverrideResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan SensorRuleOverrideModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.stage(ctx, &plan, false); err != nil {
		resp.Diagnostics.AddError("Error staging sensor rule override", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}
func (r *SensorRuleOverrideResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state SensorRuleOverrideModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	row, err := r.client.GetSensorCatalogRule(ctx, state.GID.ValueInt64(), state.SID.ValueInt64())
	if err != nil {
		if client.IsNotFound(err) {
			resp.Diagnostics.AddError("Sensor rule missing from active catalog", "The durable override may still exist, so Terraform has retained its state. Restore a ruleset containing this gid/sid to refresh it, or reset this override with a targeted terraform destroy -refresh=false. Reset does not require catalog membership and is staged until separately applied.")
			return
		}
		resp.Diagnostics.AddError("Error reading sensor rule override", err.Error())
		return
	}
	if row.OverrideAction == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	mapSensorRuleOverride(row, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
func (r *SensorRuleOverrideResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state SensorRuleOverrideModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.ResetSensorRuleOverride(ctx, state.GID.ValueInt64(), state.SID.ValueInt64()); err != nil {
		resp.Diagnostics.AddError("Error resetting sensor rule override", err.Error())
	}
}
func (r *SensorRuleOverrideResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, ":")
	if len(parts) != 2 {
		resp.Diagnostics.AddError("Invalid import ID", "Expected gid:sid, for example 1:2024218.")
		return
	}
	gid, e1 := strconv.ParseInt(parts[0], 10, 64)
	sid, e2 := strconv.ParseInt(parts[1], 10, 64)
	if e1 != nil || e2 != nil || gid <= 0 || sid <= 0 {
		resp.Diagnostics.AddError("Invalid import ID", "gid and sid must be positive integers.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("gid"), gid)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("sid"), sid)...)
}
