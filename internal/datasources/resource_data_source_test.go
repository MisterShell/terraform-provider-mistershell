package datasources

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-mistershell/internal/client"
)

func TestMapNetworkResourceCoordinatesToDataSourceModel(t *testing.T) {
	sourceID := int64(7)
	sourceName := "Paris"
	model := NetworkResourceDataSourceModel{}
	coordinates := &client.EffectiveCoordinates{
		Latitude:           48.8566,
		Longitude:          2.3522,
		SourceLocationID:   &sourceID,
		SourceLocationName: &sourceName,
	}

	mapNetworkResourceCoordinatesToDataSourceModel(coordinates, &model)

	if model.EffectiveLatitude.ValueFloat64() != 48.8566 || model.EffectiveLongitude.ValueFloat64() != 2.3522 {
		t.Fatalf("effective coordinates = %v, %v", model.EffectiveLatitude, model.EffectiveLongitude)
	}
	if model.CoordinateSourceID.ValueInt64() != 7 || model.CoordinateSourceName.ValueString() != "Paris" {
		t.Fatalf("coordinate source = %v, %v", model.CoordinateSourceID, model.CoordinateSourceName)
	}

	coordinates.SourceLocationID = nil
	coordinates.SourceLocationName = nil
	mapNetworkResourceCoordinatesToDataSourceModel(coordinates, &model)
	if model.EffectiveLatitude.IsNull() || model.EffectiveLongitude.IsNull() ||
		!model.CoordinateSourceID.IsNull() || !model.CoordinateSourceName.IsNull() {
		t.Fatalf("redacted coordinate source must preserve coordinates only: %#v", model)
	}

	mapNetworkResourceCoordinatesToDataSourceModel(nil, &model)
	if !model.EffectiveLatitude.IsNull() || !model.EffectiveLongitude.IsNull() ||
		!model.CoordinateSourceID.IsNull() || !model.CoordinateSourceName.IsNull() {
		t.Fatalf("missing effective coordinates must map to null: %#v", model)
	}
}

// The data source reads the flag straight from the API; a MisterShell <= 2.0.13
// response has no such field and must read back as false, not unknown.
func TestNetworkResourceDataSourceReadsSshgwAllowAPIKey(t *testing.T) {
	for _, tc := range []struct {
		name  string
		field string
		want  bool
	}{
		{"allowed", `"sshgw_allow_api_key":true,`, true},
		{"older backend omits the field", ``, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				switch req.Method + " " + req.URL.Path {
				case "GET /api/v1/resources/9":
					fmt.Fprintf(w, `{"success":true,"data":{"id":9,"name":"edge","resource_type":"generic_ssh","connector_id":"ssh","external_id":null,"connector_data":{"host":"192.0.2.10"},"credential_id":7,"location_id":3,"status":"verified","is_enabled":true,%s"extra_data":null,"created_at":"2026-09-18T10:00:00Z","updated_at":"2026-09-18T10:00:00Z"}}`, tc.field)
				case "GET /api/v1/resources/types":
					fmt.Fprint(w, `{"success":true,"data":[{"type":"generic_ssh","engine_id":"ssh","display_name":"Generic SSH","description":"","connector_schema":{},"credential_types":["username_password"]}]}`)
				case "GET /api/v1/resources/9/tags":
					fmt.Fprint(w, `{"success":true,"data":[]}`)
				default:
					t.Errorf("unexpected request: %s %s", req.Method, req.URL.Path)
					http.NotFound(w, req)
				}
			}))
			defer srv.Close()
			d := &NetworkResourceDataSource{client: client.NewClient(srv.URL, "test", srv.Client())}
			var schema datasource.SchemaResponse
			d.Schema(ctx, datasource.SchemaRequest{}, &schema)
			// tfsdk.Config has no setter; build the raw value through a Plan.
			plan := tfsdk.Plan{Schema: schema.Schema}
			if diags := plan.Set(ctx, &NetworkResourceDataSourceModel{
				ID: types.Int64Value(9), Tags: types.ListNull(tagObjectType),
			}); diags.HasError() {
				t.Fatal(diags)
			}
			config := tfsdk.Config{Schema: schema.Schema, Raw: plan.Raw}
			response := datasource.ReadResponse{State: tfsdk.State{Schema: schema.Schema}}
			d.Read(ctx, datasource.ReadRequest{Config: config}, &response)
			if response.Diagnostics.HasError() {
				t.Fatal(response.Diagnostics)
			}
			var state NetworkResourceDataSourceModel
			if diags := response.State.Get(ctx, &state); diags.HasError() {
				t.Fatal(diags)
			}
			if !state.SshgwAllowAPIKey.Equal(types.BoolValue(tc.want)) {
				t.Fatalf("sshgw_allow_api_key=%v, want %t", state.SshgwAllowAPIKey, tc.want)
			}
		})
	}
}
