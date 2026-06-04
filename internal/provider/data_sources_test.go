package provider_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// ---------------------------------------------------------------------------
// Location data source
// ---------------------------------------------------------------------------

func TestAccLocationDataSource_byID(t *testing.T) {
	testAccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// Self-contained: create a location, then read it back by id.
				// (Avoids assuming any pre-existing location's id or name.)
				Config: `
resource "mistershell_location" "test" {
  name      = "tf-acc-ds-loc-byid"
  kind      = "geo"
  parent_id = 1
}

data "mistershell_location" "by_id" {
  id = mistershell_location.test.id
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.mistershell_location.by_id", "id", "mistershell_location.test", "id"),
					resource.TestCheckResourceAttr("data.mistershell_location.by_id", "name", "tf-acc-ds-loc-byid"),
					resource.TestCheckResourceAttr("data.mistershell_location.by_id", "kind", "geo"),
					resource.TestCheckResourceAttrSet("data.mistershell_location.by_id", "created_at"),
				),
			},
		},
	})
}

func TestAccLocationDataSource_byName(t *testing.T) {
	testAccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// Create a location, then look it up by name
				Config: `
resource "mistershell_location" "test" {
  name      = "tf-acc-ds-loc"
  kind      = "geo"
  parent_id = 1
}

data "mistershell_location" "lookup" {
  name      = mistershell_location.test.name
  parent_id = 1
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.mistershell_location.lookup", "id", "mistershell_location.test", "id"),
					resource.TestCheckResourceAttr("data.mistershell_location.lookup", "kind", "geo"),
				),
			},
		},
	})
}

// ---------------------------------------------------------------------------
// Resource data source
// ---------------------------------------------------------------------------

func TestAccNetworkResourceDataSource_byID(t *testing.T) {
	testAccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "mistershell_location" "test" {
  name      = "tf-acc-ds-res-loc"
  kind      = "geo"
  parent_id = 1
}

resource "mistershell_credential" "test" {
  name            = "tf-acc-ds-res-cred"
  credential_type = "ssh_password"
  credential_data = jsonencode({
    username = "testuser"
    password = "testpass123"
  })
}

resource "mistershell_resource" "test" {
  name          = "tf-acc-ds-res"
  resource_type = "linux"
  external_id   = "TF-ACC-DS-SN-001"
  location_id   = mistershell_location.test.id
  credential_id = mistershell_credential.test.id
  connector_data = jsonencode({
    host = "192.168.99.98"
    port = 22
  })
}

data "mistershell_resource" "lookup" {
  id = mistershell_resource.test.id
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.mistershell_resource.lookup", "name", "tf-acc-ds-res"),
					resource.TestCheckResourceAttr("data.mistershell_resource.lookup", "resource_type", "linux"),
					resource.TestCheckResourceAttrSet("data.mistershell_resource.lookup", "connector_id"),
				),
			},
		},
	})
}

func TestAccNetworkResourceDataSource_byName(t *testing.T) {
	testAccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "mistershell_location" "test" {
  name      = "tf-acc-ds-res-name-loc"
  kind      = "geo"
  parent_id = 1
}

resource "mistershell_credential" "test" {
  name            = "tf-acc-ds-res-name-cred"
  credential_type = "ssh_password"
  credential_data = jsonencode({
    username = "testuser"
    password = "testpass123"
  })
}

resource "mistershell_resource" "test" {
  name          = "tf-acc-ds-res-name"
  resource_type = "linux"
  external_id   = "TF-ACC-DS-SN-002"
  location_id   = mistershell_location.test.id
  credential_id = mistershell_credential.test.id
  connector_data = jsonencode({
    host = "192.168.99.97"
    port = 22
  })
}

data "mistershell_resource" "lookup" {
  name = mistershell_resource.test.name
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.mistershell_resource.lookup", "id", "mistershell_resource.test", "id"),
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

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "mistershell_credential" "test" {
  name            = "tf-acc-ds-cred-id"
  credential_type = "ssh_password"
  credential_data = jsonencode({
    username = "testuser"
    password = "testpass123"
  })
}

data "mistershell_credential" "lookup" {
  id = mistershell_credential.test.id
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.mistershell_credential.lookup", "name", "tf-acc-ds-cred-id"),
					resource.TestCheckResourceAttr("data.mistershell_credential.lookup", "credential_type", "ssh_password"),
				),
			},
		},
	})
}

func TestAccCredentialDataSource_byName(t *testing.T) {
	testAccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "mistershell_credential" "test" {
  name            = "tf-acc-ds-cred-name"
  credential_type = "ssh_password"
  credential_data = jsonencode({
    username = "testuser"
    password = "testpass123"
  })
}

data "mistershell_credential" "lookup" {
  name = mistershell_credential.test.name
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.mistershell_credential.lookup", "id", "mistershell_credential.test", "id"),
					resource.TestCheckResourceAttr("data.mistershell_credential.lookup", "credential_type", "ssh_password"),
				),
			},
		},
	})
}

func TestAccCredentialDataSource_byType(t *testing.T) {
	testAccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "mistershell_credential" "test" {
  name            = "tf-acc-ds-cred-type"
  credential_type = "kubeconfig"
  credential_data = jsonencode({
    kubeconfig = "apiVersion: v1\nkind: Config"
  })
}

data "mistershell_credential" "lookup" {
  name            = mistershell_credential.test.name
  credential_type = "kubeconfig"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.mistershell_credential.lookup", "id", "mistershell_credential.test", "id"),
					resource.TestCheckResourceAttr("data.mistershell_credential.lookup", "credential_type", "kubeconfig"),
				),
			},
		},
	})
}
