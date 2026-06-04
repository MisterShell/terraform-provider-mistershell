package provider_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// Wave 1 acceptance tests: mistershell_tag / mistershell_role resources and
// their data sources, plus the mistershell_permissions data source. All names
// are prefixed with acctestPrefix so reruns/parallel runs do not collide and
// the sweepers can target them. Every case uses CheckDestroy to prove deletion.
//
// Permission names used below (app.tags.read, app.resources.read, app.tags.write)
// were taken from the live registry: GET {MISTERSHELL_URL}/api/v1/permissions/
// (Bearer token). They are real, non-superuser names and avoid the wildcard
// '*.*.*' so the superuser-exclusivity rules are never triggered.

// ---------------------------------------------------------------------------
// mistershell_tag — basic CRUD + import
// ---------------------------------------------------------------------------

func TestAccTag_basic(t *testing.T) {
	testAccPreCheck(t)

	name := acctestPrefix + "tag-basic"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			// Create with name, color, description.
			{
				Config: testAccTagBasicConfig(name, "blue", "initial description"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_tag.test", "name", name),
					resource.TestCheckResourceAttr("mistershell_tag.test", "color", "blue"),
					resource.TestCheckResourceAttr("mistershell_tag.test", "description", "initial description"),
					resource.TestCheckResourceAttrSet("mistershell_tag.test", "id"),
					resource.TestCheckResourceAttrSet("mistershell_tag.test", "created_at"),
				),
			},
			// Update color + description.
			{
				Config: testAccTagBasicConfig(name, "green", "updated description"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_tag.test", "name", name),
					resource.TestCheckResourceAttr("mistershell_tag.test", "color", "green"),
					resource.TestCheckResourceAttr("mistershell_tag.test", "description", "updated description"),
				),
			},
			// Import.
			{
				ResourceName:      "mistershell_tag.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// ---------------------------------------------------------------------------
// mistershell_tag — resource_ids assignment round-trip + clear
// ---------------------------------------------------------------------------

func TestAccTag_resourceIDs(t *testing.T) {
	testAccPreCheck(t)

	name := acctestPrefix + "tag-assign"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			// Assign the created resource to the tag.
			{
				Config: testAccTagAssignmentConfig(name, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_tag.test", "resource_ids.#", "1"),
					resource.TestCheckTypeSetElemAttrPair(
						"mistershell_tag.test", "resource_ids.*",
						"mistershell_resource.test", "id"),
				),
			},
			// Clear the assignment (empty set).
			{
				Config: testAccTagAssignmentConfig(name, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_tag.test", "resource_ids.#", "0"),
				),
			},
		},
	})
}

// ---------------------------------------------------------------------------
// mistershell_role — basic CRUD + import (with scope_location_ids = [1])
// ---------------------------------------------------------------------------

func TestAccRole_basic(t *testing.T) {
	testAccPreCheck(t)

	name := acctestPrefix + "role-basic"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			// Create with description + location scope [1].
			{
				Config: testAccRoleBasicConfig(name, "initial role", "1"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_role.test", "name", name),
					resource.TestCheckResourceAttr("mistershell_role.test", "description", "initial role"),
					resource.TestCheckResourceAttr("mistershell_role.test", "scope_location_ids.#", "1"),
					resource.TestCheckTypeSetElemAttr("mistershell_role.test", "scope_location_ids.*", "1"),
					resource.TestCheckResourceAttrSet("mistershell_role.test", "id"),
					resource.TestCheckResourceAttrSet("mistershell_role.test", "created_at"),
				),
			},
			// Update description.
			{
				Config: testAccRoleBasicConfig(name, "updated role", "1"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_role.test", "description", "updated role"),
					resource.TestCheckResourceAttr("mistershell_role.test", "scope_location_ids.#", "1"),
				),
			},
			// Import.
			{
				ResourceName:      "mistershell_role.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// ---------------------------------------------------------------------------
// mistershell_role — permission membership round-trip + reconciliation
// ---------------------------------------------------------------------------

func TestAccRole_permissions(t *testing.T) {
	testAccPreCheck(t)

	name := acctestPrefix + "role-perms"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			// Assign two real permissions.
			{
				Config: testAccRolePermissionsConfig(name, `["app.tags.read", "app.resources.read"]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_role.test", "permissions.#", "2"),
					resource.TestCheckTypeSetElemAttr("mistershell_role.test", "permissions.*", "app.tags.read"),
					resource.TestCheckTypeSetElemAttr("mistershell_role.test", "permissions.*", "app.resources.read"),
				),
			},
			// Reconcile: remove app.resources.read, add app.tags.write (keep app.tags.read).
			{
				Config: testAccRolePermissionsConfig(name, `["app.tags.read", "app.tags.write"]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_role.test", "permissions.#", "2"),
					resource.TestCheckTypeSetElemAttr("mistershell_role.test", "permissions.*", "app.tags.read"),
					resource.TestCheckTypeSetElemAttr("mistershell_role.test", "permissions.*", "app.tags.write"),
				),
			},
		},
	})
}

// ---------------------------------------------------------------------------
// Data sources: tag (by id / by name), role (by id / by name), permissions
// ---------------------------------------------------------------------------

