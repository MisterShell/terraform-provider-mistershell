package provider_test

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
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

func testAccUserRoleParentsConfig(email, roleName string) string {
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
`, email, roleName)
}

func TestAccUserRole_basic(t *testing.T) {
	testAccPreCheck(t)

	email := acctestPrefix + "role-user@example.com"
	roleName := acctestPrefix + "ur-role"
	var userID, roleID int64

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
					captureUserRoleIDs(&userID, &roleID),
				),
			},
			// Import the edge as "<user_id>:<role_id>".
			{
				ResourceName:      "mistershell_user_role.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Remove only the assignment while retaining both the user and role.
			{
				Config: testAccUserRoleParentsConfig(email, roleName),
				Check:  checkUserRoleAbsent(&userID, &roleID),
			},
		},
	})
}

func TestAccUsersDataSource_allFilteredAndByID(t *testing.T) {
	testAccPreCheck(t)

	emailOne := acctestPrefix + "catalog-one@example.com"
	emailTwo := acctestPrefix + "catalog-two@example.com"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccUsersDataSourceConfig(emailOne, emailTwo, "Catalog"),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkUsersContain("data.mistershell_users.all", map[string]string{emailOne: "Catalog", emailTwo: "Catalog"}),
					resource.TestCheckResourceAttr("data.mistershell_users.filtered", "users.#", "2"),
					checkUsersContain("data.mistershell_users.filtered", map[string]string{emailOne: "Catalog", emailTwo: "Catalog"}),
					resource.TestCheckResourceAttrPair("data.mistershell_user.by_id", "id", "mistershell_user.one", "id"),
					resource.TestCheckResourceAttr("data.mistershell_user.by_id", "email", emailOne),
				),
			},
			{
				Config: testAccUsersDataSourceConfig(emailOne, emailTwo, "Updated"),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkUsersContain("data.mistershell_users.filtered", map[string]string{emailOne: "Updated", emailTwo: "Catalog"}),
					resource.TestCheckResourceAttr("data.mistershell_user.by_id", "first_name", "Updated"),
				),
			},
		},
	})
}

func testAccUsersDataSourceConfig(emailOne, emailTwo, firstNameOne string) string {
	return fmt.Sprintf(`
resource "mistershell_user" "one" {
  email      = %q
  first_name = %q
  last_name  = "One"
  password   = "catalog-one-password"
}

resource "mistershell_user" "two" {
  email      = %q
  first_name = "Catalog"
  last_name  = "Two"
  password   = "catalog-two-password"
}

data "mistershell_users" "all" {
  depends_on = [mistershell_user.one, mistershell_user.two]
}

data "mistershell_users" "filtered" {
  search     = %q
  depends_on = [mistershell_user.one, mistershell_user.two]
}

data "mistershell_user" "by_id" {
  id = mistershell_user.one.id
}
`, emailOne, firstNameOne, emailTwo, acctestPrefix+"catalog-")
}

func checkUsersContain(addr string, expected map[string]string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[addr]
		if !ok {
			return fmt.Errorf("users data source %q not found in state", addr)
		}
		count, err := strconv.Atoi(rs.Primary.Attributes["users.#"])
		if err != nil {
			return fmt.Errorf("parsing %s users count: %w", addr, err)
		}

		found := make(map[string]string, count)
		for i := 0; i < count; i++ {
			email := rs.Primary.Attributes[fmt.Sprintf("users.%d.email", i)]
			found[email] = rs.Primary.Attributes[fmt.Sprintf("users.%d.first_name", i)]
		}
		for email, firstName := range expected {
			if found[email] != firstName {
				return fmt.Errorf("%s missing user %q with first_name %q (found %q)", addr, email, firstName, found[email])
			}
		}
		return nil
	}
}

func captureUserRoleIDs(userID, roleID *int64) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources["mistershell_user_role.test"]
		if !ok {
			return fmt.Errorf("user-role resource not found in state")
		}
		var err error
		*userID, err = strconv.ParseInt(rs.Primary.Attributes["user_id"], 10, 64)
		if err != nil {
			return fmt.Errorf("parsing user_id: %w", err)
		}
		*roleID, err = strconv.ParseInt(rs.Primary.Attributes["role_id"], 10, 64)
		if err != nil {
			return fmt.Errorf("parsing role_id: %w", err)
		}
		return nil
	}
}

func checkUserRoleAbsent(userID, roleID *int64) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		if *userID == 0 || *roleID == 0 {
			return fmt.Errorf("user-role identifiers were not captured")
		}
		user, err := testAccClient().GetUser(context.Background(), *userID)
		if err != nil {
			return fmt.Errorf("reading user %d after role removal: %w", *userID, err)
		}
		for _, role := range user.Roles {
			if role.ID == *roleID {
				return fmt.Errorf("role %d is still assigned to user %d", *roleID, *userID)
			}
		}
		return nil
	}
}
