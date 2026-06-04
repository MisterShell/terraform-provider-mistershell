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

func TestAccCredentialResource_rdpPassword(t *testing.T) {
	testAccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccCredentialRDPPasswordConfig("tf-acc-cred-rdp", "RDP credential"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_credential.test", "name", "tf-acc-cred-rdp"),
					resource.TestCheckResourceAttr("mistershell_credential.test", "credential_type", "rdp_password"),
					resource.TestCheckResourceAttrSet("mistershell_credential.test", "id"),
				),
			},
			{
				ResourceName:            "mistershell_credential.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"credential_data"},
			},
		},
	})
}

func TestAccCredentialResource_dbPassword(t *testing.T) {
	testAccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccCredentialDBPasswordConfig("tf-acc-cred-db", "Database credential"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_credential.test", "name", "tf-acc-cred-db"),
					resource.TestCheckResourceAttr("mistershell_credential.test", "credential_type", "db_password"),
					resource.TestCheckResourceAttrSet("mistershell_credential.test", "id"),
				),
			},
			{
				ResourceName:            "mistershell_credential.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"credential_data"},
			},
		},
	})
}

func testAccCredentialRDPPasswordConfig(name, description string) string {
	return `
resource "mistershell_credential" "test" {
  name            = "` + name + `"
  credential_type = "rdp_password"
  description     = "` + description + `"

  credential_data = jsonencode({
    username = "Administrator"
    domain   = "CORP"
    password = "testpass123"
  })
}
`
}

func testAccCredentialDBPasswordConfig(name, description string) string {
	return `
resource "mistershell_credential" "test" {
  name            = "` + name + `"
  credential_type = "db_password"
  description     = "` + description + `"

  credential_data = jsonencode({
    username = "app"
    password = "testpass123"
  })
}
`
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
