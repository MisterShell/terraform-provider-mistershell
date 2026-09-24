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

func TestNetworkResourceSchemaSshgwAllowPersonalKeyDefaultsToFalse(t *testing.T) {
	var resp resource.SchemaResponse
	(&NetworkResourceResource{}).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	attribute, ok := resp.Schema.Attributes["sshgw_allow_personal_key"].(resourceschema.BoolAttribute)
	if !ok || !attribute.Optional || !attribute.Computed || attribute.Required || attribute.Default == nil {
		t.Fatalf("sshgw_allow_personal_key schema = %#v", resp.Schema.Attributes["sshgw_allow_personal_key"])
	}
	var result defaults.BoolResponse
	attribute.Default.DefaultBool(context.Background(), defaults.BoolRequest{}, &result)
	if result.Diagnostics.HasError() || !result.PlanValue.Equal(types.BoolValue(false)) {
		t.Fatalf("sshgw_allow_personal_key default = %v (diags %v), want false", result.PlanValue, result.Diagnostics)
	}
}

// MisterShell < 2.0.15 rejects unknown create fields with HTTP 422, so the
// flag must only reach the initial POST when the user opts in. The mock answers
// like an older backend (no field in the response) unless the flag was sent,
// which also exercises the false read-back on those backends.
func TestNetworkResourceCreateSendsSshgwAllowPersonalKeyOnlyWhenOptedIn(t *testing.T) {
	for _, tc := range []struct {
		name     string
		planned  types.Bool
		wantSent bool
	}{
		{"opted in", types.BoolValue(true), true},
		{"explicit false", types.BoolValue(false), false},
		{"default", types.BoolNull(), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				switch req.Method + " " + req.URL.Path {
				case "POST /api/v1/resources/test/discovery":
					fmt.Fprint(w, `{"success":true,"data":{"task_id":46,"status":"queued","discovery_token":"opaque-discovery-token-at-least-32-bytes"}}`)
				case "GET /api/v1/worker-tasks/46":
					fmt.Fprint(w, `{"success":true,"data":{"id":46,"status":"success","result_success":true,"result":{"discovery":{"status":"matched","engine_id":"ssh","type_id":"generic_ssh"}}}}`)
				case "POST /api/v1/resources/test/discovery/proof-status":
					fmt.Fprint(w, `{"success":true,"data":{"state":"ready"}}`)
				case "POST /api/v1/resources":
					var body map[string]any
					if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					sent, present := body["sshgw_allow_personal_key"]
					if present != tc.wantSent || (present && sent != true) {
						t.Errorf("initial create sshgw_allow_personal_key = %#v (present=%t), want sent=%t", sent, present, tc.wantSent)
					}
					field := ""
					if present {
						field = `"sshgw_allow_personal_key":true,`
					}
					fmt.Fprintf(w, `{"success":true,"data":{"id":9,"name":"edge","resource_type":"generic_ssh","connector_id":"ssh","external_id":null,"connector_data":{"host":"192.0.2.10"},"credential_id":7,"location_id":3,"status":"verified","is_enabled":true,%s"extra_data":null,"created_at":"2026-09-18T10:00:00Z","updated_at":"2026-09-18T10:00:00Z"}}`, field)
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
			planned := tc.planned
			if planned.IsNull() {
				attribute := schema.Schema.Attributes["sshgw_allow_personal_key"].(resourceschema.BoolAttribute)
				var result defaults.BoolResponse
				attribute.Default.DefaultBool(ctx, defaults.BoolRequest{}, &result)
				if result.Diagnostics.HasError() {
					t.Fatal(result.Diagnostics)
				}
				planned = result.PlanValue
			}
			plan := tfsdk.Plan{Schema: schema.Schema}
			if diags := plan.Set(ctx, &NetworkResourceResourceModel{
				Name: types.StringValue("edge"), EngineID: types.StringValue("ssh"),
				LocationID: types.Int64Value(3), CredentialID: types.Int64Value(7),
				ConnectorData: testConnectorData("192.0.2.10"), IsEnabled: types.BoolValue(true),
				SshgwAllowPersonalKey: planned,
				TagIDs:                types.SetNull(types.Int64Type), Tags: types.ListNull(tagObjectType),
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
			if !state.SshgwAllowPersonalKey.Equal(types.BoolValue(tc.wantSent)) {
				t.Fatalf("created state sshgw_allow_personal_key=%v, want %t", state.SshgwAllowPersonalKey, tc.wantSent)
			}
		})
	}
}

func TestNetworkResourceUpdateSendsSshgwAllowPersonalKeyOnlyOnChange(t *testing.T) {
	state := NetworkResourceResourceModel{
		Name: types.StringValue("edge"), LocationID: types.Int64Value(3),
		CredentialID: types.Int64Value(7), ConnectorData: testConnectorData("192.0.2.10"),
		IsEnabled: types.BoolValue(true), SshgwAllowPersonalKey: types.BoolValue(true),
	}
	for _, tc := range []struct {
		name    string
		planned bool
		want    string
	}{
		{"unchanged", true, `{}`},
		{"revoked", false, `{"sshgw_allow_personal_key":false}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan := state
			plan.SshgwAllowPersonalKey = types.BoolValue(tc.planned)
			input, err := buildNetworkResourceUpdate(&plan, &state)
			if err != nil {
				t.Fatal(err)
			}
			body, err := json.Marshal(input)
			if err != nil {
				t.Fatal(err)
			}
			if string(body) != tc.want {
				t.Fatalf("update body=%s, want %s", body, tc.want)
			}
		})
	}
}

func TestNetworkResourceRefreshDetectsSshgwAllowPersonalKeyDrift(t *testing.T) {
	model := NetworkResourceResourceModel{SshgwAllowPersonalKey: types.BoolValue(true)}
	for _, allowed := range []bool{false, true} {
		if err := mapNetworkResourceResponseToModel(&client.NetworkResourceResponse{SshgwAllowPersonalKey: allowed}, &model); err != nil {
			t.Fatal(err)
		}
		if !model.SshgwAllowPersonalKey.Equal(types.BoolValue(allowed)) {
			t.Fatalf("refreshed sshgw_allow_personal_key=%v, want %v", model.SshgwAllowPersonalKey, allowed)
		}
	}
}
