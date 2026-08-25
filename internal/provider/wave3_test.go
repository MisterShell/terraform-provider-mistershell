package provider_test

import (
	"context"
	crand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"terraform-provider-mistershell/internal/client"
)

// Wave 3 acceptance tests: session-policy ACLs/rules and auth providers/mappings.
//
// Two environmental hazards (api-bug-register #11) are handled here:
//
//  1. Auth-provider/mapping create+update REQUIRE A LICENSE. The auth-provider
//     and mapping tests probe licensing at runtime (authProviderLicensed) and
//     t.Skip with an explicit reason when the instance is unlicensed — an
//     EXPLICIT, reported skip, not a silent drop.
//  2. The LAST session-policy rule cannot be deleted. The rule test creates its
//     own rules and relies on the instance already having other (seeded) rules,
//     so destroying the test rules never removes the final rule. The test asserts
//     (via authProviderLicensed-style probe) that >=1 non-test rule exists before
//     running so CheckDestroy never hits the last-rule 403.
//
// All object names are prefixed with acctestPrefix so reruns / parallel runs do
// not collide and the sweepers can target them. Each resource case uses
// CheckDestroy.

// authProviderLicensed reports whether the MisterShell instance has an active
// license, which is required for auth-provider/mapping create+update (register
// #11). It queries GET /api/v1/licenses/ with the Bearer token from the env. A
// non-empty license list is treated as licensed. Used to gate (skip, with an
// explicit reason) the license-bound tests.
func authProviderLicensed(t *testing.T) bool {
	t.Helper()
	url := os.Getenv("MISTERSHELL_URL")
	apiKey := os.Getenv("MISTERSHELL_API_KEY")
	if url == "" || apiKey == "" {
		return false
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url+"/api/v1/licenses/", nil)
	if err != nil {
		t.Fatalf("authProviderLicensed: building request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("authProviderLicensed: querying licenses: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	var body struct {
		Data []struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("authProviderLicensed: decoding licenses: %v", err)
	}
	return len(body.Data) > 0
}

// existingRuleCount returns the current number of session-policy rules on the
// instance. Used by the rule test to confirm seeded rules exist so deleting the
// test rules never removes the last rule (register #11).
func existingRuleCount(t *testing.T) int {
	t.Helper()
	c := testAccClient()
	if c == nil {
		t.Fatal("existingRuleCount: MISTERSHELL_URL/MISTERSHELL_API_KEY not set")
	}
	rules, err := c.ListRules(context.Background())
	if err != nil {
		t.Fatalf("existingRuleCount: %v", err)
	}
	return len(rules)
}

// ---------------------------------------------------------------------------
// mistershell_session_policy_acl — create, update, data source, import
// ---------------------------------------------------------------------------

func TestAccSessionPolicyAcl_basic(t *testing.T) {
	testAccPreCheck(t)

	name := acctestPrefix + "acl"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			// Create: two ordered patterns (glob + regex), enabled=true (default).
			{
				Config: testAccSessionPolicyAclConfig(name, "first acl", "true",
					`[{ pattern = "rm -rf *", type = "glob" }, { pattern = "^sudo", type = "regex" }]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_session_policy_acl.test", "name", name),
					resource.TestCheckResourceAttr("mistershell_session_policy_acl.test", "description", "first acl"),
					resource.TestCheckResourceAttr("mistershell_session_policy_acl.test", "enabled", "true"),
					resource.TestCheckResourceAttr("mistershell_session_policy_acl.test", "is_builtin", "false"),
					resource.TestCheckResourceAttr("mistershell_session_policy_acl.test", "patterns.#", "2"),
					// Order is significant (List): element 0 is the glob, element 1 the regex.
					resource.TestCheckResourceAttr("mistershell_session_policy_acl.test", "patterns.0.pattern", "rm -rf *"),
					resource.TestCheckResourceAttr("mistershell_session_policy_acl.test", "patterns.0.type", "glob"),
					resource.TestCheckResourceAttr("mistershell_session_policy_acl.test", "patterns.1.pattern", "^sudo"),
					resource.TestCheckResourceAttr("mistershell_session_policy_acl.test", "patterns.1.type", "regex"),
					resource.TestCheckResourceAttrSet("mistershell_session_policy_acl.test", "id"),
					resource.TestCheckResourceAttrSet("mistershell_session_policy_acl.test", "created_at"),
				),
			},
			// Re-apply identical config: plan must be empty.
			{
				Config: testAccSessionPolicyAclConfig(name, "first acl", "true",
					`[{ pattern = "rm -rf *", type = "glob" }, { pattern = "^sudo", type = "regex" }]`),
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
			// Update: change description, disable, and replace patterns (single glob).
			{
				Config: testAccSessionPolicyAclConfig(name, "updated acl", "false",
					`[{ pattern = "shutdown*", type = "glob" }]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_session_policy_acl.test", "description", "updated acl"),
					resource.TestCheckResourceAttr("mistershell_session_policy_acl.test", "enabled", "false"),
					resource.TestCheckResourceAttr("mistershell_session_policy_acl.test", "patterns.#", "1"),
					resource.TestCheckResourceAttr("mistershell_session_policy_acl.test", "patterns.0.pattern", "shutdown*"),
					resource.TestCheckResourceAttr("mistershell_session_policy_acl.test", "patterns.0.type", "glob"),
				),
			},
			// Import by integer id (Read populates via the list endpoint).
			{
				ResourceName:      "mistershell_session_policy_acl.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccSessionPolicyAclDataSource_byIDAndName(t *testing.T) {
	testAccPreCheck(t)

	name := acctestPrefix + "acl-ds"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccSessionPolicyAclDataSourceConfig(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					// by id
					resource.TestCheckResourceAttrPair("data.mistershell_session_policy_acl.by_id", "id", "mistershell_session_policy_acl.test", "id"),
					resource.TestCheckResourceAttr("data.mistershell_session_policy_acl.by_id", "name", name),
					resource.TestCheckResourceAttr("data.mistershell_session_policy_acl.by_id", "is_builtin", "false"),
					resource.TestCheckResourceAttr("data.mistershell_session_policy_acl.by_id", "patterns.#", "1"),
					resource.TestCheckResourceAttr("data.mistershell_session_policy_acl.by_id", "patterns.0.pattern", "cat /etc/shadow"),
					// by name
					resource.TestCheckResourceAttrPair("data.mistershell_session_policy_acl.by_name", "id", "mistershell_session_policy_acl.test", "id"),
					resource.TestCheckResourceAttr("data.mistershell_session_policy_acl.by_name", "patterns.0.type", "glob"),
				),
			},
		},
	})
}

