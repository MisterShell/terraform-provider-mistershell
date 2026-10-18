package datasources

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"net/http"
	"net/http/httptest"
	"terraform-provider-mistershell/internal/client"
	"terraform-provider-mistershell/internal/credentialrefs"
	"terraform-provider-mistershell/internal/tfjson"
	"testing"
)

func TestCredentialDataSourceReadsBothSourcesWithoutResolving(t *testing.T) {
	for _, byName := range []bool{false, true} {
		t.Run(fmt.Sprint("byName=", byName), func(t *testing.T) {
			requests := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.Method != "GET" {
					t.Errorf("unexpected mutation: %s", r.Method)
				}
				switch r.URL.Path {
				case "/api/v1/credentials":
					fmt.Fprint(w, `{"success":true,"data":[{"id":7,"name":"test","credential_type":"username_password"}],"meta":{"total":1,"page":1,"size":10000,"pages":1}}`)
				case "/api/v1/credentials/7":
					fmt.Fprint(w, `{"success":true,"data":{"id":7,"name":"test","credential_type":"username_password","requires_user_mapping":true,"source":"external","user_source":"external","external_vault_id":3,"user_external_vault_id":4,"external_vault_name":"platform","external_vault_type":"hashicorp","user_external_vault_name":"people","user_external_vault_type":"azure_key_vault","external_vault_ref":{"base":{"path":"servers/{{ resource.name }}","version":2},"fields":{"password":"password"}},"user_external_vault_ref":{"base":{},"fields":{"password":"{{ user.email }}"}},"credential_data":{"username":"platform"},"user_credential_data":{"username":"****"},"created_at":"today","updated_at":"today"}}`)
				default:
					t.Errorf("unexpected endpoint (must never resolve secrets): %s", r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()
			d := &CredentialDataSource{client: client.NewClient(srv.URL, "test", srv.Client())}
			ctx := context.Background()
			var s datasource.SchemaResponse
			d.Schema(ctx, datasource.SchemaRequest{}, &s)
			if diags := s.Schema.ValidateImplementation(ctx); diags.HasError() {
				t.Fatal(diags)
			}
			model := CredentialDataSourceModel{ID: types.Int64Value(7), ExternalVaultRef: types.ObjectNull(credentialrefs.AttributeTypes()), UserExternalVaultRef: types.ObjectNull(credentialrefs.AttributeTypes())}
			if byName {
				model.ID = types.Int64Null()
				model.Name = types.StringValue("test")
			}
			configState := tfsdk.State{Schema: s.Schema}
			if diags := configState.Set(ctx, &model); diags.HasError() {
				t.Fatal(diags)
			}
			resp := datasource.ReadResponse{State: tfsdk.State{Schema: s.Schema}}
			d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: s.Schema, Raw: configState.Raw}}, &resp)
			if resp.Diagnostics.HasError() {
				t.Fatal(resp.Diagnostics)
			}
			var got CredentialDataSourceModel
			if diags := resp.State.Get(ctx, &got); diags.HasError() {
				t.Fatal(diags)
			}
			if got.Source.ValueString() != "external" || got.UserSource.ValueString() != "external" || got.UserExternalVaultID.ValueInt64() != 4 || got.ExternalVaultType.ValueString() != "hashicorp" {
				t.Fatal("both sources and vault metadata must survive detail lookup")
			}
			raw, err := tfjson.ToRawJSON(got.UserCredentialData)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]string
			json.Unmarshal(raw, &fields)
			if fields["username"] != "****" {
				t.Fatal("data source must expose masked user literals")
			}
			ref, err := credentialrefs.ToJSON(got.ExternalVaultRef)
			if err != nil {
				t.Fatal(err)
			}
			var decoded struct{ Base map[string]any }
			json.Unmarshal(ref, &decoded)
			if decoded.Base["version"] != float64(2) || decoded.Base["path"] != "servers/{{ resource.name }}" {
				t.Fatal("reference numbers and Jinja must be retained")
			}
			want := 1
			if byName {
				want = 2
			}
			if requests != want {
				t.Fatalf("requests=%d, expected detail read with optional list only", requests)
			}
		})
	}
}
