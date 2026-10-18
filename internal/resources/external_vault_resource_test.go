package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"terraform-provider-mistershell/internal/client"
)

func externalVaultDynamic(t *testing.T, raw string) types.Dynamic {
	t.Helper()
	v, e := rawJSONToDynamic(json.RawMessage(raw))
	if e != nil {
		t.Fatal(e)
	}
	return v
}

// Catches backend overlay retaining removed HCL keys and stale authentication secrets.
func TestExternalVaultConfigReset(t *testing.T) {
	catalog := client.ExternalVaultType{VaultType: "hashicorp", ConfigSchema: json.RawMessage(`{"properties":{"namespace":{"type":"string","default":""},"tls_verify":{"type":"boolean","default":true},"session_token":{"type":["string","null"]},"token":{"type":"string"},"address":{"type":"string"}}}`)}
	for _, tc := range []struct{ name, old, new, want, err string }{
		{"defaults", `{"namespace":"team","tls_verify":false}`, `{}`, `{"namespace":"","tls_verify":true}`, ""},
		{"nullable", `{"session_token":"secret"}`, `{}`, `{"session_token":null}`, ""},
		{"unsafe removal", `{"address":"https://vault"}`, `{}`, "", "address"},
		{"auth switch", `{"auth_method":"token","token":"secret"}`, `{"auth_method":"approle","approle_role_id":"role","approle_secret_id":"new"}`, `{"auth_method":"approle","approle_role_id":"role","approle_secret_id":"new"}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := externalVaultConfigPatch(externalVaultDynamic(t, tc.old), externalVaultDynamic(t, tc.new), catalog)
			if tc.err != "" {
				if err == nil || !strings.Contains(err.Error(), tc.err) {
					t.Fatalf("%s %v", got, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var a, b any
			_ = json.Unmarshal(got, &a)
			_ = json.Unmarshal([]byte(tc.want), &b)
			aa, _ := json.Marshal(a)
			bb, _ := json.Marshal(b)
			if string(aa) != string(bb) {
				t.Fatalf("got %s want %s", aa, bb)
			}
		})
	}
}

// Catches replacing configured secrets with masks, losing drift, or adding server defaults to config.
func TestExternalVaultRefreshConfig(t *testing.T) {
	prior := externalVaultDynamic(t, `{"token":"secret","namespace":"old","tls_verify":true,"kv_version":2}`)
	got, err := externalVaultRefreshConfig(context.Background(), prior, json.RawMessage(`{"token":"****","namespace":"new","tls_verify":false,"kv_version":1,"kv_mount":"secret"}`), []string{"token"})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := dynamicToRawJSON(got)
	if string(raw) != `{"kv_version":1,"namespace":"new","tls_verify":false,"token":"secret"}` {
		t.Fatalf("wrong refresh: %s", raw)
	}
	imported, err := externalVaultRefreshConfig(context.Background(), types.DynamicNull(), json.RawMessage(`{"token":"****"}`), []string{"token"})
	if err != nil || imported.IsNull() {
		t.Fatalf("import: %v %v", imported, err)
	}
}

func TestExternalVaultValidateConfig(t *testing.T) {
	ctx := context.Background()
	r := &ExternalVaultResource{}
	var s resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &s)
	cases := []struct {
		name string
		v    types.Dynamic
		bad  bool
	}{
		{"object", externalVaultDynamic(t, `{"tls_verify":false,"session_token":null}`), false},
		{"scalar", types.DynamicValue(types.StringValue("bad")), true},
		{"unknown", types.DynamicUnknown(), false},
		{"nested unknown", types.DynamicValue(types.ObjectValueMust(map[string]attr.Type{"token": types.StringType}, map[string]attr.Value{"token": types.StringUnknown()})), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state := tfsdk.State{Schema: s.Schema}
			m := externalVaultTestModel(t)
			m.Config = tc.v
			if d := state.Set(ctx, &m); d.HasError() {
				t.Fatal(d)
			}
			var resp resource.ValidateConfigResponse
			r.ValidateConfig(ctx, resource.ValidateConfigRequest{Config: tfsdk.Config{Schema: s.Schema, Raw: state.Raw}}, &resp)
			if resp.Diagnostics.HasError() != tc.bad {
				t.Fatalf("%v", resp.Diagnostics)
			}
		})
	}
}

func externalVaultTestModel(t *testing.T) ExternalVaultResourceModel {
	return ExternalVaultResourceModel{Name: types.StringValue("prod"), Mnemonic: types.StringValue("prod"), VaultType: types.StringValue("hashicorp"), Description: types.StringValue(""), IsEnabled: types.BoolValue(true), CacheTTLSeconds: types.Int64Value(60), Config: externalVaultDynamic(t, `{"address":"https://vault","auth_method":"token","token":"secret","namespace":"team"}`)}
}

// Exercises resource plan -> HTTP -> state, update removal reset, refresh and 404/409 handling.
func TestExternalVaultResourceLifecycle(t *testing.T) {
	ctx := context.Background()
	deleted := false
	var patch map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/external-vaults/types" {
			fmt.Fprint(w, `{"success":true,"data":[{"vault_type":"hashicorp","config_schema":{"properties":{"namespace":{"default":""}}},"secret_config_keys":["token"]}]}`)
			return
		}
		if r.Method == "DELETE" {
			if !deleted {
				w.WriteHeader(409)
				fmt.Fprint(w, `{"success":false,"message":"referenced by 1 credentials"}`)
			} else {
				w.WriteHeader(404)
			}
			return
		}
		if deleted {
			w.WriteHeader(404)
			return
		}
		if r.Method == "PATCH" {
			_ = json.NewDecoder(r.Body).Decode(&patch)
		}
		fmt.Fprint(w, `{"success":true,"data":{"id":7,"name":"prod","mnemonic":"prod","vault_type":"hashicorp","description":"","is_enabled":true,"cache_ttl_seconds":60,"config":{"address":"https://vault","auth_method":"token","token":"****","namespace":""},"credential_count":0,"created_at":"2026-10-18","updated_at":"2026-10-18"}}`)
	}))
	defer srv.Close()
	r := &ExternalVaultResource{client: &client.Client{BaseURL: srv.URL, APIKey: "test", HTTPClient: srv.Client()}}
	var s resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &s)
	model := externalVaultTestModel(t)
	plan := tfsdk.Plan{Schema: s.Schema}
	if d := plan.Set(ctx, &model); d.HasError() {
		t.Fatal(d)
	}
	create := resource.CreateResponse{State: tfsdk.State{Schema: s.Schema}}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, &create)
	if create.Diagnostics.HasError() {
		t.Fatal(create.Diagnostics)
	}
	var got ExternalVaultResourceModel
	create.State.Get(ctx, &got)
	if got.ID.ValueInt64() != 7 || !got.Config.Equal(model.Config) {
		t.Fatalf("lost planned configuration: %+v", got)
	}
	model = got
	model.Config = externalVaultDynamic(t, `{"address":"https://vault","auth_method":"token","token":"secret"}`)
	plan.Set(ctx, &model)
	update := resource.UpdateResponse{State: create.State}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: create.State}, &update)
	if update.Diagnostics.HasError() {
		t.Fatal(update.Diagnostics)
	}
	if len(patch) != 1 || patch["config"].(map[string]any)["namespace"] != "" {
		t.Fatalf("wrong patch: %v", patch)
	}
	del := resource.DeleteResponse{}
	r.Delete(ctx, resource.DeleteRequest{State: update.State}, &del)
	if !del.Diagnostics.HasError() {
		t.Fatal("deletion guard hidden")
	}
	deleted = true
	del = resource.DeleteResponse{}
	r.Delete(ctx, resource.DeleteRequest{State: update.State}, &del)
	if del.Diagnostics.HasError() {
		t.Fatal(del.Diagnostics)
	}
	read := resource.ReadResponse{State: update.State}
	r.Read(ctx, resource.ReadRequest{State: update.State}, &read)
	if read.Diagnostics.HasError() || !read.State.Raw.IsNull() {
		t.Fatalf("404 refresh: %v %v", read.State.Raw, read.Diagnostics)
	}
}

// Removing the optional JWT selector resets it to off and prunes its dependent role,
// regardless of Go map iteration order.
func TestExternalVaultResetIdentityMode(t *testing.T) {
	catalog := client.ExternalVaultType{VaultType: "hashicorp", ConfigSchema: json.RawMessage(`{"properties":{"user_identity_mode":{"default":"off"},"user_identity_role":{"type":"string"}}}`)}
	for i := 0; i < 30; i++ {
		raw, err := externalVaultConfigPatch(externalVaultDynamic(t, `{"user_identity_mode":"jwt","user_identity_role":"engineers"}`), externalVaultDynamic(t, `{}`), catalog)
		if err != nil || string(raw) != `{"user_identity_mode":"off"}` {
			t.Fatalf("got %s %v", raw, err)
		}
	}
}

func TestExternalVaultRefreshNonSecretMask(t *testing.T) {
	got, err := externalVaultRefreshConfig(context.Background(), externalVaultDynamic(t, `{"namespace":"old","token":"secret"}`), json.RawMessage(`{"namespace":"****","token":"****"}`), []string{"token"})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := dynamicToRawJSON(got)
	if string(raw) != `{"namespace":"****","token":"secret"}` {
		t.Fatalf("nonsecret drift lost: %s", raw)
	}
}

func TestExternalVaultImport(t *testing.T) {
	ctx := context.Background()
	r := &ExternalVaultResource{}
	var s resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &s)
	for _, id := range []string{"7", "0", "-1", "abc"} {
		t.Run(id, func(t *testing.T) {
			resp := resource.ImportStateResponse{State: tfsdk.State{Schema: s.Schema}}
			if d := resp.State.Set(ctx, &ExternalVaultResourceModel{}); d.HasError() {
				t.Fatal(d)
			}
			r.ImportState(ctx, resource.ImportStateRequest{ID: id}, &resp)
			if resp.Diagnostics.HasError() != (id != "7") {
				t.Fatal(resp.Diagnostics)
			}
			if id == "7" {
				var got types.Int64
				resp.State.GetAttribute(ctx, path.Root("id"), &got)
				if got.ValueInt64() != 7 {
					t.Fatal(got)
				}
			}
		})
	}
}

// The API removes trailing origin slashes and Azure authority prefixes. Refresh
// must preserve an equivalent configured spelling without hiding actual drift.
func TestExternalVaultRefreshCanonicalURLs(t *testing.T) {
	for _, tc := range []struct{ name, key, prior, remote, want string }{
		{"hashicorp trailing slash", "address", "https://vault.example/", "https://vault.example", "https://vault.example/"},
		{"openbao port", "address", "http://bao.example:8200/", "http://bao.example:8200", "http://bao.example:8200/"},
		{"AWS endpoint", "endpoint_url", "https://secrets.example/", "https://secrets.example", "https://secrets.example/"},
		{"Azure URL", "vault_url", "https://test.vault.azure.net/", "https://test.vault.azure.net", "https://test.vault.azure.net/"},
		{"Azure authority", "authority_host", "https://login.microsoftonline.com/", "login.microsoftonline.com", "https://login.microsoftonline.com/"},
		{"Azure authority slash", "authority_host", "login.microsoftonline.com/", "login.microsoftonline.com", "login.microsoftonline.com/"},
		{"real host drift", "address", "https://old.example/", "https://new.example", "https://new.example"},
		{"real scheme drift", "endpoint_url", "https://vault.example/", "http://vault.example", "http://vault.example"},
		{"real port drift", "address", "https://vault.example:8200/", "https://vault.example:8201", "https://vault.example:8201"},
		{"invalid path not normalized", "address", "https://vault.example//", "https://vault.example", "https://vault.example"},
		{"non URL field not normalized", "namespace", "team/", "team", "team"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prior, _ := json.Marshal(map[string]string{tc.key: tc.prior})
			remote, _ := json.Marshal(map[string]string{tc.key: tc.remote})
			got, err := externalVaultRefreshConfig(context.Background(), externalVaultDynamic(t, string(prior)), remote, nil)
			if err != nil {
				t.Fatal(err)
			}
			raw, _ := dynamicToRawJSON(got)
			var values map[string]string
			_ = json.Unmarshal(raw, &values)
			if values[tc.key] != tc.want {
				t.Fatalf("got %s want %s", values[tc.key], tc.want)
			}
		})
	}
}

func TestExternalVaultImportMasksBecomeNull(t *testing.T) {
	imported, err := externalVaultRefreshConfig(context.Background(), types.DynamicNull(), json.RawMessage(`{"token":"****","namespace":"****"}`), []string{"token"})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := dynamicToRawJSON(imported)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"namespace":"****","token":null}` {
		t.Fatalf("import should hydrate readable fields and null the masked secret: %s", raw)
	}
}
func TestExternalVaultUnknownImportedSecretIsNotClearedByConfigEdit(t *testing.T) {
	catalog := client.ExternalVaultType{VaultType: "hashicorp", SecretConfigKeys: []string{"token"}, ConfigSchema: json.RawMessage(`{"properties":{"token":{"type":"string"},"namespace":{"type":"string","default":""}}}`)}
	for _, planned := range []string{`{"token":null,"namespace":"new"}`, `{"namespace":"new"}`} {
		patch, err := externalVaultConfigPatch(externalVaultDynamic(t, `{"token":null,"namespace":"old"}`), externalVaultDynamic(t, planned), catalog)
		if err != nil {
			t.Fatal(err)
		}
		if string(patch) != `{"namespace":"new"}` {
			t.Fatalf("imported unknown token must be omitted to preserve backend secret: %s", patch)
		}
	}
}
