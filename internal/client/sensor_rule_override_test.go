package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSensorRuleOverrideStagesOneRuleAndResets(t *testing.T) {
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/sensors/rule-catalog/1/900001":
			fmt.Fprint(w, `{"success":true,"data":{"gid":1,"sid":900001,"override_action":"disable","effective_enabled":false,"default_enabled":true,"pending":true,"raw":"alert tcp any any -> any any (sid:900001;)"}}`)
		case "POST /api/v1/sensors/rule-overrides":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			bodies = append(bodies, body)
			fmt.Fprint(w, `{"success":true,"data":{"changed":1}}`)
		default:
			t.Errorf("unexpected request, must never globally apply: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "test", srv.Client())
	ctx := context.Background()
	row, err := c.GetSensorCatalogRule(ctx, 1, 900001)
	if err != nil || row.OverrideAction == nil || *row.OverrideAction != "disable" {
		t.Fatalf("read override: %#v %v", row, err)
	}
	if err = c.SetSensorRuleOverride(ctx, 1, 900001, "disable"); err != nil {
		t.Fatal(err)
	}
	if err = c.ResetSensorRuleOverride(ctx, 1, 900001); err != nil {
		t.Fatal(err)
	}
	if len(bodies) != 2 {
		t.Fatal(bodies)
	}
	for i, b := range bodies {
		items, ok := b["items"].([]any)
		if !ok || len(items) != 1 {
			t.Fatalf("not a single-rule operation: %#v", b)
		}
		op := items[0].(map[string]any)
		if op["gid"] != float64(1) || op["sid"] != float64(900001) {
			t.Fatal(op)
		}
		if i == 1 && op["reset"] != true {
			t.Fatal(op)
		}
	}
}
