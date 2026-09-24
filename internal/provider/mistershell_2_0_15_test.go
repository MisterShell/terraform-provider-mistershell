package provider_test

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"terraform-provider-mistershell/internal/client"
)

// Coverage for the provider surface added for MisterShell 2.0.15 (approval
// rules since 2.0.10, LDAP MFA methods since 2.0.12, and the 2.0.15 permission
// and settings registries).

const approveRuleAddr = "mistershell_session_policy_rule.test"

func testAccApproveRuleConfig(name, action, approval string) string {
	return fmt.Sprintf(`
resource "mistershell_role" "approvers" {
  name        = "%[1]s-approvers"
  permissions = ["app.resources.read"]
}

resource "mistershell_session_policy_rule" "test" {
  name          = %[1]q
  position      = 34567
  action        = %[2]q
  session_types = ["shell"]
  %[3]s
}

data "mistershell_session_policy_rule" "by_id" {
  id = mistershell_session_policy_rule.test.id
}
`, name, action, approval)
}

// testAccCheckLiveRuleApproval reads the rule straight from the API so the
// test proves MisterShell stored (or cleared) the approval, not just the plan.
func testAccCheckLiveRuleApproval(want func(*client.RuleApproval) error) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		id, err := strconv.ParseInt(state.RootModule().Resources[approveRuleAddr].Primary.ID, 10, 64)
		if err != nil {
			return err
		}
		rule, err := testAccClient().GetRule(context.Background(), id)
		if err != nil {
			return err
		}
		return want(rule.Approval)
	}
}

func TestAccSessionPolicyRule_approve(t *testing.T) {
	testAccPreCheck(t)
	if existingRuleCount(t) < 1 {
		t.Skip("session-policy rule test requires at least one pre-existing rule so destroy never removes the last rule; instance has none")
	}
	name := acctestPrefix + "approve-rule"
	initial := `approval = {
    approver_role_ids  = [mistershell_role.approvers.id]
    expiration_seconds = 3600
    notify_email       = true
  }`
	updated := `approval = {
    approver_role_ids   = [mistershell_role.approvers.id]
    expiration_seconds  = 3600
    validity_seconds    = 7200
    notify_email        = true
    allow_self_approval = true
  }`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccApproveRuleConfig(name, "approve", initial),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(approveRuleAddr, "action", "approve"),
					resource.TestCheckResourceAttr(approveRuleAddr, "approval.approver_role_ids.#", "1"),
					resource.TestCheckTypeSetElemAttrPair(approveRuleAddr, "approval.approver_role_ids.*", "mistershell_role.approvers", "id"),
					resource.TestCheckResourceAttr(approveRuleAddr, "approval.expiration_seconds", "3600"),
					resource.TestCheckResourceAttr(approveRuleAddr, "approval.validity_seconds", "3600"),
					resource.TestCheckResourceAttr(approveRuleAddr, "approval.notify_email", "true"),
					resource.TestCheckResourceAttr(approveRuleAddr, "approval.allow_self_approval", "false"),
					resource.TestCheckResourceAttr("data.mistershell_session_policy_rule.by_id", "action", "approve"),
					resource.TestCheckResourceAttr("data.mistershell_session_policy_rule.by_id", "approval.expiration_seconds", "3600"),
					resource.TestCheckResourceAttr("data.mistershell_session_policy_rule.by_id", "approval.notify_email", "true"),
					testAccCheckLiveRuleApproval(func(a *client.RuleApproval) error {
						if a == nil || a.ExpirationSeconds != 3600 || a.ValiditySeconds != 3600 || !a.NotifyEmail || a.AllowSelfApproval || len(a.ApproverRoleIDs) != 1 {
							return fmt.Errorf("live approval = %+v", a)
						}
						return nil
					}),
				),
			},
			{Config: testAccApproveRuleConfig(name, "approve", initial), PlanOnly: true, ExpectNonEmptyPlan: false},
			{
				Config: testAccApproveRuleConfig(name, "approve", updated),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(approveRuleAddr, "approval.validity_seconds", "7200"),
					resource.TestCheckResourceAttr(approveRuleAddr, "approval.allow_self_approval", "true"),
					testAccCheckLiveRuleApproval(func(a *client.RuleApproval) error {
						if a == nil || a.ValiditySeconds != 7200 || !a.AllowSelfApproval {
							return fmt.Errorf("live approval = %+v", a)
						}
						return nil
					}),
				),
			},
			{
				ResourceName:      approveRuleAddr,
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Switching back to accept must clear the stored approval.
			{
				Config: testAccApproveRuleConfig(name, "accept", ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(approveRuleAddr, "action", "accept"),
					resource.TestCheckNoResourceAttr(approveRuleAddr, "approval.%"),
					resource.TestCheckNoResourceAttr("data.mistershell_session_policy_rule.by_id", "approval.%"),
					testAccCheckLiveRuleApproval(func(a *client.RuleApproval) error {
						if a != nil {
							return fmt.Errorf("live approval after switching to accept = %+v, want null", a)
						}
						return nil
					}),
				),
			},
			{Config: testAccApproveRuleConfig(name, "accept", ""), PlanOnly: true, ExpectNonEmptyPlan: false},
		},
	})
}

