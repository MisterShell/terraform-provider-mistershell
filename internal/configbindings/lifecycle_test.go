package configbindings

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"net/http"
	"net/http/httptest"
	"terraform-provider-mistershell/internal/client"
	"terraform-provider-mistershell/internal/tfjson"
	"testing"
)

func planFor(t *testing.T, r *bindingResource, id, config string) tfsdk.Plan {
	t.Helper()
	ctx := context.Background()
	var schema resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schema)
	value, err := tfjson.FromRawJSON(json.RawMessage(config))
	if err != nil {
		t.Fatal(err)
	}
	plan := tfsdk.Plan{Schema: schema.Schema}
	model := struct {
		ID            types.String  `tfsdk:"id"`
		Configuration types.Dynamic `tfsdk:"configuration"`
		Original      types.Dynamic `tfsdk:"original_configuration"`
	}{types.StringValue(id), value, types.DynamicUnknown()}
	if r.def.name == "builtin_sensor_source" {
		sensor := struct {
			ID            types.String  `tfsdk:"id"`
			Configuration types.Dynamic `tfsdk:"configuration"`
			Original      types.Dynamic `tfsdk:"original_configuration"`
			Restore       types.Dynamic `tfsdk:"restore_parameters"`
		}{model.ID, model.Configuration, model.Original, types.DynamicNull()}
		if dd := plan.Set(ctx, sensor); dd.HasError() {
			t.Fatal(dd)
		}
	} else {
		if dd := plan.Set(ctx, model); dd.HasError() {
			t.Fatal(dd)
		}
	}
	return plan
}
func TestBindingLifecycleRestoresOnlyOwnedConfiguration(t *testing.T) {
	current := map[string]any{"key": "linux.cpu", "is_enabled": true, "display_order": 4, "options": map[string]any{"warning_threshold": 60}}
	writes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/api/v1/metrics/linux.cpu" {
			t.Errorf("unexpected URL %s", req.URL.Path)
		}
		if req.Method != "GET" {
			if req.Method != "PATCH" {
				t.Errorf("binding must not create/delete backend object: %s", req.Method)
			}
			writes++
			var body map[string]any
			_ = json.NewDecoder(req.Body).Decode(&body)
			for k, v := range body {
				current[k] = v
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": current})
	}))
	defer server.Close()
	r := &bindingResource{def: definitionByName("metric_config"), client: client.NewClient(server.URL, "test", server.Client())}
	ctx := context.Background()
	plan := planFor(t, r, "linux.cpu", `{"is_enabled":false,"options":{"warning_threshold":80}}`)
	var created resource.CreateResponse
	r.Create(ctx, resource.CreateRequest{Plan: plan}, &created)
	if created.Diagnostics.HasError() {
		t.Fatal(created.Diagnostics)
	}
	original, dd := getObject(ctx, created.State, "original_configuration")
	if dd.HasError() {
		t.Fatal(dd)
	}
	if len(original) != 2 || string(original["is_enabled"]) != "true" {
		t.Fatalf("incorrect snapshot: %v", original)
	}
	// External changes to unowned display_order survive refresh and destroy.
	current["display_order"] = 27
	var read resource.ReadResponse
	r.Read(ctx, resource.ReadRequest{State: created.State}, &read)
	if read.Diagnostics.HasError() {
		t.Fatal(read.Diagnostics)
	}
	next := planFor(t, r, "linux.cpu", `{"is_enabled":true}`)
	var updated resource.UpdateResponse
	r.Update(ctx, resource.UpdateRequest{Plan: next, State: read.State}, &updated)
	if updated.Diagnostics.HasError() {
		t.Fatal(updated.Diagnostics)
	}
	options := current["options"].(map[string]any)
	if options["warning_threshold"] != float64(60) {
		t.Fatal("removed options were not restored")
	}
	var destroyed resource.DeleteResponse
	r.Delete(ctx, resource.DeleteRequest{State: updated.State}, &destroyed)
	if destroyed.Diagnostics.HasError() {
		t.Fatal(destroyed.Diagnostics)
	}
	if current["is_enabled"] != true || current["display_order"] != 27 || writes != 3 {
		t.Fatalf("wrong lifecycle %#v writes=%d", current, writes)
	}
}
func TestImportAndDatasourceDoNotWrite(t *testing.T) {
	writes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != "GET" {
			writes++
		}
		w.Header().Set("Content-Type", "application/json")
		if req.URL.Path == "/api/v1/tasks" {
			fmt.Fprint(w, `{"success":true,"data":[{"id":7,"task_type":"cleanup"}],"meta":{"page":1,"size":100,"total":1,"pages":1}}`)
		} else {
			fmt.Fprint(w, `{"success":true,"data":{"id":7,"task_type":"cleanup","is_enabled":true,"task_name":"Cleanup","config":{},"next_run":null,"timeout_seconds":300,"interval_minutes":60}}`)
		}
	}))
	defer server.Close()
	ctx := context.Background()
	c := client.NewClient(server.URL, "test", server.Client())
	r := &bindingResource{def: definitionByName("scheduled_task_config"), client: c}
	var sr resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &sr)
	imported := resource.ImportStateResponse{State: tfsdk.State{Schema: sr.Schema, Raw: tftypes.NewValue(sr.Schema.Type().TerraformType(ctx), nil)}}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "7"}, &imported)
	if imported.Diagnostics.HasError() {
		t.Fatal(imported.Diagnostics)
	}
	original, dd := getObject(ctx, imported.State, "original_configuration")
	if dd.HasError() || len(original) != 6 {
		t.Fatalf("import did not capture baseline: %v %v", original, dd)
	}
	ds := &bindingDataSource{def: r.def, client: c}
	var dsSchema datasource.SchemaResponse
	ds.Schema(ctx, datasource.SchemaRequest{}, &dsSchema)
	state := tfsdk.State{Schema: dsSchema.Schema}
	if dd := state.Set(ctx, struct {
		ID            types.String  `tfsdk:"id"`
		TaskType      types.String  `tfsdk:"task_type"`
		Configuration types.Dynamic `tfsdk:"configuration"`
	}{types.StringNull(), types.StringValue("cleanup"), types.DynamicNull()}); dd.HasError() {
		t.Fatal(dd)
	}
	var read datasource.ReadResponse
	ds.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: dsSchema.Schema, Raw: state.Raw}}, &read)
	if read.Diagnostics.HasError() {
		t.Fatal(read.Diagnostics)
	}
	var id types.String
	read.State.GetAttribute(ctx, path.Root("id"), &id)
	if id.ValueString() != "7" || writes != 0 {
		t.Fatalf("lookup/import mutated or wrong id %s, %d writes", id.ValueString(), writes)
	}
}
func TestSensorReadMasksAndRejectsCustomOwnership(t *testing.T) {
	f := &fakeAPI{lists: map[string][]json.RawMessage{"/api/v1/sensors/sources": {json.RawMessage(`{"id":3,"source_type":"custom"}`)}}}
	if _, err := (engine{api: f, def: definitionByName("builtin_sensor_source")}).read(context.Background(), "3"); err == nil {
		t.Fatal("accepted custom source")
	}
	r := &bindingResource{def: definitionByName("builtin_sensor_source")}
	plan := planFor(t, r, "3", `{"parameters":{"token":"plaintext"},"enabled":false}`)
	state := tfsdk.State{Schema: plan.Schema, Raw: plan.Raw}
	if dd := refresh(context.Background(), r.def, &state, object{"parameters": json.RawMessage(`{"token":"****"}`), "enabled": json.RawMessage(`false`)}); dd.HasError() {
		t.Fatal(dd)
	}
	obj, dd := getObject(context.Background(), state, "configuration")
	if dd.HasError() || string(obj["parameters"]) != `{"token":"plaintext"}` {
		t.Fatal("masked secret did not retain configured value")
	}
}
func TestSchedulerRestoresImplicitNextRunWithoutRewindingUpdates(t *testing.T) {
	d := definitionByName("scheduled_task_config")
	current := object{"is_enabled": json.RawMessage(`true`), "next_run": json.RawMessage(`"2026-11-10T00:00:00Z"`)}
	desired := object{"is_enabled": json.RawMessage(`false`)}
	body, snapshot, err := prepare(d, desired, nil, current, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := body["next_run"]; ok {
		t.Fatal("create should allow normal backend scheduling")
	}
	body, _, err = prepare(d, desired, snapshot, current, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := body["next_run"]; ok {
		t.Fatal("update rewound a moving schedule")
	}
	restored := restoreBody(snapshot)
	if string(restored["next_run"]) != string(current["next_run"]) {
		t.Fatal("destroy lost original next_run")
	}
}
func TestSensorReplacementGuardRunsBeforeParametersWrite(t *testing.T) {
	f := &fakeAPI{lists: map[string][]json.RawMessage{"/api/v1/sensors/sources": {json.RawMessage(`{"id":3,"source_type":"index","replaces":["old"]}`), json.RawMessage(`{"id":4,"name":"old","enabled":true}`)}}}
	err := (engine{api: f, def: definitionByName("builtin_sensor_source")}).write(context.Background(), "3", object{"enabled": json.RawMessage(`true`), "parameters": json.RawMessage(`{"token":"new"}`)})
	if err == nil || len(f.calls) != 0 {
		t.Fatal("replacement guard must reject before writes")
	}
}
func TestCustomizationOrderAndDisabledParent(t *testing.T) {
	d := definitionByName("snap_customizations")
	f := &fakeAPI{objects: map[string]json.RawMessage{"/api/v1/snaps/linux.config": json.RawMessage(`{"is_enabled":true}`)}, lists: map[string][]json.RawMessage{"/api/v1/snaps/linux.config/customizations": {json.RawMessage(`{"tag_id":9,"skip_collection":true,"position":0}`), json.RawMessage(`{"tag_id":2,"skip_collection":false,"position":1}`)}}}
	e := engine{api: f, def: d}
	ctx := context.Background()
	current, err := e.read(ctx, "linux.config")
	if err != nil {
		t.Fatal(err)
	}
	var rows []map[string]any
	_ = json.Unmarshal(current["items"], &rows)
	if rows[0]["tag_id"] != float64(9) || rows[1]["tag_id"] != float64(2) {
		t.Fatal("lost list order")
	}
	if _, ok := rows[0]["position"]; ok {
		t.Fatal("response-only position sent back")
	}
	if err = e.write(ctx, "linux.config", current); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 1 || f.calls[0].method != "PUT" {
		t.Fatal("wrong customization adapter")
	}
	f.objects["/api/v1/snaps/linux.config"] = json.RawMessage(`{"is_enabled":false}`)
	if err = e.write(ctx, "linux.config", current); err == nil || len(f.calls) != 1 {
		t.Fatal("disabled parent must fail before PUT")
	}
}

func TestFailedWriteRetainsSnapshotForRecovery(t *testing.T) {
	writes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if req.Method == "PATCH" {
			writes++
			if writes == 1 {
				w.WriteHeader(500)
				fmt.Fprint(w, `{"success":false,"message":"simulated failure"}`)
				return
			}
		}
		fmt.Fprint(w, `{"success":true,"data":{"key":"linux.cpu","is_enabled":true,"display_order":7,"options":{}}}`)
	}))
	defer server.Close()
	r := &bindingResource{def: definitionByName("metric_config"), client: client.NewClient(server.URL, "test", server.Client())}
	plan := planFor(t, r, "linux.cpu", `{"is_enabled":false}`)
	ctx := context.Background()
	var created resource.CreateResponse
	r.Create(ctx, resource.CreateRequest{Plan: plan}, &created)
	if !created.Diagnostics.HasError() {
		t.Fatal("expected failed write")
	}
	original, dd := getObject(ctx, created.State, "original_configuration")
	if dd.HasError() || string(original["is_enabled"]) != "true" {
		t.Fatal("lost recovery snapshot after API failure")
	}
	var destroyed resource.DeleteResponse
	r.Delete(ctx, resource.DeleteRequest{State: created.State}, &destroyed)
	if destroyed.Diagnostics.HasError() || writes != 2 {
		t.Fatalf("could not restore failed create: %v", destroyed.Diagnostics)
	}
}

