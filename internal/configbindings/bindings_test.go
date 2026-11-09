package configbindings

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"
)

type call struct {
	method, path string
	body         map[string]any
}
type fakeAPI struct {
	objects map[string]json.RawMessage
	lists   map[string][]json.RawMessage
	calls   []call
}

func (f *fakeAPI) ConfigurationRequest(_ context.Context, method, path string, body any) (json.RawMessage, error) {
	if method == "GET" {
		return f.objects[path], nil
	}
	b, _ := json.Marshal(body)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	f.calls = append(f.calls, call{method, path, m})
	return json.RawMessage(`{}`), nil
}
func (f *fakeAPI) ConfigurationList(_ context.Context, p string, _ url.Values) ([]json.RawMessage, error) {
	return f.lists[p], nil
}
func TestRejectUnknownAndInvalidFields(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"scheduled_task_config", `{"interval_minutes":0}`}, {"scheduled_task_config", `{"timeout_seconds":2}`},
		{"fact_collection_config", `{"options":{}}`}, {"metric_config", `{"options":null}`},
		{"local_auth_policy", `{"mfa_method":"email"}`}, {"snap_customizations", `{"items":[{"tag_id":1},{"tag_id":1}]}`},
	} {
		var b map[string]json.RawMessage
		_ = json.Unmarshal([]byte(tc.body), &b)
		if err := validate(definitionByName(tc.name), b); err == nil {
			t.Errorf("accepted %s %s", tc.name, tc.body)
		}
	}
}
func TestSensorUsesParamsThenDedicatedEnable(t *testing.T) {
	f := &fakeAPI{lists: map[string][]json.RawMessage{"/api/v1/sensors/sources": {json.RawMessage(`{"id":3,"name":"feed","source_type":"index","enabled":false,"replaces":[]}`)}}}
	e := engine{api: f, def: definitionByName("builtin_sensor_source")}
	err := e.write(context.Background(), "3", map[string]json.RawMessage{"enabled": json.RawMessage(`true`), "parameters": json.RawMessage(`{"token":"new"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 2 || f.calls[0].path != "/api/v1/sensors/sources/3/params" || f.calls[1].path != "/api/v1/sensors/sources/3/enable" {
		t.Fatalf("wrong adapter calls: %#v", f.calls)
	}
}
func TestSensorRejectsUnrestorableParameters(t *testing.T) {
	d := definitionByName("builtin_sensor_source")
	for _, remote := range []string{`{"parameters":{}}`, `{"parameters":{"token":"****"}}`} {
		var current map[string]json.RawMessage
		_ = json.Unmarshal([]byte(remote), &current)
		_, _, err := prepare(d, map[string]json.RawMessage{"parameters": json.RawMessage(`{"token":"new"}`)}, nil, current, nil)
		if err == nil {
			t.Fatal("accepted parameters without restorable original")
		}
	}
}
func TestRemovalRestoresOriginalAndOnlyOwnedFields(t *testing.T) {
	desired := map[string]json.RawMessage{"is_enabled": json.RawMessage(`true`)}
	original := map[string]json.RawMessage{"is_enabled": json.RawMessage(`false`), "options": json.RawMessage(`{"warning_threshold":40}`)}
	current := map[string]json.RawMessage{"is_enabled": json.RawMessage(`false`), "options": json.RawMessage(`{"warning_threshold":80}`), "display_order": json.RawMessage(`99`)}
	body, snapshot, err := prepare(definitionByName("metric_config"), desired, original, current, nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(body["options"]) != `{"warning_threshold":40}` {
		t.Fatal("did not restore removed owned field")
	}
	if _, ok := body["display_order"]; ok {
		t.Fatal("overwrote unowned field")
	}
	if len(snapshot) != 1 {
		t.Fatal("did not release removed field")
	}
}
func TestLocalEmailBaselineCannotBeOverwritten(t *testing.T) {
	_, _, err := prepare(definitionByName("local_auth_policy"), map[string]json.RawMessage{"mfa_method": json.RawMessage(`"totp"`)}, nil, map[string]json.RawMessage{"mfa_method": json.RawMessage(`"email"`)}, nil)
	if err == nil {
		t.Fatal("email baseline cannot be restored without interactive proof")
	}
}

func TestSensorCanAddOwnershipOfAnExistingParameterWithOriginal(t *testing.T) {
	body, snapshot, err := prepare(definitionByName("builtin_sensor_source"), object{"parameters": json.RawMessage(`{"old":"changed","added":"changed-too"}`)}, object{"parameters": json.RawMessage(`{"old":"original"}`)}, object{"parameters": json.RawMessage(`{"old":"****","added":"****"}`)}, object{"added": json.RawMessage(`"original-added"`)})
	if err != nil {
		t.Fatal(err)
	}
	if len(body) == 0 {
		t.Fatal("no update")
	}
	var saved map[string]string
	_ = json.Unmarshal(snapshot["parameters"], &saved)
	if saved["added"] != "original-added" || saved["old"] != "original" {
		t.Fatal("lost ownership originals")
	}
}
