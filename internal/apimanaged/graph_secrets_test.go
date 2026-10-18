package apimanaged

import (
	"context"
	"encoding/json"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"terraform-provider-mistershell/internal/tfjson"
	"testing"
)

func TestPlaybookGraphSecretsRefresh(t *testing.T) {
	ctx := context.Background()
	var d Definition
	for _, candidate := range Definitions() {
		if candidate.Name == "automation_playbook" {
			d = candidate
		}
	}
	var graph Field
	for _, f := range d.Fields {
		if f.Name == "graph" {
			graph = f
		}
	}
	if !graph.Sensitive {
		t.Error("graph must be sensitive because webhook nodes contain credentials")
	}
	d.Fields = []Field{graph}
	var schema resource.SchemaResponse
	NewResource(d).Schema(ctx, resource.SchemaRequest{}, &schema)
	if !schema.Schema.Attributes["graph"].IsSensitive() {
		t.Error("resource graph schema must redact secrets")
	}
	prior, err := tfjson.FromRawJSON(json.RawMessage(`{"nodes":[{"id":"bearer","config":{"auth":{"token":"token-secret"}}},{"id":"basic","config":{"auth":{"password":"password-secret"}}},{"id":"header","config":{"auth":{"header_value":"header-secret"}}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	state := tfsdk.State{Schema: schema.Schema}
	if diags := state.Set(ctx, &struct {
		ID    types.String  `tfsdk:"id"`
		Graph types.Dynamic `tfsdk:"graph"`
	}{ID: types.StringValue("1"), Graph: prior}); diags.HasError() {
		t.Fatal(diags)
	}
	raw := json.RawMessage(`{"id":1,"graph":{"nodes":[{"id":"bearer","config":{"auth":{"token":"****"}}},{"id":"basic","config":{"auth":{"password":"****"}}},{"id":"header","config":{"auth":{"header_value":"****"}}}]}}`)
	if diags := refresh(ctx, d, &state, raw, false); diags.HasError() {
		t.Fatal(diags)
	}
	var got types.Dynamic
	if diags := state.GetAttribute(ctx, path.Root("graph"), &got); diags.HasError() {
		t.Fatal(diags)
	}
	if !got.Equal(prior) {
		t.Error("refresh replaced webhook credentials with masks")
	}
	// Import has no previous credential. Its underlying dynamic value must
	// contain nulls, never API masks that could be mistaken for credentials.
	if diags := state.SetAttribute(ctx, path.Root("graph"), types.DynamicNull()); diags.HasError() {
		t.Fatal(diags)
	}
	if diags := refresh(ctx, d, &state, raw, false); diags.HasError() {
		t.Fatal(diags)
	}
	if diags := state.GetAttribute(ctx, path.Root("graph"), &got); diags.HasError() {
		t.Fatal(diags)
	}
	importedRaw, err := tfjson.ToRawJSON(got)
	if err != nil {
		t.Fatal(err)
	}
	var imported struct {
		Nodes []struct {
			Config struct {
				Auth map[string]any `json:"auth"`
			} `json:"config"`
		} `json:"nodes"`
	}
	if err = json.Unmarshal(importedRaw, &imported); err != nil {
		t.Fatal(err)
	}
	for i, key := range []string{"token", "password", "header_value"} {
		value, present := imported.Nodes[i].Config.Auth[key]
		if !present || value != nil {
			t.Errorf("imported %s must be a null underlying value", key)
		}
	}
}
