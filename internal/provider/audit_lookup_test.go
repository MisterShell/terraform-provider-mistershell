package provider_test

import (
	"fmt"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"strconv"
	"strings"
	"testing"
)

// Each complete lookup value exceeds the API's 100-character search bound.
// Queries may use a bounded prefix, but matching must use the complete value.
func TestAccAuditLongLookups(t *testing.T) {
	testAccSSHResourcePreCheck(t)
	name := acctestPrefix + strings.Repeat("l", 110-len(acctestPrefix))
	email := acctestPrefix + strings.Repeat("u", 60-len(acctestPrefix)) + "@" + strings.Repeat("d", 50) + ".example.com"
	config := strings.Replace(testAccSSHResourceConfig(name, "generic_ssh"), strconv.Quote(name+"-location"), strconv.Quote(acctestPrefix+"lookup-location"), 1) + fmt.Sprintf(`
resource "mistershell_worker" "audit" {
 name=%q
 location_id=mistershell_location.test.id
}
resource "mistershell_external_vault" "audit" {
 name=%q
 mnemonic=%q
 vault_type="hashicorp"
 config={address="https://vault.example.invalid",auth_method="token",token="audit-token"}
}
resource "mistershell_session_policy_acl" "audit" {
 name=%q
 patterns=[{pattern="audit-*",type="glob"}]
}
resource "mistershell_log_destination" "audit" {
 name=%q
 type="syslog"
 enabled=false
 streams=["security"]
 min_severity="medium"
 config={type="syslog",host="logs.example.invalid",port=514,protocol="UDP",format="RFC5424",facility="local1"}
}
resource "mistershell_user" "audit" {
 email=%q
 first_name="Lookup"
 last_name="Audit"
 password="AuditPassword123!"
 is_active=false
}
resource "mistershell_ai_model" "audit" {
 name=%q
 model_provider="ollama"
 model_id="llama3"
 config={base_url="http://localhost:11434"}
}
resource "mistershell_ai_prompt" "audit" {
 name=%q
 content="Audit lookup prompt"
}
resource "mistershell_ai_agent" "audit" {
 name=%q
 type="chat"
 model_id=mistershell_ai_model.audit.id
 system_prompt_id=mistershell_ai_prompt.audit.id
 config={token_budget=4000}
}
resource "mistershell_ai_skill" "audit" {
 name=%q
 description="Audit lookup skill"
 body="Audit lookup body"
 is_enabled=false
}
`, name+"-worker", name+"-vault", acctestPrefix+"lookup-vault", name+"-acl", name+"-log", email, name+"-model", name+"-prompt", name+"-agent", name+"-skill")
	var checks []resource.TestCheckFunc
	for _, kind := range []string{"resource", "worker", "credential", "external_vault", "session_policy_acl", "log_destination", "user", "ai_model", "ai_prompt", "ai_agent", "ai_skill"} {
		instance := "audit"
		if kind == "resource" || kind == "credential" {
			instance = "test"
		}
		field := "name"
		if kind == "user" {
			field = "email"
		}
		managed := "mistershell_" + kind + "." + instance
		lookup := "data.mistershell_" + kind + ".audit"
		config += fmt.Sprintf("\ndata %q %q { %s = %s.%s }\n", "mistershell_"+kind, "audit", field, managed, field)
		checks = append(checks, resource.TestCheckResourceAttrPair(lookup, "id", managed, "id"), resource.TestCheckResourceAttrPair(lookup, field, managed, field))
	}
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, CheckDestroy: testAccCheckAllDestroyed, Steps: []resource.TestStep{{Config: config, Check: resource.ComposeAggregateTestCheckFunc(checks...)}}})
}