// ---------------------------------------------------------------------------
// mistershell_session_policy_rule — create with explicit position, update,
// data source, import. Never deletes the last rule (the instance has seeded
// rules; this test only adds/removes its own).
// ---------------------------------------------------------------------------

func TestAccSessionPolicyRule_basic(t *testing.T) {
	testAccPreCheck(t)

	// Guard the last-rule hazard (register #11): require at least one existing
	// (seeded/other) rule so destroying the test rule never empties the chain.
	if existingRuleCount(t) < 1 {
		t.Skip("session-policy rule test requires at least one pre-existing rule so destroy never removes the last rule; instance has none")
	}

	name := acctestPrefix + "rule"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			// Create with an explicit position (12345 — distinctive, high to avoid
			// colliding with seeded rules), action=deny, session_types=[shell].
			{
				Config: testAccSessionPolicyRuleConfig(name, 12345, "deny", `["shell"]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_session_policy_rule.test", "name", name),
					resource.TestCheckResourceAttr("mistershell_session_policy_rule.test", "position", "12345"),
					resource.TestCheckResourceAttr("mistershell_session_policy_rule.test", "action", "deny"),
					resource.TestCheckResourceAttr("mistershell_session_policy_rule.test", "session_types.#", "1"),
					resource.TestCheckTypeSetElemAttr("mistershell_session_policy_rule.test", "session_types.*", "shell"),
					resource.TestCheckResourceAttr("mistershell_session_policy_rule.test", "enabled", "true"),
					resource.TestCheckResourceAttr("mistershell_session_policy_rule.test", "notify", "false"),
					resource.TestCheckResourceAttrSet("mistershell_session_policy_rule.test", "id"),
					resource.TestCheckResourceAttrSet("mistershell_session_policy_rule.test", "created_at"),
				),
			},
			// Re-apply identical config: plan must be empty (proves position is
			// declarative with no reorder churn).
			{
				Config:             testAccSessionPolicyRuleConfig(name, 12345, "deny", `["shell"]`),
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
			// Update: flip action to accept and change position (declarative reorder
			// via a plain position edit — no reorder endpoint).
			{
				Config: testAccSessionPolicyRuleConfig(name, 12350, "accept", `["shell", "graphical"]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_session_policy_rule.test", "position", "12350"),
					resource.TestCheckResourceAttr("mistershell_session_policy_rule.test", "action", "accept"),
					resource.TestCheckResourceAttr("mistershell_session_policy_rule.test", "session_types.#", "2"),
				),
			},
			// Import by integer id (Read populates via the list endpoint).
			{
				ResourceName:      "mistershell_session_policy_rule.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccSessionPolicyRuleDataSource_byID(t *testing.T) {
	testAccPreCheck(t)

	if existingRuleCount(t) < 1 {
		t.Skip("session-policy rule data-source test requires at least one pre-existing rule so destroy never removes the last rule; instance has none")
	}

	name := acctestPrefix + "rule-ds"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccSessionPolicyRuleDataSourceConfig(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.mistershell_session_policy_rule.by_id", "id", "mistershell_session_policy_rule.test", "id"),
					resource.TestCheckResourceAttr("data.mistershell_session_policy_rule.by_id", "name", name),
					resource.TestCheckResourceAttr("data.mistershell_session_policy_rule.by_id", "position", "23456"),
					resource.TestCheckResourceAttr("data.mistershell_session_policy_rule.by_id", "action", "deny"),
				),
			},
		},
	})
}

