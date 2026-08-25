package provider_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccCredentialResource_basic(t *testing.T) {
	testAccPreCheck(t)
	name := acctestPrefix + "cred"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			// Create
			{
				Config: testAccCredentialUsernamePasswordConfig(name, "Test credential"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_credential.test", "name", name),
					resource.TestCheckResourceAttr("mistershell_credential.test", "credential_type", "username_password"),
					resource.TestCheckResourceAttr("mistershell_credential.test", "description", "Test credential"),
					resource.TestCheckResourceAttr("mistershell_credential.test", "requires_user_mapping", "false"),
					resource.TestCheckResourceAttrSet("mistershell_credential.test", "id"),
					resource.TestCheckResourceAttrSet("mistershell_credential.test", "created_at"),
				),
			},
			// Update description
			{
				Config: testAccCredentialUsernamePasswordConfig(name, "Updated description"),
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

	config := testAccCredentialUsernamePasswordConfig(acctestPrefix+"cred-idem", "Idempotency test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
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
	name := acctestPrefix + "cred-type"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccCredentialUsernamePasswordConfig(name, "Before type change"),
			},
			// Change credential_type — should force replacement
			{
				Config: testAccCredentialSSHKeyConfig(name, "After type change"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_credential.test", "credential_type", "ssh_key"),
				),
			},
		},
	})
}

func TestAccCredentialResource_usernamePasswordWithDomain(t *testing.T) {
	testAccPreCheck(t)
	name := acctestPrefix + "cred-rdp"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccCredentialUsernamePasswordDomainConfig(name, "RDP credential"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_credential.test", "name", name),
					resource.TestCheckResourceAttr("mistershell_credential.test", "credential_type", "username_password"),
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

func TestAccCredentialResource_usernamePasswordMinimal(t *testing.T) {
	testAccPreCheck(t)
	name := acctestPrefix + "cred-db"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccCredentialUsernamePasswordMinimalConfig(name, "Database credential"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_credential.test", "name", name),
					resource.TestCheckResourceAttr("mistershell_credential.test", "credential_type", "username_password"),
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

func testAccCredentialUsernamePasswordDomainConfig(name, description string) string {
	return `
resource "mistershell_credential" "test" {
  name            = "` + name + `"
  credential_type = "username_password"
  description     = "` + description + `"

  credential_data = {
    username = "Administrator"
    domain   = "CORP"
    password = "testpass123"
  }
}
`
}

func testAccCredentialUsernamePasswordMinimalConfig(name, description string) string {
	return `
resource "mistershell_credential" "test" {
  name            = "` + name + `"
  credential_type = "username_password"
  description     = "` + description + `"

  credential_data = {
    username = "app"
    password = "testpass123"
  }
}
`
}

func testAccCredentialUsernamePasswordConfig(name, description string) string {
	return `
resource "mistershell_credential" "test" {
  name            = "` + name + `"
  credential_type = "username_password"
  description     = "` + description + `"

  credential_data = {
    username = "testuser"
    password = "testpass123"
  }
}
`
}

func testAccCredentialSSHKeyConfig(name, description string) string {
	return `
resource "mistershell_credential" "test" {
  name            = "` + name + `"
  credential_type = "ssh_key"
  description     = "` + description + `"

  credential_data = {
    username    = "testuser"
    private_key = "-----BEGIN OPENSSH PRIVATE KEY-----\ntest\n-----END OPENSSH PRIVATE KEY-----"
  }
}
`
}
