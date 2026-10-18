package provider_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func testAccVaultConfig(name, kind, config string) string {
	return fmt.Sprintf(`
resource "mistershell_external_vault" "test" {
 name = %q
 mnemonic = %q
 vault_type = %q
 config = %s
}
data "mistershell_external_vault" "by_id" { id = mistershell_external_vault.test.id }
data "mistershell_external_vault" "by_name" { name = mistershell_external_vault.test.name }
data "mistershell_external_vault" "by_mnemonic" { mnemonic = mistershell_external_vault.test.mnemonic }
`, name, name, kind, config)
}

func TestAccExternalVault_types(t *testing.T) {
	testAccPreCheck(t)
	for _, tc := range []struct{ kind, config, secret string }{
		{"hashicorp", `{address="https://vault.example.invalid/",auth_method="token",token="tfacc-test-token",namespace="test"}`, "token"},
		{"openbao", `{address="https://bao.example.invalid/",auth_method="approle",approle_role_id="role",approle_secret_id="tfacc-test-secret"}`, "approle_secret_id"},
		{"aws_secrets_manager", `{region="eu-west-1",auth_method="access_key",access_key_id="test-key",secret_access_key="tfacc-test-secret",session_token="tfacc-test-session"}`, "secret_access_key"},
		{"azure_key_vault", `{vault_url="https://tfacc.vault.azure.net/",auth_method="client_secret",tenant_id="test-tenant",client_id="test-client",client_secret="tfacc-test-secret"}`, "client_secret"},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			name := acctestPrefix + strings.ReplaceAll(tc.kind, "_", "-")
			base := testAccVaultConfig(name, tc.kind, tc.config)
			updated := strings.Replace(base, " vault_type =", " description = \"Updated vault\"\n cache_ttl_seconds = 0\n vault_type =", 1)
			// Removing keys in HCL must reset remote config, not leave old values hidden.
			if tc.kind == "hashicorp" {
				updated = strings.Replace(updated, `,namespace="test"`, "", 1)
			}
			if tc.kind == "aws_secrets_manager" {
				updated = strings.Replace(updated, `,session_token="tfacc-test-session"`, "", 1)
			}
			checks := []resource.TestCheckFunc{
				resource.TestCheckResourceAttr("mistershell_external_vault.test", "vault_type", tc.kind),
				resource.TestCheckResourceAttr("data.mistershell_external_vault.by_id", "config."+tc.secret, "****"),
				resource.TestCheckResourceAttrPair("data.mistershell_external_vault.by_name", "id", "mistershell_external_vault.test", "id"),
				resource.TestCheckResourceAttrPair("data.mistershell_external_vault.by_mnemonic", "id", "mistershell_external_vault.test", "id"),
			}
			verifyReset := func(s *terraform.State) error {
				id, _ := strconv.ParseInt(s.RootModule().Resources["mistershell_external_vault.test"].Primary.ID, 10, 64)
				vault, err := testAccClient().GetExternalVault(context.Background(), id)
				if err != nil {
					return err
				}
				var config map[string]any
				if err = json.Unmarshal(vault.Config, &config); err != nil {
					return err
				}
				if tc.kind == "hashicorp" && config["namespace"] != "" {
					return fmt.Errorf("removed namespace remained stored")
				}
				if tc.kind == "aws_secrets_manager" && config["session_token"] != nil {
					return fmt.Errorf("removed session token remained stored")
				}
				return nil
			}
			nextConfig := map[string]string{
				"hashicorp":           `{address="https://vault.example.invalid/",auth_method="approle",approle_role_id="role",approle_secret_id="new-secret"}`,
				"openbao":             `{address="https://bao.example.invalid/",auth_method="token",token="new-token"}`,
				"aws_secrets_manager": `{region="eu-west-1",auth_method="ambient"}`,
				"azure_key_vault":     `{vault_url="https://tfacc.vault.azure.net/",auth_method="ambient"}`,
			}[tc.kind]
			switched := testAccVaultConfig(name, tc.kind, nextConfig)
			verifyPruned := func(s *terraform.State) error {
				id, _ := strconv.ParseInt(s.RootModule().Resources["mistershell_external_vault.test"].Primary.ID, 10, 64)
				vault, err := testAccClient().GetExternalVault(context.Background(), id)
				if err != nil {
					return err
				}
				var config map[string]any
				if err = json.Unmarshal(vault.Config, &config); err != nil {
					return err
				}
				if _, exists := config[tc.secret]; exists {
					return fmt.Errorf("auth switch retained obsolete %s", tc.secret)
				}
				return nil
			}
			resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, CheckDestroy: testAccCheckAllDestroyed, Steps: []resource.TestStep{
				{Config: base, Check: resource.ComposeAggregateTestCheckFunc(checks...)},
				{Config: base, PlanOnly: true, ExpectNonEmptyPlan: false},
				{ResourceName: "mistershell_external_vault.test", ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: []string{"config"}},
				{Config: updated, Check: resource.ComposeAggregateTestCheckFunc(verifyReset, resource.TestCheckResourceAttr("mistershell_external_vault.test", "cache_ttl_seconds", "0"))},
				{Config: updated, PlanOnly: true, ExpectNonEmptyPlan: false},
				{Config: switched, Check: verifyPruned},
				{Config: switched, PlanOnly: true, ExpectNonEmptyPlan: false},
			}})
		})
	}
}