// ---------------------------------------------------------------------------
// mistershell_auth_provider / mistershell_auth_provider_mapping (license-gated).
//
// Every auth-provider surface below (resource CRUD, data source, group
// mappings) is driven from testAccAuthProviderFixtures, which is keyed by
// provider_type and cross-checked against the GENERATED
// client.SupportedAuthProviderTypes. A provider_type added backend-side
// therefore fails these tests until it has a fixture, so coverage cannot
// silently lag the API — an OIDC-only suite is precisely how the removal of the
// SAML config.sp_entity_id/sp_acs_url inputs went unnoticed while every test
// still passed.
// ---------------------------------------------------------------------------

// authProviderFixture builds valid HCL for one provider_type.
type authProviderFixture struct {
	// config returns the `config = jsonencode({...})` body. The altered form
	// changes one NON-SECRET field so update steps exercise a real PATCH.
	config func(altered bool) string
	// extraChecks are assertions meaningful only for this provider_type; addr
	// is the resource/data-source address to assert against.
	extraChecks func(addr string) []resource.TestCheckFunc
}

// authProviderSPURLChecks asserts the derived SP URLs are surfaced for SAML.
// They are computed backend-side from app_base_url + the provider id.
func authProviderSPURLChecks(addr string) []resource.TestCheckFunc {
	return []resource.TestCheckFunc{
		resource.TestMatchResourceAttr(addr, "sp_entity_id", regexp.MustCompile(`/auth/saml/\d+/metadata$`)),
		resource.TestMatchResourceAttr(addr, "sp_acs_url", regexp.MustCompile(`/auth/saml/\d+/acs$`)),
	}
}

