package provider_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
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
// mistershell_auth_provider — OIDC (license-gated): create with secret config,
// round-trip from config, display_order, update a non-secret field, data source.
// ---------------------------------------------------------------------------

func TestAccAuthProvider_oidc(t *testing.T) {
	testAccPreCheck(t)
	if !authProviderLicensed(t) {
		t.Skip("auth providers require a MisterShell license; instance is unlicensed")
	}

	name := acctestPrefix + "oidc"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			// Create OIDC with a client_secret, explicit display_order, is_enabled.
			{
				Config: testAccAuthProviderOIDCConfig(name, true, 7, "https://idp.example.com", "tfacc-client-secret"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_auth_provider.test", "name", name),
					resource.TestCheckResourceAttr("mistershell_auth_provider.test", "provider_type", "OIDC"),
					resource.TestCheckResourceAttr("mistershell_auth_provider.test", "is_enabled", "true"),
					// display_order set via post-create PATCH (create cannot set it).
					resource.TestCheckResourceAttr("mistershell_auth_provider.test", "display_order", "7"),
					resource.TestCheckResourceAttr("mistershell_auth_provider.test", "group_mappings_count", "0"),
					resource.TestCheckResourceAttrSet("mistershell_auth_provider.test", "config"),
					resource.TestCheckResourceAttrSet("mistershell_auth_provider.test", "id"),
					resource.TestCheckResourceAttrSet("mistershell_auth_provider.test", "created_at"),
				),
			},
			// Re-apply identical config: plan must be empty (proves config is
			// preserved from plan across masked secrets AND server-enriched
			// defaults — the syslog lesson).
			{
				Config:             testAccAuthProviderOIDCConfig(name, true, 7, "https://idp.example.com", "tfacc-client-secret"),
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
			// Update non-secret fields: is_enabled=false, display_order=9,
			// issuer_url changed (config edit).
			{
				Config: testAccAuthProviderOIDCConfig(name, false, 9, "https://idp2.example.com", "tfacc-client-secret"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_auth_provider.test", "is_enabled", "false"),
					resource.TestCheckResourceAttr("mistershell_auth_provider.test", "display_order", "9"),
				),
			},
			// Import: config carries a secret masked by the API and enriched
			// defaults, so it cannot be verified on import — ignore it.
			{
				ResourceName:            "mistershell_auth_provider.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"config"},
			},
		},
	})
}

func TestAccAuthProviderDataSource_byIDAndName(t *testing.T) {
	testAccPreCheck(t)
	if !authProviderLicensed(t) {
		t.Skip("auth providers require a MisterShell license; instance is unlicensed")
	}

	name := acctestPrefix + "oidc-ds"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccAuthProviderDataSourceConfig(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					// by id
					resource.TestCheckResourceAttrPair("data.mistershell_auth_provider.by_id", "id", "mistershell_auth_provider.test", "id"),
					resource.TestCheckResourceAttr("data.mistershell_auth_provider.by_id", "name", name),
					resource.TestCheckResourceAttr("data.mistershell_auth_provider.by_id", "provider_type", "OIDC"),
					resource.TestCheckResourceAttr("data.mistershell_auth_provider.by_id", "group_mappings_count", "0"),
					resource.TestCheckResourceAttrSet("data.mistershell_auth_provider.by_id", "config"),
					// by name
					resource.TestCheckResourceAttrPair("data.mistershell_auth_provider.by_name", "id", "mistershell_auth_provider.test", "id"),
					resource.TestCheckResourceAttr("data.mistershell_auth_provider.by_name", "provider_type", "OIDC"),
				),
			},
		},
	})
}

// ---------------------------------------------------------------------------
// mistershell_auth_provider_mapping — (license-gated) create under an OIDC
// provider, mapping external_group -> a real mistershell_role; round-trip,
// compound import, update.
// ---------------------------------------------------------------------------

