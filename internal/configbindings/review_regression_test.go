package configbindings

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"net/http"
	"net/http/httptest"
	"terraform-provider-mistershell/internal/client"
	"testing"
)

func TestFailedSensorKeyReleaseRetainsAllOriginalParameters(t *testing.T) {
	writes := 0
	var restored map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if req.Method == "PATCH" {
			writes++
			if writes == 1 {
				w.WriteHeader(500)
				fmt.Fprint(w, `{"success":false,"message":"simulated failure"}`)
				return
			}
			var body struct {
				Parameters map[string]string `json:"parameters"`
			}
			_ = json.NewDecoder(req.Body).Decode(&body)
			restored = body.Parameters
			fmt.Fprint(w, `{"success":true,"data":null}`)
			return
		}
		fmt.Fprint(w, `{"success":true,"data":[{"id":3,"source_type":"index","parameters":{"keep":"****","release":"****"},"enabled":false}]}`)
	}))
	defer server.Close()
	ctx := context.Background()
	r := &bindingResource{def: definitionByName("builtin_sensor_source"), client: client.NewClient(server.URL, "test", server.Client())}
	oldPlan := planFor(t, r, "3", `{"parameters":{"keep":"changed-keep","release":"changed-release"}}`)
	old := tfsdk.State{Schema: oldPlan.Schema, Raw: oldPlan.Raw}
	if dd := setObject(ctx, &old, "original_configuration", object{"parameters": json.RawMessage(`{"keep":"original-keep","release":"original-release"}`)}); dd.HasError() {
		t.Fatal(dd)
	}
	plan := planFor(t, r, "3", `{"parameters":{"keep":"changed-keep"}}`)
	var updated resource.UpdateResponse
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: old}, &updated)
	if !updated.Diagnostics.HasError() {
		t.Fatal("expected failed parameter release")
	}
	var deleted resource.DeleteResponse
	r.Delete(ctx, resource.DeleteRequest{State: updated.State}, &deleted)
	if deleted.Diagnostics.HasError() {
		t.Fatal(deleted.Diagnostics)
	}
	if len(restored) != 2 || restored["release"] != "original-release" {
		t.Fatal("failed release lost original parameter required for destroy recovery")
	}
}