// authProviderNoSPURLChecks asserts the SP URL attributes stay null for the
// non-SAML types (the API omits them).
func authProviderNoSPURLChecks(addr string) []resource.TestCheckFunc {
	return []resource.TestCheckFunc{
		resource.TestCheckNoResourceAttr(addr, "sp_entity_id"),
		resource.TestCheckNoResourceAttr(addr, "sp_acs_url"),
	}
}

// testAccAuthProviderFixtures returns one fixture per supported provider_type.
// It fails loudly when client.SupportedAuthProviderTypes gains a value with no
// fixture, mirroring the generator's fail-loudly stance on enum drift.
func testAccAuthProviderFixtures(t *testing.T) map[string]authProviderFixture {
	t.Helper()

	// Generated per run: idp_x509_cert is validated server-side as a real PEM
	// certificate, and a per-run cert can never fail on expiry.
	cert := testAccSelfSignedCertPEM(t)

	fixtures := map[string]authProviderFixture{
		"LDAP": {
			config: func(altered bool) string {
				serverURL, tlsVerify := "ldaps://ldap.example.com:636", "false"
				if altered {
					// tls_verify defaults to false for LDAP, so flipping it on
					// is the meaningful edit.
					serverURL, tlsVerify = "ldaps://ldap2.example.com:636", "true"
				}
				return fmt.Sprintf(`jsonencode({
    server_url    = %q
    bind_dn       = "cn=svc,dc=example,dc=com"
    bind_password = "tfacc-bind-password"
    base_dn       = "ou=people,dc=example,dc=com"
    use_tls       = true
    tls_verify    = %s
  })`, serverURL, tlsVerify)
			},
			extraChecks: authProviderNoSPURLChecks,
		},
		"OIDC": {
			config: func(altered bool) string {
				issuerURL := "https://idp.example.com"
				if altered {
					issuerURL = "https://idp2.example.com"
				}
				return fmt.Sprintf(`jsonencode({
    issuer_url    = %q
    client_id     = "tfacc-client-id"
    client_secret = "tfacc-client-secret"
    tls_verify    = true
  })`, issuerURL)
			},
			extraChecks: authProviderNoSPURLChecks,
		},
		"SAML": {
			config: func(altered bool) string {
				ssoURL := "https://idp.example.com/sso"
				if altered {
					ssoURL = "https://idp2.example.com/sso"
				}
				return fmt.Sprintf(`jsonencode({
    idp_entity_id = "https://idp.example.com/metadata"
    idp_sso_url   = %q
    idp_x509_cert = %q
  })`, ssoURL, cert)
			},
			extraChecks: authProviderSPURLChecks,
		},
	}

	for _, providerType := range client.SupportedAuthProviderTypes {
		if _, ok := fixtures[providerType]; !ok {
			t.Fatalf("no acceptance-test fixture for auth provider type %q: every value in "+
				"client.SupportedAuthProviderTypes must be covered here", providerType)
		}
	}
	return fixtures
}

