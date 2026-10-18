package provider_test

import (
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"testing"
)

func TestAccAuditTypeCatalogs(t *testing.T) {
	resource.Test(t, resource.TestCase{PreCheck: func() { testAccPreCheck(t) }, ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, Steps: []resource.TestStep{{Config: `data "mistershell_credential_types" "all" {}
data "mistershell_external_vault_types" "all" {}
output "types_present" {
 value = length(data.mistershell_credential_types.all.types)
 precondition {
 condition = length(data.mistershell_credential_types.all.types) > 0 && length(data.mistershell_external_vault_types.all.types) == 4
 error_message = "Expected installed credential types and four vault types."
 }
}`, Check: resource.ComposeAggregateTestCheckFunc(resource.TestCheckResourceAttr("data.mistershell_credential_types.all", "id", "catalog"), resource.TestCheckResourceAttr("data.mistershell_external_vault_types.all", "id", "catalog"))}}})
}
