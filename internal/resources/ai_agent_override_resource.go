package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-mistershell/internal/client"
	"terraform-provider-mistershell/internal/tfjson"
)

type AIAgentOverrideResource struct{ client *client.Client }
type AIAgentOverrideModel struct {
	ID               types.Int64   `tfsdk:"id"`
	AgentID          types.Int64   `tfsdk:"agent_id"`
	Name             types.String  `tfsdk:"name"`
	Type             types.String  `tfsdk:"type"`
	ModelID          types.Int64   `tfsdk:"model_id"`
	SystemPromptID   types.Int64   `tfsdk:"system_prompt_id"`
	Config           types.Dynamic `tfsdk:"config"`
	ToolIDs          types.Set     `tfsdk:"tool_ids"`
	OriginalSettings types.String  `tfsdk:"original_settings"`
}

func NewAIAgentOverrideResource() resource.Resource { return &AIAgentOverrideResource{} }
func (r *AIAgentOverrideResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ai_agent_override"
}
func (r *AIAgentOverrideResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{Description: "Adopts selected settings of an existing built-in AI agent. Identity remains registry-owned. Removing a setting or destroying the override restores its captured original value; config keys absent at adoption restore their effective backend defaults. Import captures current settings as its restoration baseline. Only configured fields are owned.", Attributes: map[string]schema.Attribute{
		"id":       schema.Int64Attribute{Computed: true},
		"agent_id": schema.Int64Attribute{Required: true, PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()}},
		"name":     schema.StringAttribute{Computed: true}, "type": schema.StringAttribute{Computed: true},
		"model_id":          schema.Int64Attribute{Optional: true, Description: "Model override. Zero clears the model selection to use the default. Removing this argument restores the captured original selection."},
		"system_prompt_id":  schema.Int64Attribute{Optional: true, Description: "System prompt override. Removing this argument restores the captured original prompt."},
		"config":            schema.DynamicAttribute{Optional: true, Description: "Owned runtime settings: token_budget, max_tokens, max_steps, max_execution_time_seconds, temperature, top_p, output_format. Registry identity keys are forbidden."},
		"tool_ids":          schema.SetAttribute{Optional: true, ElementType: types.Int64Type, Description: "Owned tool allowance. Empty allows all type-compatible tools; omission leaves the original allowance unmanaged."},
		"original_settings": schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}, Description: "JSON snapshot captured on adoption/import, used only to restore owned settings."},
	}}
}
func (r *AIAgentOverrideResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

var aiAgentConfigDefaults = map[string]any{"token_budget": 0, "max_tokens": 0, "max_steps": 0, "max_execution_time_seconds": 0, "temperature": 0.7, "top_p": 1.0, "output_format": "markdown"}

func aiOverrideConfig(value types.Dynamic) (map[string]any, error) {
	if value.IsNull() {
		return map[string]any{}, nil
	}
	raw, err := dynamicToRawJSON(value)
	if err != nil {
		return nil, err
	}
	var fields map[string]any
	if err = json.Unmarshal(raw, &fields); err != nil {
		return nil, fmt.Errorf("config must be an object")
	}
	for key := range fields {
		if _, ok := aiAgentConfigDefaults[key]; !ok {
			return nil, fmt.Errorf("config key %q is not an editable runtime setting", key)
		}
	}
	return fields, nil
}
func (r *AIAgentOverrideResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config AIAgentOverrideModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() || config.Config.IsUnknown() {
		return
	}
	if config.Config.IsNull() {
		return
	}
	var fields map[string]attr.Value
	switch value := config.Config.UnderlyingValue().(type) {
	case types.Object:
		fields = value.Attributes()
	case types.Map:
		fields = value.Elements()
	default:
		resp.Diagnostics.AddAttributeError(path.Root("config"), "Invalid built-in agent config", "Config must be an object of editable runtime settings.")
		return
	}
	for key := range fields {
		if _, ok := aiAgentConfigDefaults[key]; !ok {
			resp.Diagnostics.AddAttributeError(path.Root("config"), "Invalid built-in agent config", fmt.Sprintf("Config key %q is not an editable runtime setting.", key))
		}
	}

}
func aiAgentOverridePatch(plan, state *AIAgentOverrideModel, original map[string]any) (map[string]any, error) {
	desired, err := aiOverrideConfig(plan.Config)
	if err != nil {
		return nil, err
	}
	previous, err := aiOverrideConfig(state.Config)
	if err != nil {
		return nil, err
	}
	baseline, _ := original["config"].(map[string]any)
	for key := range previous {
		if _, ok := desired[key]; !ok {
			value, exists := baseline[key]
			if !exists {
				value = aiAgentConfigDefaults[key]
			}
			desired[key] = value
		}
	}
	patch := map[string]any{}
	if len(desired) > 0 {
		patch["config"] = desired
	}
	for key, pair := range map[string][2]types.Int64{"model_id": {plan.ModelID, state.ModelID}, "system_prompt_id": {plan.SystemPromptID, state.SystemPromptID}} {
		if !pair[0].IsNull() {
			patch[key] = pair[0].ValueInt64()
		} else if !pair[1].IsNull() {
			patch[key] = original[key]
		}
	}
	if !plan.ToolIDs.IsNull() {
		patch["tool_ids"] = int64SetToSlice(plan.ToolIDs)
	} else if !state.ToolIDs.IsNull() {
		patch["tool_ids"] = original["tool_ids"]
	}
	return patch, nil
}
func aiAgentOriginal(row *client.AIAgentResponse) (string, error) {
	var config map[string]any
	if err := json.Unmarshal(row.Config, &config); err != nil {
		return "", err
	}
	tools := row.ToolIDs
	if tools == nil {
		tools = []int64{}
	}
	raw, err := json.Marshal(map[string]any{"config": config, "model_id": row.ModelID, "system_prompt_id": row.SystemPromptID, "tool_ids": tools})
	return string(raw), err
}
func aiAgentBaseline(state *AIAgentOverrideModel) (map[string]any, error) {
	if state.OriginalSettings.IsNull() || state.OriginalSettings.IsUnknown() {
		return nil, fmt.Errorf("missing restoration baseline; refresh or reimport this override")
	}
	var original map[string]any
	err := json.Unmarshal([]byte(state.OriginalSettings.ValueString()), &original)
	return original, err
}
func emptyAIAgentOverride() AIAgentOverrideModel {
	return AIAgentOverrideModel{Config: types.DynamicNull(), ModelID: types.Int64Null(), SystemPromptID: types.Int64Null(), ToolIDs: types.SetNull(types.Int64Type)}
}
func mapAIAgentOverride(row *client.AIAgentResponse, model *AIAgentOverrideModel) {
	model.ID = types.Int64Value(row.ID)
	model.Name = types.StringValue(row.Name)
	model.Type = types.StringValue(row.Type)
}
func (r *AIAgentOverrideResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan AIAgentOverrideModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	row, err := r.client.GetAIAgent(ctx, plan.AgentID.ValueInt64())
	if err != nil {
		resp.Diagnostics.AddError("Error reading AI agent", err.Error())
		return
	}
	if !row.IsBuiltin {
		resp.Diagnostics.AddError("Not a built-in agent", "Use ai_agent for user-defined agents.")
		return
	}
	original, err := aiAgentOriginal(row)
	if err != nil {
		resp.Diagnostics.AddError("Error capturing original settings", err.Error())
		return
	}
	plan.OriginalSettings = types.StringValue(original)
	baseline, err := aiAgentBaseline(&plan)
	if err != nil {
		resp.Diagnostics.AddError("Error capturing original settings", err.Error())
		return
	}
	empty := emptyAIAgentOverride()
	patch, err := aiAgentOverridePatch(&plan, &empty, baseline)
	if err != nil {
		resp.Diagnostics.AddError("Invalid override settings", err.Error())
		return
	}
	if len(patch) > 0 {
		row, err = r.client.UpdateAIAgentSettings(ctx, row.ID, patch)
		if err != nil {
			resp.Diagnostics.AddError("Error overriding AI agent", err.Error())
			return
		}
	}
	mapAIAgentOverride(row, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}
func (r *AIAgentOverrideResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state AIAgentOverrideModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	row, err := r.client.GetAIAgent(ctx, state.AgentID.ValueInt64())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading AI agent override", err.Error())
		return
	}
	if !row.IsBuiltin {
		resp.Diagnostics.AddError("Not a built-in agent", "Only built-in agents support overrides.")
		return
	}
	if state.OriginalSettings.IsNull() || state.OriginalSettings.IsUnknown() {
		original, err := aiAgentOriginal(row)
		if err != nil {
			resp.Diagnostics.AddError("Error capturing original settings", err.Error())
			return
		}
		state.OriginalSettings = types.StringValue(original)
	}
	if !state.Config.IsNull() {
		state.Config, err = tfjson.Reconcile(state.Config, row.Config, nil)
		if err != nil {
			resp.Diagnostics.AddError("Error refreshing AI agent config", err.Error())
			return
		}
	}
	if !state.ModelID.IsNull() {
		if row.ModelID == nil {
			state.ModelID = types.Int64Value(0)
		} else {
			state.ModelID = types.Int64Value(*row.ModelID)
		}
	}
	if !state.SystemPromptID.IsNull() {
		state.SystemPromptID = int64PtrToValue(row.SystemPromptID)
	}
	if !state.ToolIDs.IsNull() {
		state.ToolIDs = int64SliceToSet(row.ToolIDs)
	}
	mapAIAgentOverride(row, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
func (r *AIAgentOverrideResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state AIAgentOverrideModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	baseline, err := aiAgentBaseline(&state)
	if err != nil {
		resp.Diagnostics.AddError("Error restoring AI agent settings", err.Error())
		return
	}
	patch, err := aiAgentOverridePatch(&plan, &state, baseline)
	if err != nil {
		resp.Diagnostics.AddError("Invalid override settings", err.Error())
		return
	}
	row, err := r.client.GetAIAgent(ctx, plan.AgentID.ValueInt64())
	if err != nil {
		resp.Diagnostics.AddError("Error reading AI agent", err.Error())
		return
	}
	if !row.IsBuiltin {
		resp.Diagnostics.AddError("Not a built-in agent", "Only built-in agents support overrides.")
		return
	}
	if len(patch) > 0 {
		row, err = r.client.UpdateAIAgentSettings(ctx, row.ID, patch)
		if err != nil {
			resp.Diagnostics.AddError("Error overriding AI agent", err.Error())
			return
		}
	}
	plan.OriginalSettings = state.OriginalSettings
	mapAIAgentOverride(row, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}
func (r *AIAgentOverrideResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state AIAgentOverrideModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	row, err := r.client.GetAIAgent(ctx, state.AgentID.ValueInt64())
	if client.IsNotFound(err) {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading AI agent", err.Error())
		return
	}
	if !row.IsBuiltin {
		resp.Diagnostics.AddError("Not a built-in agent", "Refusing to reset a user-defined agent.")
		return
	}
	baseline, err := aiAgentBaseline(&state)
	if err != nil {
		resp.Diagnostics.AddError("Error restoring AI agent settings", err.Error())
		return
	}
	empty := emptyAIAgentOverride()
	patch, err := aiAgentOverridePatch(&empty, &state, baseline)
	if err != nil {
		resp.Diagnostics.AddError("Error restoring AI agent settings", err.Error())
		return
	}
	if len(patch) > 0 {
		if _, err = r.client.UpdateAIAgentSettings(ctx, row.ID, patch); err != nil && !client.IsNotFound(err) {
			resp.Diagnostics.AddError("Error restoring AI agent settings", err.Error())
		}
	}
}
func (r *AIAgentOverrideResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil || id <= 0 {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("Expected positive agent ID, got %q", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("agent_id"), id)...)
}
