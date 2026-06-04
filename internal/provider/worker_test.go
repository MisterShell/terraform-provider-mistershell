package provider_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// ---------------------------------------------------------------------------
// mistershell_worker — create (under a created location), update, import.
//
// token is not returned by GET (only on create/regenerate), and config/
// config_schema are stored-from-config, so all three are excluded from
// ImportStateVerify.
// ---------------------------------------------------------------------------

func TestAccWorker_basic(t *testing.T) {
	testAccPreCheck(t)

	name := acctestPrefix + "worker"
	updated := acctestPrefix + "worker-updated"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			// Create a location + worker referencing it.
			{
				Config: testAccWorkerConfig(name, "first worker", "true"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_worker.test", "name", name),
					resource.TestCheckResourceAttr("mistershell_worker.test", "description", "first worker"),
					resource.TestCheckResourceAttr("mistershell_worker.test", "is_enabled", "true"),
					resource.TestCheckResourceAttr("mistershell_worker.test", "is_default", "false"),
					resource.TestCheckResourceAttrSet("mistershell_worker.test", "id"),
					resource.TestCheckResourceAttrSet("mistershell_worker.test", "status"),
					resource.TestCheckResourceAttrSet("mistershell_worker.test", "token"),
					resource.TestCheckResourceAttrPair("mistershell_worker.test", "location_id", "mistershell_location.test", "id"),
				),
			},
			// Update name + description + disable.
			{
				Config: testAccWorkerConfig(updated, "second worker", "false"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_worker.test", "name", updated),
					resource.TestCheckResourceAttr("mistershell_worker.test", "description", "second worker"),
					resource.TestCheckResourceAttr("mistershell_worker.test", "is_enabled", "false"),
				),
			},
			// Import by integer id. token isn't returned by GET; config and
			// config_schema are stored-from-config and not round-tripped.
			{
				ResourceName:            "mistershell_worker.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"token", "config", "config_schema"},
			},
		},
	})
}

func TestAccWorker_dataSource(t *testing.T) {
	testAccPreCheck(t)

	name := acctestPrefix + "worker-ds"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccWorkerDataSourceConfig(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					// by id
					resource.TestCheckResourceAttrPair("data.mistershell_worker.by_id", "id", "mistershell_worker.test", "id"),
					resource.TestCheckResourceAttr("data.mistershell_worker.by_id", "name", name),
					resource.TestCheckResourceAttr("data.mistershell_worker.by_id", "is_default", "false"),
					resource.TestCheckResourceAttrPair("data.mistershell_worker.by_id", "location_id", "mistershell_location.test", "id"),
					// by name
					resource.TestCheckResourceAttrPair("data.mistershell_worker.by_name", "id", "mistershell_worker.test", "id"),
					resource.TestCheckResourceAttr("data.mistershell_worker.by_name", "name", name),
				),
			},
		},
	})
}

// ---------------------------------------------------------------------------
// Config builders
// ---------------------------------------------------------------------------

func testAccWorkerConfig(name, description, isEnabled string) string {
	return fmt.Sprintf(`
resource "mistershell_location" "test" {
  name      = %q
  kind      = "geo"
  parent_id = 1
}

resource "mistershell_worker" "test" {
  name        = %q
  description = %q
  location_id = mistershell_location.test.id
  is_enabled  = %s

  config = jsonencode({
    log_level = "info"
  })
}
`, acctestPrefix+"worker-loc", name, description, isEnabled)
}

func testAccWorkerDataSourceConfig(name string) string {
	return fmt.Sprintf(`
resource "mistershell_location" "test" {
  name      = %q
  kind      = "geo"
  parent_id = 1
}

resource "mistershell_worker" "test" {
  name        = %q
  location_id = mistershell_location.test.id

  config = jsonencode({
    log_level = "info"
  })
}

data "mistershell_worker" "by_id" {
  id = mistershell_worker.test.id
}

data "mistershell_worker" "by_name" {
  name = mistershell_worker.test.name
}
`, acctestPrefix+"worker-ds-loc", name)
}
