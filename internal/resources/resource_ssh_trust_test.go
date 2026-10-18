package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-mistershell/internal/client"
)

// The backend owns discovered pins. This lifecycle verifies the provider sends
// the proof, preserves configured-only state, and never clears an omitted pin
// when replacing connector_data during an unrelated connection-setting update.
func TestNetworkResourceSSHTrustLifecycle(t *testing.T) {
	for _, strict := range []bool{true, false} {
		t.Run(fmt.Sprintf("strict_%t", strict), func(t *testing.T) {
			ctx := context.Background()
			const token = "opaque-discovery-token-at-least-32-bytes"
			const pin = "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
			const changedKey = "SHA256:BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBA"
			discoveries, creates, updates := 0, 0, 0
			serverConnector := map[string]any{}
			observedKey := pin
			writeResource := func(w http.ResponseWriter) {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"success": true, "message": "ok", "data": map[string]any{
						"id": 9, "name": "edge", "resource_type": "generic_ssh", "connector_id": "ssh",
						"credential_id": 7, "location_id": 3, "external_id": nil,
						"connector_data": serverConnector, "status": "verified", "is_enabled": true,
						"extra_data": nil, "created_at": "2026-09-08T00:00:00Z", "updated_at": "2026-09-08T00:00:00Z",
						"last_connectivity_result": map[string]any{"status": "verified", "device_info": map[string]any{"host_key_fingerprint": observedKey}},
					},
				})
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				switch req.Method + " " + req.URL.Path {
				case "GET /api/v1/credentials/7":
					fmt.Fprint(w, `{"success":true,"data":{"id":7}}`)
				case "POST /api/v1/resources/test/discovery":
					discoveries++
					var body map[string]any
					if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					connector := body["connector_data"].(map[string]any)
					if _, exists := connector["host_key_fingerprint"]; exists {
						t.Error("discovery unexpectedly requires a caller-supplied fingerprint")
					}
					if connector["strict_host_key"] != strict {
						t.Errorf("discovery strict_host_key=%v", connector["strict_host_key"])
					}
					fmt.Fprintf(w, `{"success":true,"data":{"task_id":46,"status":"queued","discovery_token":%q}}`, token)
				case "GET /api/v1/worker-tasks/46":
					fmt.Fprintf(w, `{"success":true,"data":{"id":46,"status":"success","result_success":true,"result":{"status":"verified","discovery":{"status":"matched","engine_id":"ssh","type_id":"generic_ssh"},"device_info":{"host_key_fingerprint":%q}}}}`, pin)
				case "POST /api/v1/resources/test/discovery/proof-status":
					fmt.Fprint(w, `{"success":true,"data":{"state":"ready"}}`)
				case "POST /api/v1/resources":
					creates++
					var body map[string]any
					if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					if body["discovery_token"] != token {
						t.Errorf("create did not carry discovery proof: %#v", body)
					}
					serverConnector = body["connector_data"].(map[string]any)
					if _, exists := serverConnector["host_key_fingerprint"]; exists {
						t.Error("create unexpectedly overrides proof-derived fingerprint")
					}
					// Model the API consuming the proof and enriching the saved resource.
					serverConnector["host_key_fingerprint"] = pin
					writeResource(w)
				case "PATCH /api/v1/resources/9":
					updates++
					var body map[string]any
					if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					connector := body["connector_data"].(map[string]any)
					if _, exists := connector["host_key_fingerprint"]; exists {
						t.Errorf("unrelated connector update explicitly altered the pin: %#v", connector)
					}
					if connector["port"] != float64(2222) || connector["strict_host_key"] != strict {
						t.Errorf("connector update=%#v", connector)
					}
					// The backend preserves a stored pin when that key is omitted, even
					// though the remaining connector object is replaced.
					connector["host_key_fingerprint"] = serverConnector["host_key_fingerprint"]
					serverConnector = connector
					if !strict {
						observedKey = changedKey
					}
					writeResource(w)
				case "GET /api/v1/resources/9":
					writeResource(w)
				case "GET /api/v1/resources/types":
					fmt.Fprint(w, `{"success":true,"data":[{"type":"generic_ssh","engine_id":"ssh","connector_schema":{}}]}`)
				case "GET /api/v1/resources/9/tags":
					fmt.Fprint(w, `{"success":true,"data":[]}`)
				default:
					t.Errorf("unexpected request %s %s", req.Method, req.URL.Path)
					http.NotFound(w, req)
				}
			}))
			defer srv.Close()
			r := &NetworkResourceResource{client: client.NewClient(srv.URL, "test", srv.Client())}
			var schema resource.SchemaResponse
			r.Schema(ctx, resource.SchemaRequest{}, &schema)
			connectorValue := func(port int64) types.Dynamic {
				return types.DynamicValue(types.ObjectValueMust(
					map[string]attr.Type{"host": types.StringType, "port": types.Int64Type, "strict_host_key": types.BoolType},
					map[string]attr.Value{"host": types.StringValue("192.0.2.10"), "port": types.Int64Value(port), "strict_host_key": types.BoolValue(strict)},
				))
			}
			model := NetworkResourceResourceModel{AcceptedTrustFields: types.SetNull(types.StringType),
				Name: types.StringValue("edge"), EngineID: types.StringValue("ssh"),
				CredentialID: types.Int64Value(7), LocationID: types.Int64Value(3), IsEnabled: types.BoolValue(true),
				ConnectorData: connectorValue(22), TagIDs: types.SetNull(types.Int64Type), Tags: types.ListNull(tagObjectType),
			}
			plan := tfsdk.Plan{Schema: schema.Schema}
			if diags := plan.Set(ctx, &model); diags.HasError() {
				t.Fatal(diags)
			}
			created := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
			r.Create(ctx, resource.CreateRequest{Plan: plan}, &created)
			if created.Diagnostics.HasError() {
				t.Fatal(created.Diagnostics)
			}
			if diags := created.State.Get(ctx, &model); diags.HasError() {
				t.Fatal(diags)
			}
			assertConnector := func(port int64) {
				t.Helper()
				raw, err := dynamicToRawJSON(model.ConnectorData)
				if err != nil {
					t.Fatal(err)
				}
				want := fmt.Sprintf(`{"host":"192.0.2.10","port":%d,"strict_host_key":%t}`, port, strict)
				if string(raw) != want {
					t.Fatalf("configured connector state = %s, want %s", raw, want)
				}
			}
			assertConnector(22)

			model.ConnectorData = connectorValue(2222)
			if diags := plan.Set(ctx, &model); diags.HasError() {
				t.Fatal(diags)
			}
			updated := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}}
			r.Update(ctx, resource.UpdateRequest{Plan: plan, State: created.State}, &updated)
			if updated.Diagnostics.HasError() {
				t.Fatal(updated.Diagnostics)
			}
			refreshed := resource.ReadResponse{State: updated.State}
			r.Read(ctx, resource.ReadRequest{State: updated.State}, &refreshed)
			if refreshed.Diagnostics.HasError() {
				t.Fatal(refreshed.Diagnostics)
			}
			if diags := refreshed.State.Get(ctx, &model); diags.HasError() {
				t.Fatal(diags)
			}
			assertConnector(2222)
			if model.Status.ValueString() != "verified" {
				t.Fatalf("status=%v", model.Status)
			}
			if !strict {
				raw, err := dynamicToRawJSON(model.LastConnectivityResult)
				if err != nil {
					t.Fatal(err)
				}
				var evidence map[string]any
				if err := json.Unmarshal(raw, &evidence); err != nil {
					t.Fatal(err)
				}
				if evidence["device_info"].(map[string]any)["host_key_fingerprint"] != changedKey {
					t.Fatalf("observed mismatch lost: %s", raw)
				}
			}
			if discoveries != 1 || creates != 1 || updates != 1 {
				t.Fatalf("discoveries=%d creates=%d updates=%d", discoveries, creates, updates)
			}
		})
	}
}
