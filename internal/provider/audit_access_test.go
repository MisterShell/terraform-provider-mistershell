package provider_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"terraform-provider-mistershell/internal/client"
)

func TestAccAuditAccessRoleAtomic(t *testing.T) {
	testAccPreCheck(t)
	base := fmt.Sprintf(`resource "mistershell_location" "audit" {

name=%q
kind="org"
parent_id=1
}
 resource "mistershell_role" "audit" {

name=%q
%%s
}
 data "mistershell_role" "audit" {

id=mistershell_role.audit.id
}`, acctestPrefix+"access-location", acctestPrefix+"access-role")
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, CheckDestroy: testAccCheckAllDestroyed, Steps: []resource.TestStep{
		{Config: fmt.Sprintf(base, `permissions=["*.*.*"]`), Check: resource.TestCheckResourceAttr("mistershell_role.audit", "is_builtin", "false")},
		{Config: fmt.Sprintf(base, `location_scope_mode="restricted"
scope_location_ids=[mistershell_location.audit.id]
permissions=["app.resources.read"]`), Check: resource.TestCheckResourceAttr("data.mistershell_role.audit", "location_scope_mode", "restricted")},
		{Config: fmt.Sprintf(base, `permissions=["*.*.*"]`), Check: resource.TestCheckResourceAttr("mistershell_role.audit", "permissions.0", "*.*.*")},
	}})
}

func TestAccAuditAccessUserDetailAndExternalUnmanaged(t *testing.T) {
	testAccPreCheck(t)
	base := fmt.Sprintf(`resource "mistershell_user" "test" {

email=%q
first_name="Audit"
last_name="%%s"
auth_type="OIDC"
%%s
}
 resource "mistershell_role" "test" {

name=%q
permissions=["app.tags.read"]
}
 resource "mistershell_user_role" "test" {

user_id=mistershell_user.test.id
role_id=mistershell_role.test.id
}
 data "mistershell_user" "audit" {

email=upper(mistershell_user.test.email)
depends_on=[mistershell_user_role.test]
}
 data "mistershell_user" "mixed" {
 email=replace(mistershell_user.test.email,"@example.com","@Example.COM")
 depends_on=[mistershell_user_role.test]
}`, acctestPrefix+"audit-external@example.com", acctestPrefix+"audit-user-role")
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, CheckDestroy: testAccCheckAllDestroyed, Steps: []resource.TestStep{
		{Config: fmt.Sprintf(base, "Before", `password="Audit-pass-123!"`), Check: resource.ComposeAggregateTestCheckFunc(resource.TestCheckResourceAttr("data.mistershell_user.audit", "permissions.#", "1"), resource.TestCheckResourceAttr("data.mistershell_user.audit", "roles.0.is_builtin", "false"),
			resource.TestCheckResourceAttrPair("data.mistershell_user.audit", "id", "mistershell_user.test", "id"),
			resource.TestCheckResourceAttrPair("data.mistershell_user.audit", "email", "mistershell_user.test", "email"),
			resource.TestCheckResourceAttrPair("data.mistershell_user.mixed", "id", "mistershell_user.test", "id"),
			resource.TestCheckResourceAttrPair("data.mistershell_user.mixed", "email", "mistershell_user.test", "email"),
			resource.TestCheckResourceAttr("data.mistershell_user.mixed", "permissions.#", "1"))},
		{ResourceName: "mistershell_user.test", ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: []string{"password", "roles", "permissions"}},
		{Config: fmt.Sprintf(base, "After", ``), Check: resource.TestCheckResourceAttr("mistershell_user.test", "last_name", "After")},
	}})
}

