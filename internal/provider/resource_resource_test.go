package provider_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccNetworkResource_basic(t *testing.T) {
	testAccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create location + credential + resource
			{
				Config: testAccNetworkResourceConfig("tf-acc-res", "linux", "TF-ACC-SN-001"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_resource.test", "name", "tf-acc-res"),
					resource.TestCheckResourceAttr("mistershell_resource.test", "resource_type", "linux"),
					resource.TestCheckResourceAttr("mistershell_resource.test", "external_id", "TF-ACC-SN-001"),
					resource.TestCheckResourceAttr("mistershell_resource.test", "is_enabled", "true"),
					resource.TestCheckResourceAttrSet("mistershell_resource.test", "id"),
					resource.TestCheckResourceAttrSet("mistershell_resource.test", "connector_id"),
					resource.TestCheckResourceAttrSet("mistershell_resource.test", "status"),
					resource.TestCheckResourceAttrPair("mistershell_resource.test", "location_id", "mistershell_location.test", "id"),
					resource.TestCheckResourceAttrPair("mistershell_resource.test", "credential_id", "mistershell_credential.test", "id"),
				),
			},
			// Update name
			{
				Config: testAccNetworkResourceConfig("tf-acc-res-updated", "linux", "TF-ACC-SN-001"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_resource.test", "name", "tf-acc-res-updated"),
				),
			},
			// Import
			{
				ResourceName:            "mistershell_resource.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"connector_data"},
			},
		},
	})
}

func TestAccNetworkResource_idempotency(t *testing.T) {
	testAccPreCheck(t)

	config := testAccNetworkResourceConfig("tf-acc-res-idem", "linux", "TF-ACC-SN-002")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
			},
			// Re-apply — no changes expected
			{
				Config:             config,
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
		},
	})
}

func TestAccNetworkResource_typeForceNew(t *testing.T) {
	testAccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccNetworkResourceConfig("tf-acc-res-type", "linux", "TF-ACC-SN-003"),
			},
			// Change resource_type — should force replacement
			{
				Config: testAccNetworkResourceConfig("tf-acc-res-type", "generic_ssh", "TF-ACC-SN-003"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_resource.test", "resource_type", "generic_ssh"),
				),
			},
		},
	})
}

func testAccNetworkResourceConfig(name, resourceType, externalID string) string {
	return `
resource "mistershell_location" "test" {
  name      = "tf-acc-res-loc"
  kind      = "geo"
  parent_id = 1
}

resource "mistershell_credential" "test" {
  name            = "tf-acc-res-cred"
  credential_type = "ssh_password"

  credential_data = jsonencode({
    username = "testuser"
    password = "testpass123"
  })
}

resource "mistershell_resource" "test" {
  name          = "` + name + `"
  resource_type = "` + resourceType + `"
  external_id   = "` + externalID + `"
  location_id   = mistershell_location.test.id
  credential_id = mistershell_credential.test.id

  connector_data = jsonencode({
    host = "192.168.99.99"
    port = 22
  })
}
`
}
