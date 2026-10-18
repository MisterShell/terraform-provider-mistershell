package provider_test

import (
	"bytes"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// This acceptance check uses the backend's real signing key without changing
// its TLS certificate. Vault verifies actual user assertions against the public
// keys obtained from the application's JWKS endpoint; no private key is read.
func TestAccExternalVault_userIdentity(t *testing.T) {
	testAccPreCheck(t)
	file := os.Getenv("MISTERSHELL_ACC_VAULT_FIXTURES")
	if file == "" {
		t.Skip("MISTERSHELL_ACC_VAULT_FIXTURES must point to isolated HashiCorp/OpenBao fixtures")
	}
	contents, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var fixtures map[string]testVaultFixture
	if err = json.Unmarshal(contents, &fixtures); err != nil {
		t.Fatal(err)
	}
	testAccConfigureAppBaseURL(t, "OIDC")
	c := testAccClient()
	jwksRaw, err := testAccIdentityRequest(c.HTTPClient, http.MethodGet, c.BaseURL+"/.well-known/jwks.json", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	keys, err := testAccIdentityPublicKeys(jwksRaw)
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"hashicorp", "openbao"} {
		t.Run(kind, func(t *testing.T) {
			fixture, ok := fixtures[kind]
			if !ok {
				t.Fatalf("missing %s fixture", kind)
			}
			name := acctestPrefix + kind + "-identity"
			mount := name
			policy := name
			vaultRequest := func(method, path string, payload any) ([]byte, error) {
				return testAccIdentityRequest(http.DefaultClient, method, fixture.URL+"/v1/"+path, "X-Vault-Token", fixture.Token, payload)
			}
			mustVault := func(method, path string, payload any) []byte {
				t.Helper()
				raw, e := vaultRequest(method, path, payload)
				if e != nil {
					t.Fatal(e)
				}
				return raw
			}
			// A separate auth mount avoids altering the fixture's service/JWT setup.
			mustVault(http.MethodPost, "sys/auth/"+mount, map[string]any{"type": "jwt"})
			t.Cleanup(func() {
				if _, err := vaultRequest(http.MethodDelete, "sys/auth/"+mount, nil); err != nil {
					t.Error(err)
				}
			})
			mustVault(http.MethodPost, "auth/"+mount+"/config", map[string]any{"jwt_validation_pubkeys": keys, "bound_issuer": c.BaseURL})
			var auth struct {
				Data map[string]struct {
					Accessor string `json:"accessor"`
				} `json:"data"`
			}
			if err := json.Unmarshal(mustVault(http.MethodGet, "sys/auth", nil), &auth); err != nil {
				t.Fatal(err)
			}
			accessor := auth.Data[mount+"/"].Accessor
			if accessor == "" {
				t.Fatal("new JWT mount has no accessor")
			}
			subject := "admin@mistershell.local"
			ownPath := name + "/users/" + subject + "/router"
			deniedPath := name + "/users/another-user@example.invalid/router"
			policyText := fmt.Sprintf(`path "secret/data/%s/users/{{identity.entity.aliases.%s.name}}/*" { capabilities = ["read"] }`, name, accessor)
			mustVault(http.MethodPost, "sys/policies/acl/"+policy, map[string]any{"policy": policyText})
			t.Cleanup(func() {
				if _, err := vaultRequest(http.MethodDelete, "sys/policies/acl/"+policy, nil); err != nil {
					t.Error(err)
				}
			})
			mustVault(http.MethodPost, "auth/"+mount+"/role/users", map[string]any{"role_type": "jwt", "user_claim": "email", "bound_audiences": []string{"mistershell-vault:" + name}, "token_policies": []string{policy}, "token_ttl": "60s", "token_max_ttl": "60s"})
			secretMarker := "NEVER_RETURN_IDENTITY_SECRET_" + name
			for _, path := range []string{ownPath, deniedPath} {
				path := path
				mustVault(http.MethodPost, "secret/data/"+path, map[string]any{"data": map[string]string{"username": subject, "password": secretMarker}})
				t.Cleanup(func() {
					if _, err := vaultRequest(http.MethodDelete, "secret/metadata/"+path, nil); err != nil {
						t.Error(err)
					}
				})
			}
			authConfig := fmt.Sprintf(`auth_method="token",token=%q`, fixture.Token)
			if kind == "openbao" {
				authConfig = fmt.Sprintf(`auth_method="approle",approle_role_id=%q,approle_secret_id=%q`, fixture.RoleID, fixture.SecretID)
			}
			config := testAccVaultConfig(name, kind, fmt.Sprintf(`{address=%q,%s,user_identity_mode="jwt",user_identity_mount_path=%q,user_identity_role="users"}`, fixture.BackendURL, authConfig, mount)) + fmt.Sprintf(`
resource "mistershell_credential" "test" {
 name=%q
 credential_type="username_password"
 source="local"
 credential_data={username="platform",password="tfacc-platform-secret"}
 requires_user_mapping=true
 user_source="external"
 user_external_vault_id=mistershell_external_vault.test.id
 user_external_vault_ref={base={path=%q},fields={username="username",password="password"}}
}
`, name, name+"/users/{{ user.email }}/router")
			check := func(s *terraform.State) error {
				vaultID := s.RootModule().Resources["mistershell_external_vault.test"].Primary.ID
				credentialID, err := strconv.ParseInt(s.RootModule().Resources["mistershell_credential.test"].Primary.ID, 10, 64)
				if err != nil {
					return err
				}
				for _, tc := range []struct{ path, read, classification string }{{name + "/users/{{ user.email }}/router", "found", ""}, {deniedPath, "denied", "vault_permission_denied"}} {
					payload := map[string]any{"audience": "user", "subject_email": subject, "credential_id": credentialID, "credential_type": "username_password", "base": map[string]any{"path": tc.path}, "fields": map[string]string{"username": "username", "password": "password"}}
					raw, err := testAccIdentityRequest(c.HTTPClient, http.MethodPost, c.BaseURL+"/api/v1/external-vaults/"+vaultID+"/probe", "Authorization", "Bearer "+c.APIKey, payload)
					if err != nil {
						return err
					}
					for _, secret := range []string{secretMarker, fixture.Token, fixture.SecretID} {
						if secret != "" && bytes.Contains(raw, []byte(secret)) {
							return fmt.Errorf("identity probe exposed a secret value")
						}
					}
					var result struct {
						Data struct {
							Identity       string `json:"identity"`
							Login          string `json:"login"`
							Read           string `json:"read"`
							Classification string `json:"classification"`
							Rendered       struct {
								Base map[string]any `json:"base"`
							} `json:"rendered"`
							FieldsPresent []string `json:"fields_present"`
						} `json:"data"`
					}
					if err := json.Unmarshal(raw, &result); err != nil {
						return err
					}
					if result.Data.Identity != "subject" || result.Data.Login != "ok" || result.Data.Read != tc.read || result.Data.Classification != tc.classification {
						return fmt.Errorf("user probe returned identity=%s login=%s read=%s classification=%s", result.Data.Identity, result.Data.Login, result.Data.Read, result.Data.Classification)
					}
					wantPath := deniedPath
					if tc.read == "found" {
						wantPath = ownPath
						if len(result.Data.FieldsPresent) != 2 {
							return fmt.Errorf("user probe did not find both credential fields")
						}
					}
					if result.Data.Rendered.Base["path"] != wantPath {
						return fmt.Errorf("user.email reference was not rendered to the expected path")
					}
				}
				return nil
			}
			resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, CheckDestroy: testAccCheckAllDestroyed, Steps: []resource.TestStep{
				{Config: config, Check: resource.ComposeAggregateTestCheckFunc(check, resource.TestCheckNoResourceAttr("mistershell_credential.test", "user_credential_data.password"))},
				{Config: config, PlanOnly: true, ExpectNonEmptyPlan: false},
			}})
		})
	}
}

