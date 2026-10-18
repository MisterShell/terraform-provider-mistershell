package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"terraform-provider-mistershell/internal/client"
)

// Exercise Create's actual HTTP payload: losing resource_name breaks vault
// templates on the first connection, before a resource ID exists.
func TestNetworkResourceDiscoveryVaultContext(t *testing.T) {
	for _, tc := range []struct {
		name, credential string
		wantName         bool
		denied           bool
	}{
		{"external platform", `{"id":7,"source":"external","user_source":"local"}`, true, false},
		{"local platform", `{"id":7,"source":"local"}`, false, false},
		{"legacy backend", `{"id":7}`, false, false},
		{"external user only", `{"id":7,"source":"local","user_source":"external"}`, false, false},
		{"credential metadata forbidden still permits local discovery", `{}`, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			discoveryCalled := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				switch req.Method + " " + req.URL.Path {
				case "GET /api/v1/credentials/7":
					if tc.denied {
						w.WriteHeader(http.StatusForbidden)
						fmt.Fprint(w, `{"success":false,"message":"denied"}`)
						return
					}
					fmt.Fprintf(w, `{"success":true,"data":%s}`, tc.credential)
				case "POST /api/v1/resources/test/discovery":
					discoveryCalled = true
					var body map[string]any
					if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					name, present := body["resource_name"]
					if present != tc.wantName || (present && name != "vault-router") {
						t.Errorf("resource_name=%v present=%t; want present=%t", name, present, tc.wantName)
					}
					w.WriteHeader(http.StatusBadRequest)
					fmt.Fprint(w, `{"success":false,"message":"stop after observing discovery input"}`)
				default:
					t.Errorf("unexpected request %s %s", req.Method, req.URL.Path)
					http.NotFound(w, req)
				}
			}))
			defer server.Close()
			r := &NetworkResourceResource{client: client.NewClient(server.URL, "test", server.Client())}
			ctx := context.Background()
			var schema resource.SchemaResponse
			r.Schema(ctx, resource.SchemaRequest{}, &schema)
			plan := tfsdk.Plan{Schema: schema.Schema}
			diags := plan.Set(ctx, &NetworkResourceResourceModel{AcceptedTrustFields: types.SetNull(types.StringType),
				Name: types.StringValue("vault-router"), EngineID: types.StringValue("ssh"),
				LocationID: types.Int64Value(3), CredentialID: types.Int64Value(7),
				ConnectorData: testConnectorData("192.0.2.10"), IsEnabled: types.BoolValue(false),
				TagIDs: types.SetNull(types.Int64Type), Tags: types.ListNull(tagObjectType),
			})
			if diags.HasError() {
				t.Fatal(diags)
			}
			response := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
			r.Create(ctx, resource.CreateRequest{Plan: plan}, &response)
			if !response.Diagnostics.HasError() {
				t.Fatal("expected fixture diagnostic")
			}
			if !discoveryCalled {
				t.Errorf("discovery called=%t when denied=%t", discoveryCalled, tc.denied)
			}
		})
	}
}