func testAccCredentialSourcesConfig(name, platform, user string, mixed bool) string {
	config := testAccVaultConfig(name, "hashicorp", `{address="https://vault.example.invalid/",auth_method="token",token="tfacc-test-token"}`)
	platformFields := `credential_data = {username="literal",password="tfacc-test-password"}`
	if platform == "external" {
		literals := ""
		fields := `username="username",password="password"`
		if mixed {
			literals = `credential_data={username="literal"}`
			fields = `password="password"`
		}
		platformFields = fmt.Sprintf(`external_vault_id=mistershell_external_vault.test.id
external_vault_ref={base={path="servers/{{ resource.name }}",version=1},fields={%s}}
%s`, fields, literals)
	}
	userFields := ""
	if user == "external" {
		userFields = `user_external_vault_id=mistershell_external_vault.test.id
user_external_vault_ref={base={path="users/{{ user.email }}"},fields={username="username",password="password"}}`
	}
	return config + fmt.Sprintf(`
resource "mistershell_credential" "test" {
 name=%q
 credential_type="username_password"
 source=%q
 user_source=%q
 requires_user_mapping=true
 %s
 %s
}
data "mistershell_credential" "by_id" { id=mistershell_credential.test.id }
data "mistershell_credential" "by_name" { name=mistershell_credential.test.name }
`, name, platform, user, platformFields, userFields)
}

func TestAccCredential_externalSources(t *testing.T) {
	testAccPreCheck(t)
	name := acctestPrefix + "credential-sources"
	var originalID string
	steps := []resource.TestStep{}
	for _, tc := range []struct {
		platform, user string
		mixed          bool
	}{
		{"local", "local", false}, {"local", "external", false}, {"external", "external", true},
		{"external", "external", false}, {"external", "local", false}, {"local", "local", false},
	} {
		tc := tc
		config := testAccCredentialSourcesConfig(name, tc.platform, tc.user, tc.mixed)
		check := func(s *terraform.State) error {
			rs := s.RootModule().Resources["mistershell_credential.test"]
			if originalID == "" {
				originalID = rs.Primary.ID
			} else if originalID != rs.Primary.ID {
				return fmt.Errorf("source change replaced credential")
			}
			id, _ := strconv.ParseInt(rs.Primary.ID, 10, 64)
			cred, err := testAccClient().GetCredential(context.Background(), id)
			if err != nil {
				return err
			}
			if cred.Source != tc.platform || cred.UserSource != tc.user {
				return fmt.Errorf("unexpected source selections: %s/%s", cred.Source, cred.UserSource)
			}
			if tc.platform == "external" && !tc.mixed && string(cred.CredentialData) != "{}" {
				return fmt.Errorf("all-mapped credentials retained literals")
			}
			if tc.platform == "external" || tc.user == "external" {
				vid, _ := strconv.ParseInt(s.RootModule().Resources["mistershell_external_vault.test"].Primary.ID, 10, 64)
				err = testAccClient().DeleteExternalVault(context.Background(), vid)
				if err == nil || !strings.Contains(err.Error(), "409") {
					return fmt.Errorf("expected referenced-vault conflict, got %v", err)
				}
			}
			return nil
		}
		steps = append(steps, resource.TestStep{Config: config, Check: resource.ComposeAggregateTestCheckFunc(check,
			resource.TestCheckResourceAttr("data.mistershell_credential.by_id", "source", tc.platform),
			resource.TestCheckResourceAttr("data.mistershell_credential.by_name", "user_source", tc.user),
		)}, resource.TestStep{Config: config, PlanOnly: true, ExpectNonEmptyPlan: false})
		if tc.platform == "external" && tc.user == "external" && !tc.mixed {
			steps = append(steps, resource.TestStep{ResourceName: "mistershell_credential.test", ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: []string{"credential_data", "user_credential_data"}})
		}
	}
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, CheckDestroy: testAccCheckAllDestroyed, Steps: steps})
}

type testVaultFixture struct {
	URL        string `json:"url"`
	BackendURL string `json:"backend_url"`
	Token      string `json:"token"`
	RoleID     string `json:"role_id"`
	SecretID   string `json:"secret_id"`
}