func TestRemovedSensorParameterRestoresOriginalWithoutClearing(t *testing.T) {
	d := definitionByName("builtin_sensor_source")
	desired := object{"parameters": json.RawMessage(`{"keep":"new-keep"}`)}
	original := object{"parameters": json.RawMessage(`{"keep":"old-keep","release":"old-release"}`)}
	current := object{"parameters": json.RawMessage(`{"keep":"****","release":"****","unmanaged":"****"}`)}
	body, snapshot, err := prepare(d, desired, original, current, nil)
	if err != nil {
		t.Fatal(err)
	}
	var params map[string]string
	_ = json.Unmarshal(body["parameters"], &params)
	if params["release"] != "old-release" || params["keep"] != "new-keep" || len(params) != 2 {
		t.Fatal("removal did not restore only owned parameters")
	}
	_ = json.Unmarshal(snapshot["parameters"], &params)
	var saved map[string]string
	_ = json.Unmarshal(snapshot["parameters"], &saved)
	if len(saved) != 1 || saved["keep"] != "old-keep" {
		t.Fatal("released parameter remains owned")
	}
}

func TestSensorImportDoesNotAdoptMaskedSecrets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != "GET" {
			t.Errorf("import wrote %s", req.Method)
		}
		fmt.Fprint(w, `{"success":true,"data":[{"id":3,"name":"feed","source_type":"index","enabled":false,"parameters":{"token":"****"}}]}`)
	}))
	defer server.Close()
	ctx := context.Background()
	r := &bindingResource{def: definitionByName("builtin_sensor_source"), client: client.NewClient(server.URL, "test", server.Client())}
	var sr resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &sr)
	imported := resource.ImportStateResponse{State: tfsdk.State{Schema: sr.Schema, Raw: tftypes.NewValue(sr.Schema.Type().TerraformType(ctx), nil)}}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "3"}, &imported)
	if imported.Diagnostics.HasError() {
		t.Fatal(imported.Diagnostics)
	}
	for _, attribute := range []string{"configuration", "original_configuration"} {
		obj, dd := getObject(ctx, imported.State, attribute)
		if dd.HasError() {
			t.Fatal(dd)
		}
		if _, ok := obj["parameters"]; ok {
			t.Fatal("import adopted masked parameters")
		}
	}
}

func TestSnapshotMustBeAvailableBeforeAnyWrite(t *testing.T) {
	writes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != "GET" {
			writes++
		}
		fmt.Fprint(w, `{"success":true,"data":{"key":"linux.cpu"}}`)
	}))
	defer server.Close()
	r := &bindingResource{def: definitionByName("metric_config"), client: client.NewClient(server.URL, "test", server.Client())}
	plan := planFor(t, r, "linux.cpu", `{"is_enabled":false}`)
	var created resource.CreateResponse
	r.Create(context.Background(), resource.CreateRequest{Plan: plan}, &created)
	if !created.Diagnostics.HasError() || writes != 0 || !created.State.Raw.IsNull() {
		t.Fatal("unrestorable create wrote or retained invalid ownership")
	}
}