func TestAccTag_dataSource(t *testing.T) {
	testAccPreCheck(t)

	name := acctestPrefix + "tag-ds"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccTagDataSourceConfig(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					// by id
					resource.TestCheckResourceAttrPair("data.mistershell_tag.by_id", "id", "mistershell_tag.test", "id"),
					resource.TestCheckResourceAttr("data.mistershell_tag.by_id", "name", name),
					resource.TestCheckResourceAttr("data.mistershell_tag.by_id", "color", "purple"),
					resource.TestCheckResourceAttrSet("data.mistershell_tag.by_id", "created_at"),
					// by name
					resource.TestCheckResourceAttrPair("data.mistershell_tag.by_name", "id", "mistershell_tag.test", "id"),
					resource.TestCheckResourceAttr("data.mistershell_tag.by_name", "color", "purple"),
				),
			},
		},
	})
}

func TestAccRole_dataSource(t *testing.T) {
	testAccPreCheck(t)

	name := acctestPrefix + "role-ds"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccRoleDataSourceConfig(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					// by id
					resource.TestCheckResourceAttrPair("data.mistershell_role.by_id", "id", "mistershell_role.test", "id"),
					resource.TestCheckResourceAttr("data.mistershell_role.by_id", "name", name),
					resource.TestCheckResourceAttr("data.mistershell_role.by_id", "description", "ds role"),
					resource.TestCheckTypeSetElemAttr("data.mistershell_role.by_id", "permissions.*", "app.tags.read"),
					// by name
					resource.TestCheckResourceAttrPair("data.mistershell_role.by_name", "id", "mistershell_role.test", "id"),
					resource.TestCheckTypeSetElemAttr("data.mistershell_role.by_name", "permissions.*", "app.tags.read"),
				),
			},
		},
	})
}

func TestAccPermissions_dataSource(t *testing.T) {
	testAccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccPermissionsDataSourceConfig(),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Non-empty list.
					resource.TestCheckResourceAttrSet("data.mistershell_permissions.all", "permissions.#"),
					resource.TestCheckResourceAttr("data.mistershell_permissions.all", "id", "permissions"),
					// modules set contains "tags".
					resource.TestCheckTypeSetElemAttr("data.mistershell_permissions.all", "modules.*", "tags"),
					// Filtered-by-module list contains one of the names we use.
					resource.TestCheckTypeSetElemNestedAttrs("data.mistershell_permissions.tags", "permissions.*", map[string]string{
						"name": "app.tags.read",
					}),
				),
			},
		},
	})
}

// ---------------------------------------------------------------------------
// Config builders
// ---------------------------------------------------------------------------

func testAccTagBasicConfig(name, color, description string) string {
	return fmt.Sprintf(`
resource "mistershell_tag" "test" {
  name        = %q
  color       = %q
  description = %q
}
`, name, color, description)
}

// testAccTagAssignmentConfig builds a location + credential + resource, then a
// tag that either references the resource id or has an empty resource_ids set.
func testAccTagAssignmentConfig(name string, assigned bool) string {
	resourceIDs := "[]"
	if assigned {
		resourceIDs = "[mistershell_resource.test.id]"
	}
	return fmt.Sprintf(`
resource "mistershell_location" "test" {
  name      = %[1]q
  kind      = "geo"
  parent_id = 1
}

resource "mistershell_credential" "test" {
  name            = %[2]q
  credential_type = "ssh_password"

  credential_data = jsonencode({
    username = "testuser"
    password = "testpass123"
  })
}

resource "mistershell_resource" "test" {
  name          = %[3]q
  resource_type = "linux"
  external_id   = %[4]q
  location_id   = mistershell_location.test.id
  credential_id = mistershell_credential.test.id

  connector_data = jsonencode({
    host = "192.168.99.99"
    port = 22
  })
}

resource "mistershell_tag" "test" {
  name         = %[5]q
  color        = "orange"
  resource_ids = %[6]s
}
`,
		name+"-loc",
		name+"-cred",
		name+"-res",
		name+"-extid",
		name,
		resourceIDs,
	)
}

func testAccRoleBasicConfig(name, description, scope string) string {
	return fmt.Sprintf(`
resource "mistershell_role" "test" {
  name               = %q
  description        = %q
  scope_location_ids = [%s]
}
`, name, description, scope)
}

func testAccRolePermissionsConfig(name, permissions string) string {
	return fmt.Sprintf(`
resource "mistershell_role" "test" {
  name        = %q
  permissions = %s
}
`, name, permissions)
}

func testAccTagDataSourceConfig(name string) string {
	return fmt.Sprintf(`
resource "mistershell_tag" "test" {
  name  = %[1]q
  color = "purple"
}

data "mistershell_tag" "by_id" {
  id = mistershell_tag.test.id
}

data "mistershell_tag" "by_name" {
  name = mistershell_tag.test.name
}
`, name)
}

func testAccRoleDataSourceConfig(name string) string {
	return fmt.Sprintf(`
resource "mistershell_role" "test" {
  name        = %[1]q
  description = "ds role"
  permissions = ["app.tags.read"]
}

data "mistershell_role" "by_id" {
  id = mistershell_role.test.id
}

data "mistershell_role" "by_name" {
  name = mistershell_role.test.name
}
`, name)
}

func testAccPermissionsDataSourceConfig() string {
	return `
data "mistershell_permissions" "all" {}

data "mistershell_permissions" "tags" {
  module = "tags"
}
`
}
