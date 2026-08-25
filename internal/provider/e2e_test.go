package provider_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// This live graph intentionally creates one reachable resource. MisterShell
// 2.0 proves every onboarding through a real connection, so an exhaustive
// synthetic graph of fake targets is neither valid nor useful.
func TestAccEndToEnd_onboardingGraph(t *testing.T) {
	testAccSSHResourcePreCheck(t)
	config := testAccSSHResourceConfig(acctestPrefix+"e2e", "") + `
data "mistershell_location" "test" {
  id = mistershell_location.test.id
}

data "mistershell_credential" "test" {
  id = mistershell_credential.test.id
}

data "mistershell_resource" "test" {
  id = mistershell_resource.test.id
}
`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.mistershell_location.test", "id", "mistershell_location.test", "id"),
					resource.TestCheckResourceAttrPair("data.mistershell_credential.test", "id", "mistershell_credential.test", "id"),
					resource.TestCheckResourceAttrPair("data.mistershell_resource.test", "id", "mistershell_resource.test", "id"),
					resource.TestCheckResourceAttr("data.mistershell_resource.test", "engine_id", "ssh"),
				),
			},
			{Config: config, PlanOnly: true, ExpectNonEmptyPlan: false},
		},
	})
}
