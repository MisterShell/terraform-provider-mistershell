package provider_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func testAccUserBasicConfig(email, first, last, password string, active bool) string {
	return fmt.Sprintf(`
resource "mistershell_user" "test" {
  email      = %q
  first_name = %q
  last_name  = %q
  password   = %q
  is_active  = %t
}
`, email, first, last, password, active)
}

func TestAccUser_basic(t *testing.T) {
	testAccPreCheck(t)

	email := acctestPrefix + "user@example.com"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			// Create.
			{
				Config: testAccUserBasicConfig(email, "First", "Last", "initial-password", true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_user.test", "email", email),
					resource.TestCheckResourceAttr("mistershell_user.test", "first_name", "First"),
					resource.TestCheckResourceAttr("mistershell_user.test", "last_name", "Last"),
					resource.TestCheckResourceAttr("mistershell_user.test", "auth_type", "LOCAL"),
					resource.TestCheckResourceAttr("mistershell_user.test", "is_active", "true"),
					resource.TestCheckResourceAttr("mistershell_user.test", "roles.#", "0"),
					resource.TestCheckResourceAttrSet("mistershell_user.test", "id"),
					resource.TestCheckResourceAttrSet("mistershell_user.test", "created_at"),
				),
			},
			// Update names + is_active + rotate password.
			{
				Config: testAccUserBasicConfig(email, "Firsty", "Lasty", "rotated-password", false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_user.test", "first_name", "Firsty"),
					resource.TestCheckResourceAttr("mistershell_user.test", "last_name", "Lasty"),
					resource.TestCheckResourceAttr("mistershell_user.test", "is_active", "false"),
				),
			},
			// Import (password is not returned by the API, so ignore it on verify).
			{
				ResourceName:            "mistershell_user.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"password"},
			},
		},
	})
}

func testAccUserRoleConfig(email, roleName string) string {
	return fmt.Sprintf(`
resource "mistershell_user" "test" {
  email      = %q
  first_name = "Role"
  last_name  = "Tester"
  password   = "role-tester-pass"
}

resource "mistershell_role" "test" {
  name        = %q
  permissions = ["app.tags.read"]
}

resource "mistershell_user_role" "test" {
  user_id = mistershell_user.test.id
  role_id = mistershell_role.test.id
}

data "mistershell_user" "by_email" {
  email      = mistershell_user.test.email
  depends_on = [mistershell_user_role.test]
}
`, email, roleName)
}

func TestAccUserRole_basic(t *testing.T) {
	testAccPreCheck(t)

	email := acctestPrefix + "role-user@example.com"
	roleName := acctestPrefix + "ur-role"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			// Assign role; verify edge + read-back on the user data source.
			{
				Config: testAccUserRoleConfig(email, roleName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("mistershell_user_role.test", "user_id", "mistershell_user.test", "id"),
					resource.TestCheckResourceAttrPair("mistershell_user_role.test", "role_id", "mistershell_role.test", "id"),
					resource.TestCheckResourceAttr("mistershell_user_role.test", "role_name", roleName),
					resource.TestCheckResourceAttrSet("mistershell_user_role.test", "id"),
					// Data source sees the assigned role.
					resource.TestCheckResourceAttr("data.mistershell_user.by_email", "roles.#", "1"),
					resource.TestCheckResourceAttr("data.mistershell_user.by_email", "roles.0.name", roleName),
				),
			},
			// Import the edge as "<user_id>:<role_id>".
			{
				ResourceName:      "mistershell_user_role.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}