// TestAccAuthProvider_basic exercises create/round-trip/update/import for EVERY
// supported provider_type, including the secret field each type carries and the
// display_order round trip.
func TestAccAuthProvider_basic(t *testing.T) {
	testAccPreCheck(t)
	if !authProviderLicensed(t) {
		t.Skip("auth providers require a MisterShell license; instance is unlicensed")
	}

	fixtures := testAccAuthProviderFixtures(t)
	const addr = "mistershell_auth_provider.test"

	for _, providerType := range client.SupportedAuthProviderTypes {
		fx := fixtures[providerType]
		t.Run(providerType, func(t *testing.T) {
			name := acctestPrefix + strings.ToLower(providerType)

			createChecks := []resource.TestCheckFunc{
				resource.TestCheckResourceAttr(addr, "name", name),
				resource.TestCheckResourceAttr(addr, "provider_type", providerType),
				resource.TestCheckResourceAttr(addr, "is_enabled", "true"),
				// display_order is honored on create.
				resource.TestCheckResourceAttr(addr, "display_order", "7"),
				resource.TestCheckResourceAttr(addr, "group_mappings_count", "0"),
				resource.TestCheckResourceAttrSet(addr, "config"),
				resource.TestCheckResourceAttrSet(addr, "id"),
				resource.TestCheckResourceAttrSet(addr, "created_at"),
			}
			createChecks = append(createChecks, fx.extraChecks(addr)...)

			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				CheckDestroy:             testAccCheckAllDestroyed,
				Steps: []resource.TestStep{
					{
						Config: testAccAuthProviderConfig(name, providerType, true, 7, fx.config(false)),
						Check:  resource.ComposeAggregateTestCheckFunc(createChecks...),
					},
					// Re-apply identical config: the plan must be empty. Proves
					// config survives BOTH the API's secret masking and its
					// server-enriched defaults (the syslog lesson).
					{
						Config:             testAccAuthProviderConfig(name, providerType, true, 7, fx.config(false)),
						PlanOnly:           true,
						ExpectNonEmptyPlan: false,
					},
					// Update non-secret fields: is_enabled, display_order, and a
					// type-specific config edit.
					{
						Config: testAccAuthProviderConfig(name, providerType, false, 9, fx.config(true)),
						Check: resource.ComposeAggregateTestCheckFunc(
							resource.TestCheckResourceAttr(addr, "is_enabled", "false"),
							resource.TestCheckResourceAttr(addr, "display_order", "9"),
						),
					},
					// config carries API-masked secrets and enriched defaults, so
					// it cannot be verified on import — ignore it.
					{
						ResourceName:            addr,
						ImportState:             true,
						ImportStateVerify:       true,
						ImportStateVerifyIgnore: []string{"config"},
					},
				},
			})
		})
	}
}

