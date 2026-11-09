package apimanaged

import (
	"context"
	"encoding/json"
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

func contract(t *testing.T, name string) Definition {
	t.Helper()
	for _, d := range Definitions() {
		if d.Name == name {
			return d
		}
	}
	t.Fatalf("missing contract %s", name)
	return Definition{}
}
func initialState(t *testing.T, d Definition) tfsdk.State {
	t.Helper()
	ctx := context.Background()
	var response resource.SchemaResponse
	NewResource(d).Schema(ctx, resource.SchemaRequest{}, &response)
	typ := response.Schema.Type().TerraformType(ctx)
	values := map[string]tftypes.Value{}
	for k, v := range typ.(tftypes.Object).AttributeTypes {
		values[k] = tftypes.NewValue(v, nil)
	}
	state := tfsdk.State{Schema: response.Schema, Raw: tftypes.NewValue(typ, values)}
	for _, f := range d.Fields {
		if len(f.Default) > 0 {
			if ds := refresh(ctx, Definition{Fields: []Field{f}}, &state, json.RawMessage(`{"`+f.Name+`":`+string(f.Default)+`}`), false); ds.HasError() {
				t.Fatal(ds)
			}
		}
	}
	return state
}
func set(t *testing.T, s *tfsdk.State, name string, v any) {
	t.Helper()
	if d := s.SetAttribute(context.Background(), path.Root(name), v); d.HasError() {
		t.Fatal(d)
	}
}
func TestCanaryCRUDPreservesEnrollmentAndRecovery(t *testing.T) {
	ctx := context.Background()
	d := contract(t, "canary")
	model := map[string]any{"id": float64(7), "status": "offline", "health": map[string]any{}, "counters": map[string]any{}, "name": "decoy", "location_id": float64(2), "is_enabled": true, "description": "initial", "decoy_interface": "eth1", "ip_ignorelist": []any{}, "rate_limit_per_min": float64(600)}
	recovery := false
	deleted := false
	putNull := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("missing API authentication")
		}
		w.Header().Set("Content-Type", "application/json")
		var result any = model
		switch r.Method {
		case "POST":
			copy := map[string]any{}
			for k, v := range model {
				copy[k] = v
			}
			copy["token"] = "one-time"
			result = copy
		case "PUT":
			var changes map[string]any
			_ = json.NewDecoder(r.Body).Decode(&changes)
			v, present := changes["decoy_interface"]
			putNull = present && v == nil
			for k, v := range changes {
				model[k] = v
			}
		case "DELETE":
			deleted = true
			result = nil
		case "GET":
			if recovery {
				result = map[string]any{"id": 7, "name": "decoy", "location_id": 2, "is_enabled": true, "status": "offline", "license_reason": "expired"}
			}
		default:
			t.Errorf("unexpected method %s", r.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": result})
	}))
	defer server.Close()
	r := &ManagedResource{definition: d, client: client.NewClient(server.URL, "test-key", server.Client())}
	plan := initialState(t, d)
	set(t, &plan, "name", types.StringValue("decoy"))
	set(t, &plan, "location_id", types.Int64Value(2))
	set(t, &plan, "description", types.StringValue("initial"))
	set(t, &plan, "decoy_interface", types.StringValue("eth1"))
	var created resource.CreateResponse
	r.Create(ctx, resource.CreateRequest{Plan: tfsdk.Plan{Schema: plan.Schema, Raw: plan.Raw}}, &created)
	if created.Diagnostics.HasError() {
		t.Fatal(created.Diagnostics)
	}
	var token types.String
	_ = created.State.GetAttribute(ctx, path.Root("token"), &token)
	if token.ValueString() != "one-time" {
		t.Fatal("lost enrollment token")
	}
	recovery = true
	var read resource.ReadResponse
	r.Read(ctx, resource.ReadRequest{State: created.State}, &read)
	if read.Diagnostics.HasError() {
		t.Fatal(read.Diagnostics)
	}
	var description types.String
	_ = read.State.GetAttribute(ctx, path.Root("description"), &description)
	if description.ValueString() != "initial" {
		t.Fatal("recovery response erased protected configuration")
	}
	recovery = false
	plan = read.State
	set(t, &plan, "decoy_interface", types.StringNull())
	var updated resource.UpdateResponse
	r.Update(ctx, resource.UpdateRequest{State: read.State, Plan: tfsdk.Plan{Schema: plan.Schema, Raw: plan.Raw}}, &updated)
	if updated.Diagnostics.HasError() {
		t.Fatal(updated.Diagnostics)
	}
	if !putNull {
		t.Fatal("nullable interface not cleared explicitly")
	}
	_ = updated.State.GetAttribute(ctx, path.Root("token"), &token)
	if token.ValueString() != "one-time" {
		t.Fatal("update lost enrollment token")
	}
	var removed resource.DeleteResponse
	r.Delete(ctx, resource.DeleteRequest{State: updated.State}, &removed)
	if removed.Diagnostics.HasError() || !deleted {
		t.Fatalf("delete failed %v", removed.Diagnostics)
	}
}
func TestCanaryBuiltinServiceCannotImportUpdateOrDelete(t *testing.T) {
	mutations := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			mutations++
		}
		_, _ = w.Write([]byte(`{"success":true,"data":{"id":1,"seed_key":"canary:ssh","enabled":false}}`))
	}))
	defer server.Close()
	r := &ManagedResource{definition: contract(t, "canary_service"), client: client.NewClient(server.URL, "key", server.Client())}
	if err := r.validateImportOwnership(context.Background(), "1"); err == nil {
		t.Fatal("accepted built-in import")
	}
	if err := r.guardCanaryOwnership(context.Background(), "1"); err == nil {
		t.Fatal("accepted built-in mutation")
	}
	if mutations != 0 {
		t.Fatal("ownership check mutated server")
	}
}
func TestCanaryReadAuthorizationFailureKeepsResource(t *testing.T) {
	for _, status := range []int{401, 403, 503} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"success":false,"message":"unavailable"}`))
			}))
			defer server.Close()
			d := contract(t, "canary")
			state := initialState(t, d)
			set(t, &state, "id", types.StringValue("7"))
			r := &ManagedResource{definition: d, client: client.NewClient(server.URL, "key", server.Client())}
			resp := resource.ReadResponse{State: state}
			r.Read(context.Background(), resource.ReadRequest{State: state}, &resp)
			if !resp.Diagnostics.HasError() || resp.State.Raw.IsNull() {
				t.Fatal("authorization or outage treated as absent")
			}
		})
	}
}
func TestSyslogRuleUsesListAndExplicitNullPatch(t *testing.T) {
	ctx := context.Background()
	d := contract(t, "syslog_rule")
	patched := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			if r.URL.Path != "/api/v1/collector/policy" {
				t.Errorf("used nonexistent detail GET %s", r.URL.Path)
			}
			_, _ = w.Write([]byte(`{"success":true,"data":[{"id":8,"name":"syslog","msg_contains":null}]}`))
		case "PATCH":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			v, ok := body["msg_contains"]
			patched = ok && v == nil
			_, _ = w.Write([]byte(`{"success":true,"data":{"id":8}}`))
		default:
			t.Errorf("unexpected method %s", r.Method)
		}
	}))
	defer server.Close()
	r := &ManagedResource{definition: d, client: client.NewClient(server.URL, "key", server.Client())}
	if _, err := r.read(ctx, "8"); err != nil {
		t.Fatal(err)
	}
	prior := initialState(t, d)
	set(t, &prior, "msg_contains", types.StringValue("old"))
	plan := initialState(t, d)
	body, dd := payload(ctx, d, plan, prior, false)
	if dd.HasError() {
		t.Fatal(dd)
	}
	if err := r.mutate(ctx, "8", body); err != nil {
		t.Fatal(err)
	}
	if !patched {
		t.Fatal("syslog filter not cleared")
	}
}

func TestCanaryDuplicateSelectorsRejectedBeforeHTTP(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct{ name, field, values string }{
		{"canary_alert_rule", "service_types", `["ssh","ssh"]`},
		{"canary_alert_rule", "event_codes", `[1000,1000]`},
		{"canary_alert_rule", "src_cidrs", `["192.0.2.1","192.0.2.1"]`},
		{"canary_alert_rule", "canary_ids", `[2,2]`},
		{"canary_alert_rule", "location_ids", `[3,3]`},
		{"canary_service", "location_ids", `[3,3]`},
	} {
		t.Run(tc.name+"/"+tc.field, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.WriteHeader(422)
				_, _ = w.Write([]byte(`{"success":false,"message":"unexpected request"}`))
			}))
			defer server.Close()
			d := contract(t, tc.name)
			state := initialState(t, d)
			v, err := tfjson.FromRawJSON(json.RawMessage(tc.values))
			if err != nil {
				t.Fatal(err)
			}
			set(t, &state, tc.field, v)
			set(t, &state, "id", types.StringValue("9"))
			set(t, &state, "name", types.StringValue("duplicates"))
			r := &ManagedResource{definition: d, client: client.NewClient(server.URL, "key", server.Client())}
			var config resource.ValidateConfigResponse
			r.ValidateConfig(ctx, resource.ValidateConfigRequest{Config: tfsdk.Config{Schema: state.Schema, Raw: state.Raw}}, &config)
			if !config.Diagnostics.HasError() {
				t.Error("duplicate configuration was accepted")
			}
			var create resource.CreateResponse
			r.Create(ctx, resource.CreateRequest{Plan: tfsdk.Plan{Schema: state.Schema, Raw: state.Raw}}, &create)
			if !create.Diagnostics.HasError() {
				t.Error("duplicate apply was accepted")
			}
			var update resource.UpdateResponse
			r.Update(ctx, resource.UpdateRequest{Plan: tfsdk.Plan{Schema: state.Schema, Raw: state.Raw}, State: initialState(t, d)}, &update)
			if !update.Diagnostics.HasError() {
				t.Error("duplicate update was accepted")
			}
			if calls != 0 {
				t.Errorf("duplicates reached HTTP %d times", calls)
			}
		})
	}
}

func TestCanaryRefreshRejectsMalformedProjectionWithoutChangingState(t *testing.T) {
	ctx := context.Background()
	d := contract(t, "canary")
	state := initialState(t, d)
	set(t, &state, "id", types.StringValue("7"))
	set(t, &state, "description", types.StringValue("must preserve"))
	for _, raw := range []string{`null`, `{}`, `{"id":7}`, `{"id":7,"name":"decoy","location_id":2,"is_enabled":true,"license_reason":"expired"}`, `{"id":8,"name":"decoy","location_id":2,"is_enabled":true,"status":"offline","license_reason":"expired"}`, `{"id":7,"name":"decoy","location_id":2,"is_enabled":true,"status":"offline"}`} {
		before := state.Raw.String()
		if dd := refresh(ctx, d, &state, json.RawMessage(raw), false); !dd.HasError() {
			t.Errorf("accepted malformed projection %s", raw)
		}
		if state.Raw.String() != before {
			t.Errorf("invalid response changed state: %s", raw)
		}
	}
}

func TestCanaryMissingProtectedFieldsArePreservedOnlyForRecovery(t *testing.T) {
	ctx := context.Background()
	d := contract(t, "canary")
	state := initialState(t, d)
	set(t, &state, "id", types.StringValue("7"))
	set(t, &state, "description", types.StringValue("retained only in recovery"))
	recovery := json.RawMessage(`{"id":7,"name":"decoy","location_id":2,"is_enabled":true,"status":"offline","license_reason":"expired"}`)
	if dd := refresh(ctx, d, &state, recovery, false); dd.HasError() {
		t.Fatal(dd)
	}
	var desc types.String
	_ = state.GetAttribute(ctx, path.Root("description"), &desc)
	if desc.IsNull() {
		t.Fatal("recovery lost protected field")
	}
	full := json.RawMessage(`{"id":7,"name":"decoy","location_id":2,"is_enabled":true,"status":"offline","license_reason":null,"ip_ignorelist":[],"rate_limit_per_min":600,"health":{},"counters":{}}`)
	if dd := refresh(ctx, d, &state, full, false); dd.HasError() {
		t.Fatal(dd)
	}
	_ = state.GetAttribute(ctx, path.Root("description"), &desc)
	if !desc.IsNull() {
		t.Fatal("licensed response retained omitted field")
	}
}
