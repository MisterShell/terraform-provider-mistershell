package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/defaults"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-mistershell/internal/client"
)

// A false value must reach the initial POST: toggling it after creation allows
// automatic collection to be scheduled before Terraform disables it.
func TestNetworkResourceCreateSetsAutomaticCollectionAtomically(t *testing.T) {
	for _, tc := range []struct {
		name    string
		enabled types.Bool
		want    bool
	}{
		{"disabled", types.BoolValue(false), false},
		{"enabled", types.BoolValue(true), true},
		{"default", types.BoolNull(), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			creates, updates := 0, 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				switch req.Method + " " + req.URL.Path {
				case "GET /api/v1/credentials/7":
					fmt.Fprint(w, `{"success":true,"data":{"id":7}}`)
				case "POST /api/v1/resources/test/discovery":
					fmt.Fprint(w, `{"success":true,"data":{"task_id":46,"status":"queued","discovery_token":"opaque-discovery-token-at-least-32-bytes"}}`)
				case "GET /api/v1/worker-tasks/46":
					fmt.Fprint(w, `{"success":true,"data":{"id":46,"status":"success","result_success":true,"result":{"discovery":{"status":"matched","engine_id":"ssh","type_id":"generic_ssh"}}}}`)
				case "POST /api/v1/resources/test/discovery/proof-status":
					fmt.Fprint(w, `{"success":true,"data":{"state":"ready"}}`)
				case "POST /api/v1/resources":
					creates++
					var body map[string]any
					if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					if body["is_enabled"] != tc.want {
						t.Errorf("initial create is_enabled = %#v, want %v", body["is_enabled"], tc.want)
					}
					fmt.Fprintf(w, `{"success":true,"data":{"id":9,"name":"edge","resource_type":"generic_ssh","connector_id":"ssh","external_id":null,"connector_data":{"host":"192.0.2.10"},"credential_id":7,"location_id":3,"status":"verified","is_enabled":%t,"extra_data":null,"created_at":"2026-09-07T10:00:00Z","updated_at":"2026-09-07T10:00:00Z"}}`, tc.want)
				case "PATCH /api/v1/resources/9":
					updates++
					http.Error(w, "post-create toggle is not atomic", http.StatusBadRequest)
				case "GET /api/v1/resources/9/tags":
					fmt.Fprint(w, `{"success":true,"data":[]}`)
				default:
					t.Errorf("unexpected request: %s %s", req.Method, req.URL.Path)
					http.NotFound(w, req)
				}
			}))
			defer srv.Close()
			r := &NetworkResourceResource{client: client.NewClient(srv.URL, "test", srv.Client())}
			var schema resource.SchemaResponse
			r.Schema(ctx, resource.SchemaRequest{}, &schema)
			enabled := tc.enabled
			if enabled.IsNull() {
				attribute := schema.Schema.Attributes["is_enabled"].(resourceschema.BoolAttribute)
				if attribute.Default == nil {
					t.Fatal("automatic collection must have a default")
				}
				var result defaults.BoolResponse
				attribute.Default.DefaultBool(ctx, defaults.BoolRequest{}, &result)
				if result.Diagnostics.HasError() {
					t.Fatal(result.Diagnostics)
				}
				enabled = result.PlanValue
			}
			plan := tfsdk.Plan{Schema: schema.Schema}
			if diags := plan.Set(ctx, &NetworkResourceResourceModel{AcceptedTrustFields: types.SetNull(types.StringType),
				Name: types.StringValue("edge"), EngineID: types.StringValue("ssh"),
				LocationID: types.Int64Value(3), CredentialID: types.Int64Value(7),
				ConnectorData: testConnectorData("192.0.2.10"), IsEnabled: enabled,
				TagIDs: types.SetNull(types.Int64Type), Tags: types.ListNull(tagObjectType),
			}); diags.HasError() {
				t.Fatal(diags)
			}
			response := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
			r.Create(ctx, resource.CreateRequest{Plan: plan}, &response)
			if response.Diagnostics.HasError() {
				t.Fatal(response.Diagnostics)
			}
			var state NetworkResourceResourceModel
			if diags := response.State.Get(ctx, &state); diags.HasError() {
				t.Fatal(diags)
			}
			if state.ID.ValueInt64() != 9 || !state.IsEnabled.Equal(types.BoolValue(tc.want)) {
				t.Fatalf("created state: id=%v is_enabled=%v", state.ID, state.IsEnabled)
			}
			if creates != 1 || updates != 0 {
				t.Fatalf("creates=%d updates=%d, want one atomic create", creates, updates)
			}
		})
	}
}

func TestNetworkResourceParkAndRestoreUpdatesCollectionWithoutReconnect(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		wasEnabled, enabled      bool
		fromLocation, toLocation int64
	}{
		{"park", true, false, 3, 8},
		{"restore", false, true, 8, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := NetworkResourceResourceModel{AcceptedTrustFields: types.SetNull(types.StringType),
				Name: types.StringValue("edge"), LocationID: types.Int64Value(tc.fromLocation),
				CredentialID: types.Int64Value(7), ConnectorData: testConnectorData("192.0.2.10"),
				IsEnabled: types.BoolValue(tc.wasEnabled),
			}
			plan := state
			plan.LocationID = types.Int64Value(tc.toLocation)
			plan.IsEnabled = types.BoolValue(tc.enabled)
			input, err := buildNetworkResourceUpdate(&plan, &state)
			if err != nil {
				t.Fatal(err)
			}
			body, err := json.Marshal(input)
			if err != nil {
				t.Fatal(err)
			}
			want := fmt.Sprintf(`{"location_id":%d,"is_enabled":%t}`, tc.toLocation, tc.enabled)
			if string(body) != want {
				t.Fatalf("update body=%s, want %s", body, want)
			}
		})
	}
}

func TestNetworkResourceRefreshDetectsAutomaticCollectionDrift(t *testing.T) {
	model := NetworkResourceResourceModel{AcceptedTrustFields: types.SetNull(types.StringType), IsEnabled: types.BoolValue(true)}
	for _, enabled := range []bool{false, true} {
		if err := mapNetworkResourceResponseToModel(&client.NetworkResourceResponse{IsEnabled: enabled}, &model); err != nil {
			t.Fatal(err)
		}
		if !model.IsEnabled.Equal(types.BoolValue(enabled)) {
			t.Fatalf("refreshed is_enabled=%v, want %v", model.IsEnabled, enabled)
		}
	}
}