// A SAML config carrying a removed sp_* key must fail the apply with the
// backend's 422 rather than silently no-op'ing. SAMLConfig sets extra="forbid";
// LDAP/OIDC ignore unknown keys, so this is SAML-specific by construction.
func TestAccAuthProvider_samlRejectsRemovedSPFields(t *testing.T) {
	testAccPreCheck(t)
	if !authProviderLicensed(t) {
		t.Skip("auth providers require a MisterShell license; instance is unlicensed")
	}

	name := acctestPrefix + "saml-bad"
	cert := testAccSelfSignedCertPEM(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "mistershell_auth_provider" "bad" {
  name          = %q
  mnemonic      = %q
  provider_type = "SAML"

  config = jsonencode({
    idp_entity_id = "https://idp.example.com/metadata"
    idp_sso_url   = "https://idp.example.com/sso"
    idp_x509_cert = %q
    sp_entity_id  = "https://sp.example.com"
  })
}
`, name, name, cert),
				// Terraform hard-wraps diagnostic text, so match across the
				// wrapping rather than on a literal single-line string.
				ExpectError: regexp.MustCompile(`sp_entity_id:\s+Extra\s+inputs\s+are\s+not\s+permitted`),
			},
		},
	})
}

// TestAccAuthProviderDataSource_byIDAndName covers lookup by id and by exact
// name for EVERY supported provider_type.
func TestAccAuthProviderDataSource_byIDAndName(t *testing.T) {
	testAccPreCheck(t)
	if !authProviderLicensed(t) {
		t.Skip("auth providers require a MisterShell license; instance is unlicensed")
	}

	fixtures := testAccAuthProviderFixtures(t)

	for _, providerType := range client.SupportedAuthProviderTypes {
		fx := fixtures[providerType]
		t.Run(providerType, func(t *testing.T) {
			name := acctestPrefix + strings.ToLower(providerType) + "-ds"
			const byID = "data.mistershell_auth_provider.by_id"
			const byName = "data.mistershell_auth_provider.by_name"

			checks := []resource.TestCheckFunc{
				// by id
				resource.TestCheckResourceAttrPair(byID, "id", "mistershell_auth_provider.test", "id"),
				resource.TestCheckResourceAttr(byID, "name", name),
				resource.TestCheckResourceAttr(byID, "provider_type", providerType),
				resource.TestCheckResourceAttr(byID, "group_mappings_count", "0"),
				resource.TestCheckResourceAttrSet(byID, "config"),
				// by name
				resource.TestCheckResourceAttrPair(byName, "id", "mistershell_auth_provider.test", "id"),
				resource.TestCheckResourceAttr(byName, "provider_type", providerType),
			}
			// The data source must surface the SP URLs on the same terms as the
			// resource.
			checks = append(checks, fx.extraChecks(byID)...)

			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				CheckDestroy:             testAccCheckAllDestroyed,
				Steps: []resource.TestStep{
					{
						Config: testAccAuthProviderDataSourceConfig(name, providerType, fx.config(false)),
						Check:  resource.ComposeAggregateTestCheckFunc(checks...),
					},
				},
			})
		})
	}
}

// TestAccAuthProviderMapping_basic maps an external group to a real
// mistershell_role under EVERY supported provider_type: round-trip, update, and
// compound import.
func TestAccAuthProviderMapping_basic(t *testing.T) {
	testAccPreCheck(t)
	if !authProviderLicensed(t) {
		t.Skip("auth providers require a MisterShell license; instance is unlicensed")
	}

	fixtures := testAccAuthProviderFixtures(t)
	const addr = "mistershell_auth_provider_mapping.test"

	for _, providerType := range client.SupportedAuthProviderTypes {
		fx := fixtures[providerType]
		t.Run(providerType, func(t *testing.T) {
			name := acctestPrefix + strings.ToLower(providerType) + "-map"

			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				CheckDestroy:             testAccCheckAllDestroyed,
				Steps: []resource.TestStep{
					{
						Config: testAccAuthProviderMappingConfig(name, providerType, fx.config(false), "cn=admins,ou=groups,dc=example,dc=com"),
						Check: resource.ComposeAggregateTestCheckFunc(
							resource.TestCheckResourceAttr(addr, "external_group", "cn=admins,ou=groups,dc=example,dc=com"),
							resource.TestCheckResourceAttrPair(addr, "provider_id", "mistershell_auth_provider.test", "id"),
							resource.TestCheckResourceAttrPair(addr, "role_id", "mistershell_role.test", "id"),
							resource.TestCheckResourceAttr(addr, "role_name", name+"-role"),
							resource.TestCheckResourceAttrSet(addr, "id"),
							resource.TestCheckResourceAttrSet(addr, "created_at"),
						),
					},
					// Re-apply identical config: plan must be empty.
					{
						Config:             testAccAuthProviderMappingConfig(name, providerType, fx.config(false), "cn=admins,ou=groups,dc=example,dc=com"),
						PlanOnly:           true,
						ExpectNonEmptyPlan: false,
					},
					// Update the external_group (PATCH).
					{
						Config: testAccAuthProviderMappingConfig(name, providerType, fx.config(false), "cn=ops,ou=groups,dc=example,dc=com"),
						Check: resource.ComposeAggregateTestCheckFunc(
							resource.TestCheckResourceAttr(addr, "external_group", "cn=ops,ou=groups,dc=example,dc=com"),
						),
					},
					// Import with the compound id "<provider_id>:<mapping_id>".
					{
						ResourceName:      addr,
						ImportState:       true,
						ImportStateVerify: true,
						ImportStateIdFunc: func(s *terraform.State) (string, error) {
							rs, ok := s.RootModule().Resources[addr]
							if !ok {
								return "", fmt.Errorf("mapping resource not found in state")
							}
							return fmt.Sprintf("%s:%s", rs.Primary.Attributes["provider_id"], rs.Primary.ID), nil
						},
					},
				},
			})
		})
	}
}

// ---------------------------------------------------------------------------
// Config builders
// ---------------------------------------------------------------------------

func testAccSessionPolicyAclConfig(name, description, enabled, patterns string) string {
	return fmt.Sprintf(`
resource "mistershell_session_policy_acl" "test" {
  name        = %q
  description = %q
  enabled     = %s
  patterns    = %s
}
`, name, description, enabled, patterns)
}

func testAccSessionPolicyAclDataSourceConfig(name string) string {
	return fmt.Sprintf(`
resource "mistershell_session_policy_acl" "test" {
  name     = %q
  patterns = [{ pattern = "cat /etc/shadow", type = "glob" }]
}

data "mistershell_session_policy_acl" "by_id" {
  id = mistershell_session_policy_acl.test.id
}

data "mistershell_session_policy_acl" "by_name" {
  name = mistershell_session_policy_acl.test.name
}
`, name)
}

func testAccSessionPolicyRuleConfig(name string, position int, action, sessionTypes string) string {
	return fmt.Sprintf(`
resource "mistershell_session_policy_rule" "test" {
  name          = %q
  position      = %d
  action        = %q
  session_types = %s
}
`, name, position, action, sessionTypes)
}

func testAccSessionPolicyRuleDataSourceConfig(name string) string {
	return fmt.Sprintf(`
resource "mistershell_session_policy_rule" "test" {
  name     = %q
  position = 23456
  action   = "deny"
}

data "mistershell_session_policy_rule" "by_id" {
  id = mistershell_session_policy_rule.test.id
}
`, name)
}

// testAccSelfSignedCertPEM returns a freshly generated self-signed PEM
// certificate. SAML's idp_x509_cert is validated server-side as a real
// PEM-encoded certificate (BEGIN/END CERTIFICATE markers required), so a
// placeholder string will not do. Generated per-run rather than embedded as a
// constant so the suite can never start failing on certificate expiry.
func testAccSelfSignedCertPEM(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(crand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating test key: %v", err)
	}
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "tfacc-idp"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
	}
	der, err := x509.CreateCertificate(crand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("generating test certificate: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

// The configBody argument on the builders below is a `jsonencode({...})`
// expression supplied by an authProviderFixture.

func testAccAuthProviderConfig(name, providerType string, enabled bool, displayOrder int, configBody string) string {
	return fmt.Sprintf(`
resource "mistershell_auth_provider" "test" {
  name          = %q
  mnemonic      = %q
  provider_type = %q
  is_enabled    = %t
  display_order = %d

  config = %s
}
`, name, name, providerType, enabled, displayOrder, configBody)
}

func testAccAuthProviderDataSourceConfig(name, providerType, configBody string) string {
	return fmt.Sprintf(`
resource "mistershell_auth_provider" "test" {
  name          = %q
  mnemonic      = %q
  provider_type = %q

  config = %s
}

data "mistershell_auth_provider" "by_id" {
  id = mistershell_auth_provider.test.id
}

data "mistershell_auth_provider" "by_name" {
  name = mistershell_auth_provider.test.name
}
`, name, name, providerType, configBody)
}

func testAccAuthProviderMappingConfig(name, providerType, configBody, externalGroup string) string {
	return fmt.Sprintf(`
resource "mistershell_role" "test" {
  name        = %q
  description = "wave3 mapping role"
  permissions = ["app.tags.read"]
}

resource "mistershell_auth_provider" "test" {
  name          = %q
  mnemonic      = %q
  provider_type = %q

  config = %s
}

resource "mistershell_auth_provider_mapping" "test" {
  provider_id    = mistershell_auth_provider.test.id
  external_group = %q
  role_id        = mistershell_role.test.id
}
`, name+"-role", name+"-prov", name+"-prov", providerType, configBody, externalGroup)
}
