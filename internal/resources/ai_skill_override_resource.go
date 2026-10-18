package resources

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-mistershell/internal/client"
)

type AISkillOverrideResource struct{ client *client.Client }
type AISkillOverrideModel struct {
	ID              types.Int64  `tfsdk:"id"`
	SkillID         types.Int64  `tfsdk:"skill_id"`
	IsEnabled       types.Bool   `tfsdk:"is_enabled"`
	OriginalEnabled types.Bool   `tfsdk:"original_enabled"`
	Name            types.String `tfsdk:"name"`
}

func NewAISkillOverrideResource() resource.Resource { return &AISkillOverrideResource{} }
func (r *AISkillOverrideResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ai_skill_override"
}
func (r *AISkillOverrideResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{Description: "Adopts the enabled setting of an existing built-in AI skill. Destroy restores its captured original setting and never deletes the skill. Import captures the current setting as the restoration baseline.", Attributes: map[string]schema.Attribute{
		"id": schema.Int64Attribute{Computed: true}, "skill_id": schema.Int64Attribute{Required: true, PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()}}, "is_enabled": schema.BoolAttribute{Required: true}, "name": schema.StringAttribute{Computed: true}, "original_enabled": schema.BoolAttribute{Computed: true, PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}, Description: "Enabled state captured at adoption/import, restored on destroy."},
	}}
}
func (r *AISkillOverrideResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
func (r *AISkillOverrideResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan AISkillOverrideModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	row, err := r.client.GetAISkill(ctx, plan.SkillID.ValueInt64())
	if err != nil {
		resp.Diagnostics.AddError("Error reading AI skill", err.Error())
		return
	}
	if !row.IsBuiltin {
		resp.Diagnostics.AddError("Not a built-in skill", "Use ai_skill to manage user-defined skills.")
		return
	}
	plan.OriginalEnabled = types.BoolValue(row.IsEnabled)
	row, err = r.client.UpdateAISkillEnabled(ctx, row.ID, plan.IsEnabled.ValueBool())
	if err != nil {
		resp.Diagnostics.AddError("Error overriding AI skill", err.Error())
		return
	}
	plan.ID = types.Int64Value(row.ID)
	plan.Name = types.StringValue(row.Name)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}
func (r *AISkillOverrideResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state AISkillOverrideModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	row, err := r.client.GetAISkill(ctx, state.SkillID.ValueInt64())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading AI skill override", err.Error())
		return
	}
	if !row.IsBuiltin {
		resp.Diagnostics.AddError("Not a built-in skill", "Only built-in skills support overrides.")
		return
	}
	state.ID = types.Int64Value(row.ID)
	state.Name = types.StringValue(row.Name)
	state.IsEnabled = types.BoolValue(row.IsEnabled)
	if state.OriginalEnabled.IsNull() || state.OriginalEnabled.IsUnknown() {
		state.OriginalEnabled = state.IsEnabled
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
func (r *AISkillOverrideResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state AISkillOverrideModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	row, err := r.client.GetAISkill(ctx, plan.SkillID.ValueInt64())
	if err != nil {
		resp.Diagnostics.AddError("Error reading AI skill", err.Error())
		return
	}
	if !row.IsBuiltin {
		resp.Diagnostics.AddError("Not a built-in skill", "Only built-in skills support overrides.")
		return
	}
	row, err = r.client.UpdateAISkillEnabled(ctx, row.ID, plan.IsEnabled.ValueBool())
	if err != nil {
		resp.Diagnostics.AddError("Error overriding AI skill", err.Error())
		return
	}
	plan.OriginalEnabled = state.OriginalEnabled
	plan.ID = types.Int64Value(row.ID)
	plan.Name = types.StringValue(row.Name)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}
func (r *AISkillOverrideResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state AISkillOverrideModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	row, err := r.client.GetAISkill(ctx, state.SkillID.ValueInt64())
	if client.IsNotFound(err) {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading AI skill", err.Error())
		return
	}
	if !row.IsBuiltin {
		resp.Diagnostics.AddError("Not a built-in skill", "Refusing to reset a user-defined skill.")
		return
	}
	if state.OriginalEnabled.IsNull() || state.OriginalEnabled.IsUnknown() {
		resp.Diagnostics.AddError("Missing restoration baseline", "Refresh or reimport this override before destroying it.")
		return
	}
	if _, err = r.client.UpdateAISkillEnabled(ctx, row.ID, state.OriginalEnabled.ValueBool()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error restoring AI skill", err.Error())
	}
}
func (r *AISkillOverrideResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil || id <= 0 {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("Expected positive skill ID, got %q", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("skill_id"), id)...)
}
