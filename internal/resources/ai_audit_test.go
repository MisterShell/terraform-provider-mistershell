package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"net/http"
	"net/http/httptest"
	"strings"
	"terraform-provider-mistershell/internal/client"
	"testing"
)

func TestAIAuditClearAndRefresh(t *testing.T) {
	for _, kind := range []string{"model", "agent", "prompt"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			var body map[string]json.RawMessage
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "PATCH" {
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
				}
				payload := `{"success":true,"data":{"id":1,"name":"test","provider":"ollama","model_id":"llama3","type":"background","system_prompt_id":1,"content":"hello","config":{"token_budget":44},"variable_schema":{"type":"object"}}}`
				if kind != "model" {
					payload = strings.Replace(payload, `"model_id":"llama3"`, `"model_id":null`, 1)
				}
				fmt.Fprint(w, payload)
			}))
			defer srv.Close()
			c := client.NewClient(srv.URL, "test", srv.Client())
			var r resource.Resource
			var model any
			field := "config"
			switch kind {
			case "model":
				r = &AIModelResource{client: c}
				model = &AIModelResourceModel{ID: types.Int64Value(1), Name: types.StringValue("test"), Provider: types.StringValue("ollama"), ModelID: types.StringValue("llama3"), Config: types.DynamicNull()}
			case "agent":
				r = &AIAgentResource{client: c}
				model = &AIAgentResourceModel{ID: types.Int64Value(1), Name: types.StringValue("test"), Type: types.StringValue("background"), SystemPromptID: types.Int64Value(1), Config: types.DynamicNull(), ToolIDs: types.SetNull(types.Int64Type)}
			case "prompt":
				r = &AIPromptResource{client: c}
				model = &AIPromptResourceModel{ID: types.Int64Value(1), Name: types.StringValue("test"), Type: types.StringValue("user"), Content: types.StringValue("hello"), VariableSchema: types.DynamicNull()}
				field = "variable_schema"
			}
			var sr resource.SchemaResponse
			r.Schema(ctx, resource.SchemaRequest{}, &sr)
			plan := tfsdk.Plan{Schema: sr.Schema}
			state := tfsdk.State{Schema: sr.Schema}
			if d := plan.Set(ctx, model); d.HasError() {
				t.Fatal(d)
			}
			if d := state.Set(ctx, model); d.HasError() {
				t.Fatal(d)
			}
			ur := resource.UpdateResponse{State: state}
			r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, &ur)
			if ur.Diagnostics.HasError() {
				t.Fatal(ur.Diagnostics)
			}
			want := "{}"
			if kind == "prompt" {
				want = "null"
			}
			if string(body[field]) != want {
				t.Errorf("clear %s = %s, want %s", field, body[field], want)
			}
			if kind == "agent" && string(body["type"]) != `"background"` {
				t.Errorf("agent update missing mutable type: %s", body["type"])
			}
			rr := resource.ReadResponse{State: ur.State}
			r.Read(ctx, resource.ReadRequest{State: ur.State}, &rr)
			if rr.Diagnostics.HasError() {
				t.Fatal(rr.Diagnostics)
			}
			var value types.Dynamic
			if d := rr.State.GetAttribute(ctx, path.Root(field), &value); d.HasError() {
				t.Fatal(d)
			}
			if value.IsNull() {
				t.Errorf("read discarded remote %s", field)
			}
		})
	}
}