func TestSchedulerImplicitOwnershipRetainsEarlierExplicitOriginal(t *testing.T) {
	d := definitionByName("scheduled_task_config")
	t0 := json.RawMessage(`"2026-11-10T00:00:00Z"`)
	t1 := json.RawMessage(`"2026-11-11T00:00:00Z"`)
	current := object{"next_run": t0, "interval_minutes": json.RawMessage(`30`)}
	_, original, err := prepare(d, object{"next_run": t1}, nil, current, nil)
	if err != nil {
		t.Fatal(err)
	}
	current["next_run"] = t1
	_, original, err = prepare(d, object{"next_run": t1, "interval_minutes": json.RawMessage(`60`)}, original, current, nil)
	if err != nil {
		t.Fatal(err)
	}
	current["interval_minutes"] = json.RawMessage(`60`)
	_, original, err = prepare(d, object{"interval_minutes": json.RawMessage(`60`)}, original, current, nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(restoreBody(original)["next_run"]) != string(t0) {
		t.Fatal("implicit schedule ownership overwrote the earlier explicit original")
	}
}

func TestWholeOwnedObjectRefreshDetectsAddedRemoteKeys(t *testing.T) {
	for _, tc := range []struct{ name, field, prior, remote string }{
		{"metric_config", "options", `{"threshold":80}`, `{"threshold":80,"added":true}`},
		{"snap_config", "options", `{}`, `{"added":true}`},
		{"scheduled_task_config", "config", `{"nested":{"known":1}}`, `{"nested":{"known":1,"added":true}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &bindingResource{def: definitionByName(tc.name)}
			id := "linux.cpu"
			if tc.name == "scheduled_task_config" {
				id = "7"
			}
			plan := planFor(t, r, id, fmt.Sprintf(`{"%s":%s}`, tc.field, tc.prior))
			state := tfsdk.State{Schema: plan.Schema, Raw: plan.Raw}
			if dd := refresh(context.Background(), r.def, &state, object{tc.field: json.RawMessage(tc.remote), "is_enabled": json.RawMessage(`true`)}); dd.HasError() {
				t.Fatal(dd)
			}
			got, dd := getObject(context.Background(), state, "configuration")
			if dd.HasError() {
				t.Fatal(dd)
			}
			var actual, expected any
			_ = json.Unmarshal(got[tc.field], &actual)
			_ = json.Unmarshal([]byte(tc.remote), &expected)
			a, _ := json.Marshal(actual)
			b, _ := json.Marshal(expected)
			if string(a) != string(b) {
				t.Fatalf("whole-object drift hidden: got %s, want %s", a, b)
			}
			if _, ok := got["is_enabled"]; ok {
				t.Fatal("adopted unowned top-level field")
			}
		})
	}
}

func TestMetricRemovalRejectsUnrestorableOriginalNullThreshold(t *testing.T) {
	writes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if req.Method != "GET" {
			writes++
			fmt.Fprint(w, `{"success":true,"data":null}`)
			return
		}
		if req.URL.Path == "/api/v1/metrics/linux.cpu" {
			fmt.Fprint(w, `{"success":true,"data":{"is_enabled":true,"metric_type":"gauge","default_options":{"warning_threshold":null,"critical_threshold":null},"options":{"warning_threshold":80}}}`)
			return
		}
		fmt.Fprint(w, `{"success":true,"data":[{"tag_id":2,"skip_collection":false,"thresholds":{"warning_threshold":null,"critical_threshold":null},"position":0}]}`)
	}))
	defer server.Close()
	r := &bindingResource{def: definitionByName("metric_customizations"), client: client.NewClient(server.URL, "test", server.Client())}
	plan := planFor(t, r, "linux.cpu", `{"items":[]}`)
	var created resource.CreateResponse
	r.Create(context.Background(), resource.CreateRequest{Plan: plan}, &created)
	if !created.Diagnostics.HasError() || writes != 0 {
		t.Fatalf("unrestorable customization removal was not rejected before PUT: writes=%d diagnostics=%v", writes, created.Diagnostics)
	}
}

func TestMetricRestorationGuardHonorsEffectiveOptionsAndRetainedRows(t *testing.T) {
	for _, tc := range []struct{ name, defaults, options, desired string }{
		{"nullable default", `{"warning_threshold":null}`, `{}`, `[]`},
		{"nullable override", `{"warning_threshold":80}`, `{"warning_threshold":null}`, `[]`},
		{"retained original", `{"warning_threshold":80}`, `{}`, `[{"tag_id":2,"thresholds":{"warning_threshold":null}}]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeAPI{objects: map[string]json.RawMessage{"/api/v1/metrics/linux.cpu": json.RawMessage(fmt.Sprintf(`{"default_options":%s,"options":%s}`, tc.defaults, tc.options))}}
			e := engine{api: f, def: definitionByName("metric_customizations")}
			err := e.validateRestoration(context.Background(), "linux.cpu", object{"items": json.RawMessage(tc.desired)}, object{"items": json.RawMessage(`[{"tag_id":2,"thresholds":{"warning_threshold":null}}]`)})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSensorRefreshStillExcludesUnownedParameterKeys(t *testing.T) {
	r := &bindingResource{def: definitionByName("builtin_sensor_source")}
	plan := planFor(t, r, "3", `{"parameters":{"owned":"configured"}}`)
	state := tfsdk.State{Schema: plan.Schema, Raw: plan.Raw}
	dd := refresh(context.Background(), r.def, &state, object{"parameters": json.RawMessage(`{"owned":"****","unowned":"****"}`), "enabled": json.RawMessage(`true`)})
	if dd.HasError() {
		t.Fatal(dd)
	}
	got, dd := getObject(context.Background(), state, "configuration")
	if dd.HasError() {
		t.Fatal(dd)
	}
	var params map[string]string
	_ = json.Unmarshal(got["parameters"], &params)
	if len(params) != 1 || params["owned"] != "configured" {
		t.Fatal("merge-only Sensor parameter ownership changed")
	}
	if _, ok := got["enabled"]; ok {
		t.Fatal("adopted unowned enablement")
	}
}
