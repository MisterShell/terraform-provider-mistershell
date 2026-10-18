package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"net/http"
	"net/http/httptest"
	"terraform-provider-mistershell/internal/client"
	"testing"
)

func TestAIGuardrailRestoresDisabledBaseline(t *testing.T) {
	enabled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			fmt.Fprintf(w, `{"success":true,"data":[{"resource_type":"generic_ssh","allowlist_enabled":%t}]}`, enabled)
			return
		case http.MethodPut:
			var body struct {
				Enabled bool `json:"allowlist_enabled"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			enabled = body.Enabled
		case http.MethodDelete:
			enabled = true
		}
		fmt.Fprintf(w, `{"success":true,"data":{"resource_type":"generic_ssh","allowlist_enabled":%t}}`, enabled)
	}))
	defer srv.Close()
	r := &AIGuardrailResource{client: client.NewClient(srv.URL, "test", srv.Client())}
	ctx := context.Background()
	var sr resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &sr)
	plan := tfsdk.Plan{Schema: sr.Schema}
	model := AIGuardrailResourceModel{ResourceType: types.StringValue("generic_ssh"), AllowlistEnabled: types.BoolValue(true)}
	if d := plan.Set(ctx, &model); d.HasError() {
		t.Fatal(d)
	}
	created := resource.CreateResponse{State: tfsdk.State{Schema: sr.Schema}}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, &created)
	if created.Diagnostics.HasError() {
		t.Fatal(created.Diagnostics)
	}
	if !enabled {
		t.Fatal("create failed to apply desired toggle")
	}
	var deleted resource.DeleteResponse
	r.Delete(ctx, resource.DeleteRequest{State: created.State}, &deleted)
	if deleted.Diagnostics.HasError() {
		t.Fatal(deleted.Diagnostics)
	}
	if enabled {
		t.Fatal("destroy discarded pre-existing disabled toggle instead of restoring it")
	}
}
