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

// Catches reads calling a test/probe endpoint or leaking/unmasking vault config.
func TestExternalVaultDataSourceRead(t *testing.T) {
	ctx := context.Background()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("unexpected mutation %s", r.Method)
			w.WriteHeader(400)
			return
		}
		switch r.URL.Path {
		case "/api/v1/external-vaults":
			fmt.Fprint(w, `{"success":true,"data":[{"id":7,"name":"prod","mnemonic":"production"}]}`)
		case "/api/v1/external-vaults/7":
			fmt.Fprint(w, `{"success":true,"data":{"id":7,"name":"prod","mnemonic":"production","vault_type":"hashicorp","config":{"token":"****","tls_verify":true},"credential_count":2}}`)
		default:
			t.Errorf("unexpected endpoint %s", r.URL)
			w.WriteHeader(400)
		}
	}))
	defer srv.Close()
	d := &ExternalVaultDataSource{client: &client.Client{BaseURL: srv.URL, APIKey: "test", HTTPClient: srv.Client()}}
	var schema datasource.SchemaResponse
	d.Schema(ctx, datasource.SchemaRequest{}, &schema)
	for _, lookup := range []string{"id", "name", "mnemonic", "missing", "mismatch"} {
		t.Run(lookup, func(t *testing.T) {
			m := ExternalVaultDataSourceModel{}
			switch lookup {
			case "id":
				m.ID = types.Int64Value(7)
			case "name":
				m.Name = types.StringValue("prod")
			case "mnemonic":
				m.Mnemonic = types.StringValue("production")
			case "mismatch":
				m.ID = types.Int64Value(7)
				m.Name = types.StringValue("wrong")
			}
			state := tfsdk.State{Schema: schema.Schema}
			if diag := state.Set(ctx, &m); diag.HasError() {
				t.Fatal(diag)
			}
			response := datasource.ReadResponse{State: tfsdk.State{Schema: schema.Schema}}
			d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: schema.Schema, Raw: state.Raw}}, &response)
			if lookup == "missing" || lookup == "mismatch" {
				if !response.Diagnostics.HasError() {
					t.Fatal("missing lookup diagnostic")
				}
				return
			}
			if response.Diagnostics.HasError() {
				t.Fatal(response.Diagnostics)
			}
			var got ExternalVaultDataSourceModel
			response.State.Get(ctx, &got)
			if got.ID.ValueInt64() != 7 || got.CredentialCount.ValueInt64() != 2 {
				t.Fatalf("%+v", got)
			}
			expected, err := rawJSONToDynamic([]byte(`{"token":"****","tls_verify":true}`))
			if err != nil || !got.Config.Equal(expected) {
				t.Fatalf("config=%v %v", got.Config, err)
			}
		})
	}
}
