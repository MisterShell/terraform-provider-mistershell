package resources

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"net/http"
	"net/http/httptest"
	"terraform-provider-mistershell/internal/client"
	"testing"
)

func TestSensorRuleOverrideMissingCatalogPreservesStateAndCanReset(t *testing.T) {
	reset := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodGet {
			w.WriteHeader(404)
			fmt.Fprint(w, `{"success":false,"message":"Rule not found"}`)
			return
		}
		if req.Method == http.MethodPost && req.URL.Path == "/api/v1/sensors/rule-overrides" {
			reset = true
			fmt.Fprint(w, `{"success":true,"data":{"changed":1}}`)
			return
		}
		t.Errorf("unexpected request %s %s", req.Method, req.URL.Path)
	}))
	defer srv.Close()
	r := &SensorRuleOverrideResource{client: client.NewClient(srv.URL, "test", srv.Client())}
	ctx := context.Background()
	var sr resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &sr)
	model := SensorRuleOverrideModel{ID: types.StringValue("1:9900001"), GID: types.Int64Value(1), SID: types.Int64Value(9900001), Action: types.StringValue("disable")}
	state := tfsdk.State{Schema: sr.Schema}
	if d := state.Set(ctx, &model); d.HasError() {
		t.Fatal(d)
	}
	rr := resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, &rr)
	if !rr.Diagnostics.HasError() {
		t.Error("missing catalog must report unverifiable durable override")
	}
	if rr.State.Raw.IsNull() {
		t.Fatal("catalog disappearance discarded override ownership")
	}
	var dr resource.DeleteResponse
	r.Delete(ctx, resource.DeleteRequest{State: rr.State}, &dr)
	if dr.Diagnostics.HasError() {
		t.Fatal(dr.Diagnostics)
	}
	if !reset {
		t.Error("delete did not reset the durable override")
	}
}

func TestSensorRuleOverrideCreateRequiresImportForExistingOverride(t *testing.T) {
	writes := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			writes++
		}
		fmt.Fprint(w, `{"success":true,"data":{"gid":1,"sid":9900001,"override_action":"enable","default_enabled":true,"effective_enabled":true,"pending":true}}`)
	}))
	defer srv.Close()
	r := &SensorRuleOverrideResource{client: client.NewClient(srv.URL, "test", srv.Client())}
	ctx := context.Background()
	var sr resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &sr)
	plan := tfsdk.Plan{Schema: sr.Schema}
	model := SensorRuleOverrideModel{GID: types.Int64Value(1), SID: types.Int64Value(9900001), Action: types.StringValue("disable")}
	if d := plan.Set(ctx, &model); d.HasError() {
		t.Fatal(d)
	}
	response := resource.CreateResponse{State: tfsdk.State{Schema: sr.Schema}}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, &response)
	if !response.Diagnostics.HasError() {
		t.Error("create must require import before taking over an existing override")
	}
	if writes != 0 {
		t.Fatalf("create changed existing override: %d writes", writes)
	}
}
