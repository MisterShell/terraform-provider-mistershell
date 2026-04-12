package provider_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccCredentialResource_basic(t *testing.T) {
	testAccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create
			{
				Config: testAccCredentialSSHPasswordConfig("tf-acc-cred", "Test credential"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_credential.test", "name", "tf-acc-cred"),
					resource.TestCheckResourceAttr("mistershell_credential.test", "credential_type", "ssh_password"),
					resource.TestCheckResourceAttr("mistershell_credential.test", "description", "Test credential"),
					resource.TestCheckResourceAttr("mistershell_credential.test", "requires_user_mapping", "false"),
					resource.TestCheckResourceAttrSet("mistershell_credential.test", "id"),
					resource.TestCheckResourceAttrSet("mistershell_credential.test", "created_at"),
				),
			},
			// Update description
			{
				Config: testAccCredentialSSHPasswordConfig("tf-acc-cred", "Updated description"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_credential.test", "description", "Updated description"),
				),
			},
			// Import — credential_data won't match (masked by API), so skip verify for it
			{
				ResourceName:            "mistershell_credential.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"credential_data"},
			},
		},
	})
}

func TestAccCredentialResource_idempotency(t *testing.T) {
	testAccPreCheck(t)

	config := testAccCredentialSSHPasswordConfig("tf-acc-cred-idem", "Idempotency test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
			},
			// Re-apply same config — should produce no changes
			{
				Config:             config,
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
		},
	})
}

func TestAccCredentialResource_typeChange(t *testing.T) {
	testAccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccCredentialSSHPasswordConfig("tf-acc-cred-type", "Before type change"),
			},
			// Change credential_type — should force replacement
			{
				Config: testAccCredentialSSHKeyConfig("tf-acc-cred-type", "After type change"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_credential.test", "credential_type", "ssh_key"),
				),
			},
		},
	})
}

func testAccCredentialSSHPasswordConfig(name, description string) string {
	return `
resource "mistershell_credential" "test" {
  name            = "` + name + `"
  credential_type = "ssh_password"
  description     = "` + description + `"

  credential_data = jsonencode({
    username = "testuser"
    password = "testpass123"
  })
}
`
}

func testAccCredentialSSHKeyConfig(name, description string) string {
	return `
resource "mistershell_credential" "test" {
  name            = "` + name + `"
  credential_type = "ssh_key"
  description     = "` + description + `"

  credential_data = jsonencode({
    username    = "testuser"
    private_key = "-----BEGIN OPENSSH PRIVATE KEY-----\ntest\n-----END OPENSSH PRIVATE KEY-----"
  })
}
`
}
