package resources

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestAIAgentOverrideRestoresOnlyOwnedSettings(t *testing.T) {
	previous, _ := rawJSONToDynamic([]byte(`{"temperature":0.5,"max_steps":7}`))
	next, _ := rawJSONToDynamic([]byte(`{"max_steps":9}`))
	state := AIAgentOverrideModel{Config: previous, ModelID: types.Int64Value(3), ToolIDs: types.SetNull(types.Int64Type)}
	plan := AIAgentOverrideModel{Config: next, ModelID: types.Int64Null(), ToolIDs: types.SetNull(types.Int64Type)}
	original := map[string]any{"config": map[string]any{"temperature": 0.1, "max_steps": 0, "label": "registry"}, "model_id": float64(2)}
	patch, err := aiAgentOverridePatch(&plan, &state, original)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(patch)
	if string(raw) != `{"config":{"max_steps":9,"temperature":0.1},"model_id":2}` {
		t.Fatalf("patch affected unowned fields or failed to restore removed ones: %s", raw)
	}
}

func TestAIAgentOverrideValidationAllowsUnknownRuntimeValues(t *testing.T) {
	ctx := context.Background()
	r := &AIAgentOverrideResource{}
	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	model := emptyAIAgentOverride()
	model.AgentID = types.Int64Value(1)
	model.Config = types.DynamicValue(types.ObjectValueMust(map[string]attr.Type{"temperature": types.NumberType}, map[string]attr.Value{"temperature": types.NumberUnknown()}))
	state := tfsdk.State{Schema: schemaResp.Schema}
	if d := state.Set(ctx, &model); d.HasError() {
		t.Fatal(d)
	}
	config := tfsdk.Config{Schema: schemaResp.Schema, Raw: state.Raw}
	var response resource.ValidateConfigResponse
	r.ValidateConfig(ctx, resource.ValidateConfigRequest{Config: config}, &response)
	if response.Diagnostics.HasError() {
		t.Fatal(response.Diagnostics)
	}
}
