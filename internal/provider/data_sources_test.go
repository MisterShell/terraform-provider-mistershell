package provider_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// ---------------------------------------------------------------------------
// Location data source
// ---------------------------------------------------------------------------

func TestAccLocationDataSource_byID(t *testing.T) {
	testAccPreCheck(t)
	name := acctestPrefix + "ds-loc-byid"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			{
				// Self-contained: create a location, then read it back by id.
				// (Avoids assuming any pre-existing location's id or name.)
				Config: fmt.Sprintf(`
resource "mistershell_location" "test" {
  name      = %q
  kind      = "geo"
  parent_id = 1
}

data "mistershell_location" "by_id" {
  id = mistershell_location.test.id
}
`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.mistershell_location.by_id", "id", "mistershell_location.test", "id"),
					resource.TestCheckResourceAttr("data.mistershell_location.by_id", "name", name),
					resource.TestCheckResourceAttr("data.mistershell_location.by_id", "kind", "geo"),
					resource.TestCheckResourceAttrSet("data.mistershell_location.by_id", "created_at"),
				),
			},
		},
	})
}

func TestAccLocationDataSource_byName(t *testing.T) {
	testAccPreCheck(t)
	name := acctestPrefix + "ds-loc"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			{
				// Create a location, then look it up by name
				Config: fmt.Sprintf(`
resource "mistershell_location" "test" {
  name      = %q
  kind      = "geo"
  description = "initial description"
  parent_id = 1
}

data "mistershell_location" "lookup" {
  name      = mistershell_location.test.name
  kind      = "geo"
  parent_id = 1
}
`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.mistershell_location.lookup", "id", "mistershell_location.test", "id"),
					resource.TestCheckResourceAttr("data.mistershell_location.lookup", "kind", "geo"),
					resource.TestCheckResourceAttr("data.mistershell_location.lookup", "description", "initial description"),
				),
			},
			{
				Config: fmt.Sprintf(`
resource "mistershell_location" "test" {
  name        = %q
  kind        = "geo"
  description = "updated description"
  parent_id   = 1
}

data "mistershell_location" "lookup" {
  name      = mistershell_location.test.name
  kind      = "geo"
  parent_id = 1
}
`, name),
				Check: resource.TestCheckResourceAttr("data.mistershell_location.lookup", "description", "updated description"),
			},
		},
	})
}

// ---------------------------------------------------------------------------
// Resource data source
// ---------------------------------------------------------------------------

func TestAccNetworkResourceDataSource_byID(t *testing.T) {
	testAccSSHResourcePreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccSSHResourceConfig(acctestPrefix+"ds-res", "") + `
data "mistershell_resource" "lookup" {
  id = mistershell_resource.test.id
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.mistershell_resource.lookup", "name", acctestPrefix+"ds-res"),
					resource.TestCheckResourceAttr("data.mistershell_resource.lookup", "engine_id", "ssh"),
					resource.TestCheckResourceAttrSet("data.mistershell_resource.lookup", "connector_id"),
				),
			},
		},
	})
}

func TestAccNetworkResourceDataSource_byName(t *testing.T) {
	testAccSSHResourcePreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccSSHResourceConfig(acctestPrefix+"ds-res-name", "") + `
data "mistershell_resource" "lookup" {
  name          = mistershell_resource.test.name
  resource_type = "linux"
  location_id   = mistershell_location.test.id
  status        = mistershell_resource.test.status
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.mistershell_resource.lookup", "id", "mistershell_resource.test", "id"),
					resource.TestCheckResourceAttr("data.mistershell_resource.lookup", "engine_id", "ssh"),
					resource.TestCheckResourceAttrSet("data.mistershell_resource.lookup", "connector_data.%"),
				),
			},
			{
				Config: testAccSSHResourceConfig(acctestPrefix+"ds-res-name-updated", "") + `
data "mistershell_resource" "lookup" {
  name          = mistershell_resource.test.name
  resource_type = "linux"
  location_id   = mistershell_location.test.id
  status        = mistershell_resource.test.status
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.mistershell_resource.lookup", "id", "mistershell_resource.test", "id"),
					resource.TestCheckResourceAttr("data.mistershell_resource.lookup", "name", acctestPrefix+"ds-res-name-updated"),
					resource.TestCheckResourceAttr("data.mistershell_resource.lookup", "resource_type", "linux"),
				),
			},
		},
	})
}

// ---------------------------------------------------------------------------
// Credential data source
// ---------------------------------------------------------------------------

func TestAccCredentialDataSource_byID(t *testing.T) {
	testAccPreCheck(t)
	name := acctestPrefix + "ds-cred-id"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "mistershell_credential" "test" {
  name            = %q
  description     = "initial description"
  credential_type = "username_password"
  credential_data = {
    username = "testuser"
    password = "testpass123"
  }
}

data "mistershell_credential" "lookup" {
  id = mistershell_credential.test.id
}
`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.mistershell_credential.lookup", "name", name),
					resource.TestCheckResourceAttr("data.mistershell_credential.lookup", "credential_type", "username_password"),
					resource.TestCheckResourceAttr("data.mistershell_credential.lookup", "description", "initial description"),
				),
			},
			{
				Config: fmt.Sprintf(`
resource "mistershell_credential" "test" {
  name            = %q
  description     = "updated description"
  credential_type = "username_password"
  credential_data = {
    username = "testuser"
    password = "testpass123"
  }
}

data "mistershell_credential" "lookup" {
  id = mistershell_credential.test.id
}
`, name),
				Check: resource.TestCheckResourceAttr("data.mistershell_credential.lookup", "description", "updated description"),
			},
		},
	})
}

func TestAccCredentialDataSource_byName(t *testing.T) {
	testAccPreCheck(t)
	name := acctestPrefix + "ds-cred-name"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "mistershell_credential" "test" {
  name            = %q
  credential_type = "username_password"
  credential_data = {
    username = "testuser"
    password = "testpass123"
  }
}

data "mistershell_credential" "lookup" {
  name = mistershell_credential.test.name
}
`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.mistershell_credential.lookup", "id", "mistershell_credential.test", "id"),
					resource.TestCheckResourceAttr("data.mistershell_credential.lookup", "credential_type", "username_password"),
					resource.TestCheckResourceAttrSet("data.mistershell_credential.lookup", "credential_data.%"),
				),
			},
		},
	})
}

func TestAccCredentialDataSource_byType(t *testing.T) {
	testAccPreCheck(t)
	name := acctestPrefix + "ds-cred-type"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "mistershell_credential" "test" {
  name            = %q
  credential_type = "kubeconfig"
  credential_data = {
    kubeconfig = "apiVersion: v1\nkind: Config"
  }
}

data "mistershell_credential" "lookup" {
  name            = mistershell_credential.test.name
  credential_type = "kubeconfig"
}
`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.mistershell_credential.lookup", "id", "mistershell_credential.test", "id"),
					resource.TestCheckResourceAttr("data.mistershell_credential.lookup", "credential_type", "kubeconfig"),
				),
			},
		},
	})
}