// A real vault and a real SSH server prove that the credential references are
// resolved by Core using the planned resource name, without secrets in state.
func TestAccExternalVault_resourceOnboarding(t *testing.T) {
	testAccPreCheck(t)
	file := os.Getenv("MISTERSHELL_ACC_VAULT_FIXTURES")
	if file == "" {
		t.Skip("MISTERSHELL_ACC_VAULT_FIXTURES must point to isolated HashiCorp/OpenBao fixture metadata")
	}
	contents, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var fixtures map[string]testVaultFixture
	if err = json.Unmarshal(contents, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"hashicorp", "openbao"} {
		t.Run(kind, func(t *testing.T) {
			fixture, ok := fixtures[kind]
			if !ok {
				t.Fatalf("missing %s fixture", kind)
			}
			host, port, username, password := startGenericSSHFixture(t)
			name := acctestPrefix + kind + "-vault-router"
			payload, _ := json.Marshal(map[string]any{"data": map[string]string{"username": username, "password": password}})
			request, err := http.NewRequest(http.MethodPost, fixture.URL+"/v1/secret/data/"+name, bytes.NewReader(payload))
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("X-Vault-Token", fixture.Token)
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if response.StatusCode != 200 {
				t.Fatalf("seeding vault: HTTP %d", response.StatusCode)
			}
			t.Cleanup(func() {
				req, _ := http.NewRequest(http.MethodDelete, fixture.URL+"/v1/secret/metadata/"+name, nil)
				req.Header.Set("X-Vault-Token", fixture.Token)
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Error(err)
					return
				}
				resp.Body.Close()
				if resp.StatusCode != 204 {
					t.Errorf("removing vault secret: HTTP %d", resp.StatusCode)
				}
			})
			auth := fmt.Sprintf(`auth_method="token",token=%q`, fixture.Token)
			if kind == "openbao" {
				auth = fmt.Sprintf(`auth_method="approle",approle_role_id=%q,approle_secret_id=%q`, fixture.RoleID, fixture.SecretID)
			}
			config := testAccVaultConfig(name, kind, fmt.Sprintf(`{address=%q,%s}`, fixture.BackendURL, auth)) + fmt.Sprintf(`
resource "mistershell_credential" "test" {
 name=%q
 credential_type="username_password"
 source="external"
 external_vault_id=mistershell_external_vault.test.id
 external_vault_ref={base={path="{{ resource.name }}",version=1},fields={username="username",password="password"}}
}
resource "mistershell_resource" "test" {
 name=%q
 engine_id="ssh"
 location_id=1
 credential_id=mistershell_credential.test.id
 is_enabled=false
 connector_data={host=%q,port=%d,strict_host_key=false}
}
`, name, name, host, port)
			resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, CheckDestroy: testAccCheckAllDestroyed, Steps: []resource.TestStep{
				{Config: config, Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_resource.test", "resource_type", "generic_ssh"),
					resource.TestCheckNoResourceAttr("mistershell_credential.test", "credential_data.password"),
				)},
				{Config: config, PlanOnly: true, ExpectNonEmptyPlan: false},
			}})
		})
	}
}

// The reference schema differs across vault types; notably Azure's base is an
// empty object and AWS selectors are not HashiCorp paths.
func TestAccExternalVault_referenceContracts(t *testing.T) {
	testAccPreCheck(t)
	for _, tc := range []struct{ kind, config, ref string }{
		{"hashicorp", `{address="https://vault.example.invalid",auth_method="token",token="test-token"}`, `{base={path="servers/{{ resource.name }}",version=1},fields={username="username",password="password"}}`},
		{"openbao", `{address="https://vault.example.invalid",auth_method="token",token="test-token"}`, `{base={path="servers/{{ resource.name }}"},fields={username="username",password="password"}}`},
		{"aws_secrets_manager", `{region="eu-west-1",auth_method="ambient"}`, `{base={secret_id="servers/{{ resource.name }}",version_stage="AWSCURRENT"},fields={username="username",password="password"}}`},
		{"azure_key_vault", `{vault_url="https://tfacc.vault.azure.net",auth_method="ambient"}`, `{base={},fields={username="server-username",password="server-password"}}`},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			name := acctestPrefix + "ref-" + strings.ReplaceAll(tc.kind, "_", "-")
			config := testAccVaultConfig(name, tc.kind, tc.config) + fmt.Sprintf(`
resource "mistershell_credential" "test" {
 name=%q
 credential_type="username_password"
 source="external"
 external_vault_id=mistershell_external_vault.test.id
 external_vault_ref=%s
 user_source="external"
 user_external_vault_id=mistershell_external_vault.test.id
 user_external_vault_ref=%s
 requires_user_mapping=true
}
data "mistershell_credential" "test" { id=mistershell_credential.test.id }
`, name, tc.ref, tc.ref)
			resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, CheckDestroy: testAccCheckAllDestroyed, Steps: []resource.TestStep{
				{Config: config, Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_credential.test", "external_vault_type", tc.kind),
					resource.TestCheckResourceAttr("data.mistershell_credential.test", "user_external_vault_type", tc.kind),
				)},
				{Config: config, PlanOnly: true, ExpectNonEmptyPlan: false},
			}})
		})
	}
}