func TestAccAuditAccessAuthConfigAndMapping(t *testing.T) {
	testAccPreCheck(t)
	if !authProviderLicensed(t) {
		t.Skip("requires active license")
	}
	var providerID int64
	config := func(extra string) string {
		return fmt.Sprintf(`resource "mistershell_auth_provider" "audit" {


name=%q
mnemonic=%q
provider_type="OIDC"
is_enabled=false

config={
issuer_url="https://idp.example.com"
client_id="audit"
client_secret="audit-secret"
%s
}

}
 resource "mistershell_role" "audit" {
name=%q
permissions=["app.tags.read"]
}
 resource "mistershell_auth_provider_mapping" "audit" {
provider_id=mistershell_auth_provider.audit.id
external_group="audit-group"
role_id=mistershell_role.audit.id
}
 data "mistershell_auth_provider_mapping" "by_group" {
provider_id=mistershell_auth_provider.audit.id
external_group=mistershell_auth_provider_mapping.audit.external_group
}
 data "mistershell_auth_provider_mapping" "by_id" {
provider_id=mistershell_auth_provider.audit.id
id=mistershell_auth_provider_mapping.audit.id
}
 `, acctestPrefix+"audit-oidc", acctestPrefix+"audit-oidc", extra, acctestPrefix+"audit-map-role")
	}
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, CheckDestroy: testAccCheckAllDestroyed, Steps: []resource.TestStep{
		{Config: config(`mcp_audience="audit-api"
tls_verify=false
scopes=["openid"]
additional_params={
prompt="login"
}`), Check: func(s *terraform.State) error {
			var err error
			providerID, err = strconv.ParseInt(s.RootModule().Resources["mistershell_auth_provider.audit"].Primary.ID, 10, 64)
			return err
		}},
		{Config: config(""), Check: resource.ComposeAggregateTestCheckFunc(resource.TestCheckResourceAttrPair("data.mistershell_auth_provider_mapping.by_id", "id", "data.mistershell_auth_provider_mapping.by_group", "id"), func(_ *terraform.State) error {
			got, err := testAccClient().GetAuthProvider(context.Background(), providerID)
			if err != nil {
				return err
			}
			var values map[string]any
			if err = json.Unmarshal(got.Config, &values); err != nil {
				return err
			}
			if values["mcp_audience"] != nil || values["tls_verify"] != true || len(values["scopes"].([]any)) != 3 || len(values["additional_params"].(map[string]any)) != 0 {
				return fmt.Errorf("removed configuration fields were not reset")
			}
			return nil
		})},
		{Config: config(""), PlanOnly: true, ExpectNonEmptyPlan: true, PreConfig: func() {
			_, err := testAccClient().UpdateAuthProvider(context.Background(), providerID, client.AuthProviderUpdateInput{Config: json.RawMessage(`{"issuer_url":"https://drift.example.com"}`)})
			if err != nil {
				t.Fatal(err)
			}
		}},
		{Config: config(""), Check: func(_ *terraform.State) error {
			got, err := testAccClient().GetAuthProvider(context.Background(), providerID)
			if err != nil {
				return err
			}
			var cfg map[string]any
			if err = json.Unmarshal(got.Config, &cfg); err != nil {
				return err
			}
			if cfg["issuer_url"] != "https://idp.example.com" {
				return fmt.Errorf("auth provider drift was not corrected")
			}
			return nil
		}},
		{ResourceName: "mistershell_auth_provider.audit", ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: []string{"config"}},
	}})
}

func TestAccAuditAccessLogDestinationRefreshAndType(t *testing.T) {
	testAccPreCheck(t)
	var id int64
	config := func(kind, cfg string) string {
		return fmt.Sprintf(`resource "mistershell_log_destination" "audit" {
name=%q
enabled=false
type=%q
streams=["security"]
config=%s
}`, acctestPrefix+"audit-log", kind, cfg)
	}
	webhook := config("webhook", `{
type="webhook"
url="https://example.com/logs"
auth={
type="bearer"
token="audit-secret"
}
timeout_seconds=12
}`)
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, CheckDestroy: testAccCheckAllDestroyed, Steps: []resource.TestStep{
		{Config: webhook, Check: func(s *terraform.State) error {
			var err error
			id, err = strconv.ParseInt(s.RootModule().Resources["mistershell_log_destination.audit"].Primary.ID, 10, 64)
			return err
		}},
		{Config: webhook, PreConfig: func() {
			enabled := false
			_, err := testAccClient().UpdateLogDestination(context.Background(), id, client.LogDestinationUpdateInput{Name: acctestPrefix + "audit-log", Enabled: &enabled, Type: "webhook", Streams: []string{"security"}, MinSeverity: "info", Config: json.RawMessage(`{
"type":"webhook","url":"https://example.com/drift","auth":{
"type":"bearer","token":"audit-secret"
},"timeout_seconds":12
}`)})
			if err != nil {
				t.Fatal(err)
			}
		}, Check: func(_ *terraform.State) error {
			got, err := testAccClient().GetLogDestination(context.Background(), id)
			if err != nil {
				return err
			}
			var cfg map[string]any
			if err = json.Unmarshal(got.Config, &cfg); err != nil {
				return err
			}
			if cfg["url"] != "https://example.com/logs" {
				return fmt.Errorf("log destination drift was not corrected")
			}
			return nil
		}},
		{Config: config("syslog", `{
type="syslog"
host="127.0.0.1"
port=514
protocol="UDP"
format="RFC5424"
}`), Check: func(s *terraform.State) error {
			if s.RootModule().Resources["mistershell_log_destination.audit"].Primary.ID != strconv.FormatInt(id, 10) {
				return fmt.Errorf("changing type replaced destination")
			}
			return nil
		}},
		{ResourceName: "mistershell_log_destination.audit", ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: []string{"config"}},
	}})
}

