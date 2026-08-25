package provider_test

import (
	"fmt"
	"os"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func testAccSSHResourcePreCheck(t *testing.T) {
	t.Helper()
	testAccPreCheck(t)
	for _, name := range []string{"MISTERSHELL_ACC_SSH_HOST", "MISTERSHELL_ACC_SSH_USERNAME", "MISTERSHELL_ACC_SSH_PASSWORD"} {
		if os.Getenv(name) == "" {
			t.Skipf("live SSH onboarding test requires %s", name)
		}
	}
	if value := os.Getenv("MISTERSHELL_ACC_SSH_PORT"); value != "" {
		if _, err := strconv.Atoi(value); err != nil {
			t.Fatalf("MISTERSHELL_ACC_SSH_PORT must be an integer: %v", err)
		}
	}
}

func TestAccNetworkResource_automaticSSHOnboarding(t *testing.T) {
	testAccSSHResourcePreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccSSHResourceConfig("tf-acc-auto", ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_resource.test", "engine_id", "ssh"),
					resource.TestCheckResourceAttrSet("mistershell_resource.test", "resource_type"),
					resource.TestCheckResourceAttrSet("mistershell_resource.test", "id"),
					resource.TestCheckResourceAttrSet("mistershell_resource.test", "connector_id"),
				),
			},
			{
				Config:             testAccSSHResourceConfig("tf-acc-auto", ""),
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
			{
				ResourceName:      "mistershell_resource.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"connector_data", "status", "updated_at", "last_connectivity_check",
					"last_connectivity_result", "last_collection_at", "next_collection_at",
				},
			},
		},
	})
}

func TestAccNetworkResource_explicitGenericSSH(t *testing.T) {
	testAccSSHResourcePreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{{
			Config: testAccSSHResourceConfig("tf-acc-generic", "generic_ssh"),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("mistershell_resource.test", "engine_id", "ssh"),
				resource.TestCheckResourceAttr("mistershell_resource.test", "resource_type", "generic_ssh"),
				resource.TestCheckNoResourceAttr("mistershell_resource.test", "external_id"),
			),
		}},
	})
}

func testAccSSHResourceConfig(name, resourceType string) string {
	port, _ := strconv.Atoi(testAccSSHPort())
	typeLine := "  engine_id = \"ssh\""
	if resourceType != "" {
		typeLine = "  resource_type = " + strconv.Quote(resourceType)
	}

	return fmt.Sprintf(`
resource "mistershell_location" "test" {
  name      = %q
  kind      = "geo"
  parent_id = 1
}

resource "mistershell_credential" "test" {
  name            = %q
  credential_type = "username_password"
  credential_data = jsonencode({
    username = %q
    password = %q
  })
}

resource "mistershell_resource" "test" {
  name          = %q
%s
  location_id   = mistershell_location.test.id
  credential_id = mistershell_credential.test.id
  connector_data = jsonencode({
    host = %q
    port = %d
  })
}
`, name+"-location", name+"-credential", os.Getenv("MISTERSHELL_ACC_SSH_USERNAME"), os.Getenv("MISTERSHELL_ACC_SSH_PASSWORD"), name, typeLine, os.Getenv("MISTERSHELL_ACC_SSH_HOST"), port)
}

func testAccSSHPort() string {
	if value := os.Getenv("MISTERSHELL_ACC_SSH_PORT"); value != "" {
		return value
	}
	return "22"
}
