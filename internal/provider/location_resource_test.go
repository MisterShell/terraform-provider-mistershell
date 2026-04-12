package provider_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccLocationResource_basic(t *testing.T) {
	testAccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create a location under Root (id=1)
			{
				Config: testAccLocationConfig("tf-acc-loc", "geo", "Acceptance test location"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_location.test", "name", "tf-acc-loc"),
					resource.TestCheckResourceAttr("mistershell_location.test", "kind", "geo"),
					resource.TestCheckResourceAttr("mistershell_location.test", "description", "Acceptance test location"),
					resource.TestCheckResourceAttr("mistershell_location.test", "parent_id", "1"),
					resource.TestCheckResourceAttrSet("mistershell_location.test", "id"),
					resource.TestCheckResourceAttrSet("mistershell_location.test", "created_at"),
					resource.TestCheckResourceAttrSet("mistershell_location.test", "updated_at"),
				),
			},
			// Update name and description
			{
				Config: testAccLocationConfig("tf-acc-loc-updated", "geo", "Updated description"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_location.test", "name", "tf-acc-loc-updated"),
					resource.TestCheckResourceAttr("mistershell_location.test", "description", "Updated description"),
				),
			},
			// Import
			{
				ResourceName:      "mistershell_location.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccLocationResource_hierarchy(t *testing.T) {
	testAccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccLocationHierarchyConfig(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_location.parent", "name", "tf-acc-parent"),
					resource.TestCheckResourceAttr("mistershell_location.child", "name", "tf-acc-child"),
					resource.TestCheckResourceAttrPair("mistershell_location.child", "parent_id", "mistershell_location.parent", "id"),
				),
			},
		},
	})
}

func TestAccLocationResource_coordinates(t *testing.T) {
	testAccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccLocationWithCoordinatesConfig(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_location.test", "latitude", "47.3769"),
					resource.TestCheckResourceAttr("mistershell_location.test", "longitude", "8.5417"),
				),
			},
		},
	})
}

func testAccLocationConfig(name, kind, description string) string {
	return fmt.Sprintf(`
resource "mistershell_location" "test" {
  name        = %q
  kind        = %q
  description = %q
  parent_id   = 1
}
`, name, kind, description)
}

func testAccLocationHierarchyConfig() string {
	return `
resource "mistershell_location" "parent" {
  name        = "tf-acc-parent"
  kind        = "geo"
  parent_id   = 1
}

resource "mistershell_location" "child" {
  name      = "tf-acc-child"
  kind      = "geo"
  parent_id = mistershell_location.parent.id
}
`
}

func testAccLocationWithCoordinatesConfig() string {
	return `
resource "mistershell_location" "test" {
  name      = "tf-acc-coords"
  kind      = "geo"
  parent_id = 1
  latitude  = 47.3769
  longitude = 8.5417
}
`
}