func TestAccAuditAccessRolePermissionsRemoval(t *testing.T) {
	testAccPreCheck(t)
	var id int64
	config := func(permissions string) string {
		return fmt.Sprintf("resource \"mistershell_role\" \"audit\" {\n name=%q\n %s\n}\n", acctestPrefix+"permission-removal", permissions)
	}
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, CheckDestroy: testAccCheckAllDestroyed, Steps: []resource.TestStep{
		{Config: config(`permissions=["app.tags.read"]`), Check: func(s *terraform.State) error {
			var err error
			id, err = strconv.ParseInt(s.RootModule().Resources["mistershell_role.audit"].Primary.ID, 10, 64)
			return err
		}},
		{ResourceName: "mistershell_role.audit", ImportState: true, ImportStateVerify: true},
		{Config: config(""), Check: resource.ComposeAggregateTestCheckFunc(resource.TestCheckNoResourceAttr("mistershell_role.audit", "permissions.#"), func(_ *terraform.State) error {
			role, err := testAccClient().GetRole(context.Background(), id)
			if err != nil {
				return err
			}
			if len(role.Permissions) != 0 {
				return fmt.Errorf("removing permissions retained %d grants", len(role.Permissions))
			}
			return nil
		})},
		{Config: config(""), PlanOnly: true, ExpectNonEmptyPlan: false},
		{Config: config(`permissions=[]`)},
	}})
}

func TestAccAuditConfigurationPlaybookWebhookSecrets(t *testing.T) {
	testAccPreCheck(t)
	for _, authType := range []string{"bearer", "basic", "header"} {
		t.Run(authType, func(t *testing.T) {
			var id string
			secretField := map[string]string{"bearer": "token", "basic": "password", "header": "header_value"}[authType]
			graph := func(secret, url string) string {
				auth := map[string]any{"type": authType, secretField: secret}
				if authType == "basic" {
					auth["username"] = "audit"
				}
				if authType == "header" {
					auth["header_name"] = "X-Audit-Key"
				}
				raw, _ := json.Marshal(map[string]any{
					"trigger": map[string]any{"event_name": "resource.session.recorded"},
					"nodes":   []any{map[string]any{"id": "webhook", "action_id": "webhook", "position": map[string]any{"x": 200, "y": 0}, "config": map[string]any{"url": url, "auth": auth}}},
					"edges":   []any{map[string]any{"from": "trigger", "to": "webhook", "when": "always"}},
				})
				return string(raw)
			}
			config := func(secret, url string) string {
				return fmt.Sprintf("resource \"mistershell_automation_playbook\" \"audit\" {\n name=%q\n enabled=false\n graph=jsondecode(%q)\n}\n", acctestPrefix+"webhook-"+authType, graph(secret, url))
			}
			check := func(secret, url string) resource.TestCheckFunc {
				return resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_automation_playbook.audit", "graph.nodes.0.config.auth."+secretField, secret),
					resource.TestCheckResourceAttr("mistershell_automation_playbook.audit", "graph.nodes.0.config.url", url),
					func(s *terraform.State) error {
						id = s.RootModule().Resources["mistershell_automation_playbook.audit"].Primary.ID
						raw, err := testAccClient().ConfigurationRequest(context.Background(), "GET", "/api/v1/automation/playbooks/"+id, nil)
						if err != nil {
							return err
						}
						var remote struct {
							Graph struct {
								Nodes []struct {
									Config struct {
										Auth map[string]any `json:"auth"`
									} `json:"config"`
								} `json:"nodes"`
							} `json:"graph"`
						}
						if err = json.Unmarshal(raw, &remote); err != nil {
							return err
						}
						if len(remote.Graph.Nodes) != 1 || remote.Graph.Nodes[0].Config.Auth[secretField] != "****" {
							return fmt.Errorf("API did not mask webhook authentication secret")
						}
						return nil
					},
				)
			}
			resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, CheckDestroy: func(_ *terraform.State) error {
				_, err := testAccClient().ConfigurationRequest(context.Background(), "GET", "/api/v1/automation/playbooks/"+id, nil)
				if client.IsNotFound(err) {
					return nil
				}
				if err != nil {
					return err
				}
				return fmt.Errorf("playbook survived deletion")
			}, Steps: []resource.TestStep{
				{Config: config("initial-secret", "https://example.com/initial"), Check: check("initial-secret", "https://example.com/initial")},
				{Config: config("initial-secret", "https://example.com/initial"), PlanOnly: true, ExpectNonEmptyPlan: false},
				{Config: config("rotated-secret", "https://example.com/updated"), Check: check("rotated-secret", "https://example.com/updated")},
				{Config: config("rotated-secret", "https://example.com/updated"), PlanOnly: true, ExpectNonEmptyPlan: false},
				{ResourceName: "mistershell_automation_playbook.audit", ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: []string{"graph"}, ImportStateCheck: func(states []*terraform.InstanceState) error {
					for _, state := range states {
						if value := state.Attributes["graph.nodes.0.config.auth."+secretField]; value != "" {
							return fmt.Errorf("import must not expose a masked webhook secret as a credential")
						}
					}
					return nil
				}},
			}})
		})
	}
}