func TestAccSessionPolicyRule_approveValidation(t *testing.T) {
	testAccPreCheck(t)
	name := acctestPrefix + "approve-invalid"
	approval := `approval = { approver_role_ids = [mistershell_role.approvers.id] }`
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			{
				Config:      testAccApproveRuleConfig(name, "approve", ""),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Missing approval configuration`),
			},
			{
				Config:      testAccApproveRuleConfig(name, "deny", approval),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Unexpected approval configuration`),
			},
			{
				Config:      testAccApproveRuleConfig(name, "approve", approval+"\n  command_acl_ids = [1]"),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Command ACLs not allowed on approve rules`),
			},
			{
				Config:      testAccApproveRuleConfig(name, "approve", `approval = { approver_role_ids = [mistershell_role.approvers.id], validity_seconds = 30 }`),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`validity_seconds`),
			},
		},
	})
}

func testAccLDAPMfaConfig(name, mfaMethodLine string) string {
	return fmt.Sprintf(`
resource "mistershell_auth_provider" "test" {
  name          = %[1]q
  mnemonic      = %[1]q
  provider_type = "LDAP"
  is_enabled    = false

  config = {
    server_url    = "ldaps://ldap.example.com:636"
    bind_dn       = "cn=svc,dc=example,dc=com"
    bind_password = "tfacc-bind-password"
    base_dn       = "ou=people,dc=example,dc=com"
    %[2]s
  }
}

data "mistershell_auth_provider" "by_id" {
  id = mistershell_auth_provider.test.id
}
`, name, mfaMethodLine)
}

// LDAP mfa_method round-trips for the supported values, an omitted key leaves
// the stored method unchanged, and email is refused at plan time. The invalid
// configs run first: the post-test destroy uses the last step's config.
func TestAccAuthProvider_ldapMfaMethod(t *testing.T) {
	testAccPreCheck(t)
	if !authProviderLicensed(t) {
		t.Skip("auth providers require a MisterShell license; instance is unlicensed")
	}
	name := acctestPrefix + "ldap-mfa"
	const addr = "mistershell_auth_provider.test"
	const ds = "data.mistershell_auth_provider.by_id"
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			{
				Config:      testAccLDAPMfaConfig(name, `mfa_method = "sms"`),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`config.mfa_method must be one of: none, totp`),
			},
			{
				Config:      testAccLDAPMfaConfig(name, `mfa_method = "email"`),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Email MFA cannot be managed with Terraform`),
			},
			{
				Config: testAccLDAPMfaConfig(name, `mfa_method = "totp"`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "config.mfa_method", "totp"),
					resource.TestCheckResourceAttr(ds, "config.mfa_method", "totp"),
				),
			},
			{Config: testAccLDAPMfaConfig(name, `mfa_method = "totp"`), PlanOnly: true, ExpectNonEmptyPlan: false},
			{
				Config: testAccLDAPMfaConfig(name, `mfa_method = "none"`),
				Check:  resource.TestCheckResourceAttr(ds, "config.mfa_method", "none"),
			},
			{
				Config: testAccLDAPMfaConfig(name, `mfa_method = "totp"`),
				Check:  resource.TestCheckResourceAttr(ds, "config.mfa_method", "totp"),
			},
			// Dropping the key updates config but keeps the stored method, which
			// is how a UI-managed method (e.g. email) survives applies.
			{
				Config: testAccLDAPMfaConfig(name, `connection_timeout = 15`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(ds, "config.mfa_method", "totp"),
					resource.TestCheckResourceAttr(ds, "config.connection_timeout", "15"),
				),
			},
		},
	})
}

// Backend 2.0.15 retired app.tasks.delete and split *.execute evidence
// visibility out of three *.read grants.
func TestAccRole_2015Permissions(t *testing.T) {
	testAccPreCheck(t)
	name := acctestPrefix + "perms-2015"
	current := testAccRolePermissionsConfig(name, `["app.collector.read", "app.collector.execute", "app.sensors.read", "app.sensors.execute", "app.fact_policies.read", "app.fact_policies.execute", "app.tasks.write"]`)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			{
				Config:      testAccRolePermissionsConfig(name, `["app.tasks.write", "app.tasks.delete"]`),
				ExpectError: regexp.MustCompile(`Invalid permission`),
			},
			{Config: current, Check: resource.TestCheckResourceAttr("mistershell_role.test", "permissions.#", "7")},
			{Config: current, PlanOnly: true, ExpectNonEmptyPlan: false},
		},
	})
}

func TestAccSetting_2015Keys(t *testing.T) {
	testAccPreCheck(t)
	const key = "sshgw_max_tabs"
	c := testAccClient()
	original, err := c.GetSetting(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	originalValue := append(json.RawMessage(nil), original.Value...)
	t.Cleanup(func() {
		if _, err := c.UpdateSetting(context.Background(), key, client.SettingUpdateInput{Value: originalValue}); err != nil {
			t.Errorf("restoring original %s: %v", key, err)
		}
	})
	config := `resource "mistershell_setting" "test" {
  key   = "sshgw_max_tabs"
  value = 9
}`
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			{Config: config, Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("mistershell_setting.test", "value", "9"),
				func(_ *terraform.State) error {
					live, err := c.GetSetting(context.Background(), key)
					if err != nil {
						return err
					}
					if strings.TrimSpace(string(live.Value)) != "9" {
						return fmt.Errorf("live %s = %s, want 9", key, live.Value)
					}
					return nil
				},
			)},
			{Config: config, PlanOnly: true, ExpectNonEmptyPlan: false},
			{
				Config: `resource "mistershell_setting" "local_mfa" {
  key   = "local_mfa"
  value = { method = "none", revision = "tfacc" }
}`,
				ExpectError: regexp.MustCompile(`Manage Local MFA through Auth Providers`),
			},
		},
	})
}
