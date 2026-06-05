package provider_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// resource_tags_test.go exercises the resource-tagging features:
//   - mistershell_resource.tag_ids (exclusive, PUT-managed) + computed tags
//   - mistershell_resource data source computed tags
//   - mistershell_tags data source (list + optional search)
//
// Every object name is prefixed with acctestPrefix for isolation, locations
// hang under parent_id = 1, and each resource gets an ssh_password credential.

// testAccTagsPreamble builds the shared location + ssh_password credential that
// every resource in this file depends on. The names are acctestPrefix-scoped so
// the generic CheckDestroy / sweepers find them.
func testAccTagsPreamble() string {
	return `
resource "mistershell_location" "test" {
  name      = "` + acctestPrefix + `tags-loc"
  kind      = "geo"
  parent_id = 1
}

resource "mistershell_credential" "test" {
  name            = "` + acctestPrefix + `tags-cred"
  credential_type = "ssh_password"

  credential_data = jsonencode({
    username = "testuser"
    password = "testpass123"
  })
}
`
}

// testAccTwoTags returns two standalone tag resources named
// acctestPrefix+"tag-a" and acctestPrefix+"tag-b".
func testAccTwoTags() string {
	return `
resource "mistershell_tag" "a" {
  name        = "` + acctestPrefix + `tag-a"
  color       = "blue"
  description = "tag a"
}

resource "mistershell_tag" "b" {
  name        = "` + acctestPrefix + `tag-b"
  color       = "green"
  description = "tag b"
}
`
}

// testAccResourceTagsConfig builds the preamble + two tags + a resource whose
// tag_ids is whatever HCL expression the caller passes (e.g.
// "[mistershell_tag.a.id, mistershell_tag.b.id]" or "[]").
func testAccResourceTagsConfig(externalID, tagIDsExpr string) string {
	return testAccTagsPreamble() + testAccTwoTags() + `
resource "mistershell_resource" "test" {
  name          = "` + acctestPrefix + `tag-res"
  resource_type = "linux"
  external_id   = "` + externalID + `"
  location_id   = mistershell_location.test.id
  credential_id = mistershell_credential.test.id

  tag_ids = ` + tagIDsExpr + `

  connector_data = jsonencode({
    host = "192.168.99.99"
    port = 22
  })
}
`
}

// testAccResourceUnmanagedTagsConfig builds the preamble + a resource with NO
// tag_ids attribute at all (unmanaged tags).
func testAccResourceUnmanagedTagsConfig(externalID string) string {
	return testAccTagsPreamble() + `
resource "mistershell_resource" "test" {
  name          = "` + acctestPrefix + `tag-res-unmanaged"
  resource_type = "linux"
  external_id   = "` + externalID + `"
  location_id   = mistershell_location.test.id
  credential_id = mistershell_credential.test.id

  connector_data = jsonencode({
    host = "192.168.99.99"
    port = 22
  })
}
`
}

func TestAccResourceTags_basic(t *testing.T) {
	testAccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			// Step 1: two tags assigned.
			{
				Config: testAccResourceTagsConfig(acctestPrefix+"TAG-SN-001",
					"[mistershell_tag.a.id, mistershell_tag.b.id]"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_resource.test", "tag_ids.#", "2"),
					resource.TestCheckResourceAttr("mistershell_resource.test", "tags.#", "2"),
					// Tie tag_ids membership to the actual tag ids.
					resource.TestCheckTypeSetElemAttrPair(
						"mistershell_resource.test", "tag_ids.*", "mistershell_tag.a", "id"),
					resource.TestCheckTypeSetElemAttrPair(
						"mistershell_resource.test", "tag_ids.*", "mistershell_tag.b", "id"),
				),
			},
			// Step 2: drop one — exclusive removal leaves exactly one.
			{
				Config: testAccResourceTagsConfig(acctestPrefix+"TAG-SN-001",
					"[mistershell_tag.a.id]"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_resource.test", "tag_ids.#", "1"),
					resource.TestCheckResourceAttr("mistershell_resource.test", "tags.#", "1"),
					resource.TestCheckTypeSetElemAttrPair(
						"mistershell_resource.test", "tag_ids.*", "mistershell_tag.a", "id"),
				),
			},
			// Step 3: clear all.
			{
				Config: testAccResourceTagsConfig(acctestPrefix+"TAG-SN-001", "[]"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_resource.test", "tag_ids.#", "0"),
					resource.TestCheckResourceAttr("mistershell_resource.test", "tags.#", "0"),
				),
			},
			// Step 4: import. tag_ids is unmanaged/null on import; tags and
			// connector_data are not round-tripped.
			{
				ResourceName:            "mistershell_resource.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"tag_ids", "tags", "connector_data"},
			},
		},
	})
}

func TestAccResourceTags_unmanaged(t *testing.T) {
	testAccPreCheck(t)

	config := testAccResourceUnmanagedTagsConfig(acctestPrefix + "TAG-SN-002")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("mistershell_resource.test", "id"),
					resource.TestCheckResourceAttr("mistershell_resource.test", "tags.#", "0"),
				),
			},
			// Re-apply identical config — omitting tag_ids must not drift.
			{
				Config:             config,
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
		},
	})
}

func TestAccTagsDataSource_all(t *testing.T) {
	testAccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			// Create the two tags first so they exist when the data source reads.
			{
				Config: testAccTwoTags(),
			},
			{
				Config: testAccTwoTags() + `
data "mistershell_tags" "all" {}

data "mistershell_tags" "filtered" {
  search = "` + acctestPrefix + `"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					// The prefix-filtered data source matches exactly our two tags.
					resource.TestCheckResourceAttr("data.mistershell_tags.filtered", "tags.#", "2"),
					// The unfiltered list returns at least our two tags.
					resource.TestCheckResourceAttrSet("data.mistershell_tags.all", "tags.#"),
				),
			},
		},
	})
}

func TestAccResourceDataSource_tags(t *testing.T) {
	testAccPreCheck(t)

	config := testAccTagsPreamble() + `
resource "mistershell_tag" "a" {
  name        = "` + acctestPrefix + `ds-tag"
  color       = "blue"
  description = "ds tag"
}

resource "mistershell_resource" "test" {
  name          = "` + acctestPrefix + `ds-tag-res"
  resource_type = "linux"
  external_id   = "` + acctestPrefix + `TAG-SN-003"
  location_id   = mistershell_location.test.id
  credential_id = mistershell_credential.test.id

  tag_ids = [mistershell_tag.a.id]

  connector_data = jsonencode({
    host = "192.168.99.99"
    port = 22
  })
}

data "mistershell_resource" "by_id" {
  id = mistershell_resource.test.id
}
`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.mistershell_resource.by_id", "tags.#", "1"),
					resource.TestCheckResourceAttr("data.mistershell_resource.by_id", "tags.0.name", acctestPrefix+"ds-tag"),
				),
			},
		},
	})
}