// Do not include response bodies in failures: login/probe fixtures can carry
// sensitive values even when a server returns an unexpected status.
func testAccIdentityRequest(c *http.Client, method, url, header, value string, payload any) ([]byte, error) {
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	if header != "" {
		req.Header.Set(header, value)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s %s: HTTP %d", method, req.URL.Path, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1<<20))
}

func testAccIdentityPublicKeys(raw []byte) ([]string, error) {
	var jwks struct {
		Keys []struct{ Kty, N, E string } `json:"keys"`
	}
	if err := json.Unmarshal(raw, &jwks); err != nil {
		return nil, err
	}
	var keys []string
	for _, key := range jwks.Keys {
		if key.Kty != "RSA" {
			continue
		}
		n, err := base64.RawURLEncoding.DecodeString(key.N)
		if err != nil {
			return nil, err
		}
		e, err := base64.RawURLEncoding.DecodeString(key.E)
		if err != nil {
			return nil, err
		}
		exponent := new(big.Int).SetBytes(e)
		if !exponent.IsInt64() || exponent.Int64() < 3 || exponent.Int64() > 1<<31-1 || len(n) == 0 {
			return nil, fmt.Errorf("JWKS contained an invalid RSA public key")
		}
		der, err := x509.MarshalPKIXPublicKey(&rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(exponent.Int64())})
		if err != nil {
			return nil, err
		}
		keys = append(keys, strings.TrimSpace(string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))))
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("backend JWKS contained no RSA public signing key")
	}
	return keys, nil
}