func TestAccAuthProviderMapping_basic(t *testing.T) {
	testAccPreCheck(t)
	if !authProviderLicensed(t) {
		t.Skip("auth providers require a MisterShell license; instance is unlicensed")
	}

	name := acctestPrefix + "map"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			// Create a provider, a role, and a mapping linking an external group
			// to that role.
			{
				Config: testAccAuthProviderMappingConfig(name, "cn=admins,ou=groups,dc=example,dc=com"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_auth_provider_mapping.test", "external_group", "cn=admins,ou=groups,dc=example,dc=com"),
					resource.TestCheckResourceAttrPair("mistershell_auth_provider_mapping.test", "provider_id", "mistershell_auth_provider.test", "id"),
					resource.TestCheckResourceAttrPair("mistershell_auth_provider_mapping.test", "role_id", "mistershell_role.test", "id"),
					resource.TestCheckResourceAttr("mistershell_auth_provider_mapping.test", "role_name", name+"-role"),
					resource.TestCheckResourceAttrSet("mistershell_auth_provider_mapping.test", "id"),
					resource.TestCheckResourceAttrSet("mistershell_auth_provider_mapping.test", "created_at"),
				),
			},
			// Re-apply identical config: plan must be empty.
			{
				Config:             testAccAuthProviderMappingConfig(name, "cn=admins,ou=groups,dc=example,dc=com"),
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
			// Update the external_group (PATCH).
			{
				Config: testAccAuthProviderMappingConfig(name, "cn=ops,ou=groups,dc=example,dc=com"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_auth_provider_mapping.test", "external_group", "cn=ops,ou=groups,dc=example,dc=com"),
				),
			},
			// Import with the compound id "<provider_id>:<mapping_id>".
			{
				ResourceName:      "mistershell_auth_provider_mapping.test",
				ImportState:       true,
				ImportStateVerify: true,
				// Compound import id "<provider_id>:<mapping_id>".
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs, ok := s.RootModule().Resources["mistershell_auth_provider_mapping.test"]
					if !ok {
						return "", fmt.Errorf("mapping resource not found in state")
					}
					return fmt.Sprintf("%s:%s", rs.Primary.Attributes["provider_id"], rs.Primary.ID), nil
				},
			},
		},
	})
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

func testAccAuthProviderOIDCConfig(name string, enabled bool, displayOrder int, issuerURL, clientSecret string) string {
	return fmt.Sprintf(`
resource "mistershell_auth_provider" "test" {
  name          = %q
  provider_type = "OIDC"
  is_enabled    = %t
  display_order = %d

  config = jsonencode({
    issuer_url    = %q
    client_id     = "tfacc-client-id"
    client_secret = %q
  })
}
`, name, enabled, displayOrder, issuerURL, clientSecret)
}

func testAccAuthProviderDataSourceConfig(name string) string {
	return fmt.Sprintf(`
resource "mistershell_auth_provider" "test" {
  name          = %q
  provider_type = "OIDC"

  config = jsonencode({
    issuer_url    = "https://idp.example.com"
    client_id     = "tfacc-client-id"
    client_secret = "tfacc-client-secret"
  })
}

data "mistershell_auth_provider" "by_id" {
  id = mistershell_auth_provider.test.id
}

data "mistershell_auth_provider" "by_name" {
  name = mistershell_auth_provider.test.name
}
`, name)
}

func testAccAuthProviderMappingConfig(name, externalGroup string) string {
	return fmt.Sprintf(`
resource "mistershell_role" "test" {
  name        = %q
  description = "wave3 mapping role"
  permissions = ["app.tags.read"]
}

resource "mistershell_auth_provider" "test" {
  name          = %q
  provider_type = "OIDC"

  config = jsonencode({
    issuer_url    = "https://idp.example.com"
    client_id     = "tfacc-client-id"
    client_secret = "tfacc-client-secret"
  })
}

resource "mistershell_auth_provider_mapping" "test" {
  provider_id    = mistershell_auth_provider.test.id
  external_group = %q
  role_id        = mistershell_role.test.id
}
`, name+"-role", name+"-prov", externalGroup)
}
