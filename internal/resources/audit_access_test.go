package resources

import (
	"context"
	"encoding/json"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"net/http"
	"net/http/httptest"
	"strings"
	"terraform-provider-mistershell/internal/client"
	"testing"
)

func TestAuditAccessPasswordOptionalAndDestinationMutable(t *testing.T) {
	var s resource.SchemaResponse
	(&UserResource{}).Schema(context.Background(), resource.SchemaRequest{}, &s)
	if !s.Schema.Attributes["password"].IsOptional() {
		t.Error("password must permit imported users without password management")
	}
	(&LogDestinationResource{}).Schema(context.Background(), resource.SchemaRequest{}, &s)
	if len(s.Schema.Attributes["type"].(schema.StringAttribute).PlanModifiers) > 0 {
		t.Error("destination type supports in-place updates")
	}
}

func TestAuditAccessAuthConfigRemovals(t *testing.T) {
	prior, _ := rawJSONToDynamic([]byte(`{"client_id":"client","client_secret":"secret","mcp_audience":"aud","tls_verify":false,"scopes":["openid"],"additional_params":{"prompt":"login"}}`))
	next, _ := rawJSONToDynamic([]byte(`{"client_id":"client","client_secret":"secret"}`))
	raw, err := authProviderConfigPatch("OIDC", prior, next)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if v, exists := got["mcp_audience"]; !exists || v != nil {
		t.Errorf("removed audience must be explicitly null: %s", raw)
	}
	if got["tls_verify"] != true {
		t.Errorf("removed tls_verify must reset default: %s", raw)
	}
	if len(got["scopes"].([]any)) != 3 {
		t.Errorf("removed scopes must reset defaults: %s", raw)
	}
	if len(got["additional_params"].(map[string]any)) != 0 {
		t.Errorf("removed params must reset empty: %s", raw)
	}
	_, err = authProviderConfigPatch("OIDC", next, types.DynamicValue(types.ObjectValueMust(map[string]attr.Type{}, map[string]attr.Value{})))
	if err == nil {
		t.Error("removing required keys must not silently retain backend values")
	}
}

func TestAuditAccessCanonicalEmail(t *testing.T) {
	for _, email := range []string{"user@example.com", "User@example.com", "user@Example.com", " user@example.com "} {
		var resp validator.StringResponse
		canonicalEmailValidator{}.ValidateString(context.Background(), validator.StringRequest{Path: path.Root("email"), ConfigValue: types.StringValue(email)}, &resp)
		if resp.Diagnostics.HasError() != (email != "user@example.com") {
			t.Errorf("unexpected validation for %q: %v", email, resp.Diagnostics)
		}
	}
}

func TestAuditAccessLDAPConfigImport(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != "GET" || req.URL.Path != "/api/v1/auth-providers/7" {
			t.Errorf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
		_, _ = w.Write([]byte(`{"success":true,"data":{"id":7,"name":"ldap","mnemonic":"ldap","provider_type":"LDAP","is_enabled":false,"config":{"server_url":"ldaps://ldap.example.com","bind_password":"****","mfa_revision":"internal-value"}}}`))
	}))
	defer server.Close()
	r := &AuthProviderResource{client: client.NewClient(server.URL, "test-key", server.Client())}
	var s resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &s)
	state := tfsdk.State{Schema: s.Schema}
	if d := state.Set(ctx, &AuthProviderResourceModel{ID: types.Int64Value(7)}); d.HasError() {
		t.Fatal(d)
	}
	resp := resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	var got AuthProviderResourceModel
	if d := resp.State.Get(ctx, &got); d.HasError() {
		t.Fatal(d)
	}
	raw, err := dynamicToRawJSON(got.Config)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "mfa_revision") || strings.Contains(string(raw), "****") {
		t.Fatalf("import exposed server-only data: %s", raw)
	}
	if !strings.Contains(string(raw), "ldaps://ldap.example.com") {
		t.Fatalf("import lost readable config: %s", raw)
	}
}

func TestAuditAccessSAMLRefreshCertificateWhitespace(t *testing.T) {
	const certificate = "-----BEGIN CERTIFICATE-----\nQUJD\n-----END CERTIFICATE-----"
	priorRaw, _ := json.Marshal(map[string]any{"idp_x509_cert": certificate + "\n", "sp_x509_cert": "  " + certificate + "\n", "idp_sso_url": "https://original.example.com/sso", "sp_private_key": "configured-secret"})
	prior, err := rawJSONToDynamic(priorRaw)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, remoteCert, remoteURL string
		wantEqual                   bool
	}{
		{"API whitespace normalization", certificate, "https://original.example.com/sso", true},
		{"changed certificate", strings.ReplaceAll(certificate, "QUJD", "REVG"), "https://original.example.com/sso", false},
		{"changed endpoint", certificate, "https://changed.example.com/sso", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, _ := json.Marshal(map[string]any{"idp_x509_cert": tc.remoteCert, "sp_x509_cert": certificate, "idp_sso_url": tc.remoteURL, "sp_private_key": "****", "sign_requests": true})
			got, err := authProviderRefreshConfig("SAML", prior, raw)
			if err != nil {
				t.Fatal(err)
			}
			if got.Equal(prior) != tc.wantEqual {
				t.Fatalf("refresh equality=%t, want %t", got.Equal(prior), tc.wantEqual)
			}
			gotRaw, _ := dynamicToRawJSON(got)
			var attrs map[string]any
			if err = json.Unmarshal(gotRaw, &attrs); err != nil {
				t.Fatal(err)
			}
			if attrs["sp_private_key"] != "configured-secret" {
				t.Fatal("refresh lost configured secret")
			}
			if tc.remoteURL != "https://original.example.com/sso" && attrs["idp_sso_url"] != tc.remoteURL {
				t.Fatal("refresh lost endpoint drift")
			}
			if tc.remoteCert != certificate && attrs["idp_x509_cert"] != tc.remoteCert {
				t.Fatal("refresh lost certificate drift")
			}
		})
	}
}

func TestAuditAccessRolePermissionsAreManagedWhenOmitted(t *testing.T) {
	var s resource.SchemaResponse
	(&RoleResource{}).Schema(context.Background(), resource.SchemaRequest{}, &s)
	if s.Schema.Attributes["permissions"].IsComputed() {
		t.Error("omitted permissions must revoke grants rather than retaining computed state")
	}
	model := RoleResourceModel{Permissions: types.SetNull(types.StringType)}
	mapRoleResponseToModel(&client.RoleDetailResponse{}, &model)
	if !model.Permissions.IsNull() {
		t.Error("empty API permissions must preserve omitted configuration")
	}
}
