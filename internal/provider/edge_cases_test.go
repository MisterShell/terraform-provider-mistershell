package provider_test

import (
	"context"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"terraform-provider-mistershell/internal/client"
)

// edge_cases_test.go adds negative / error-path, out-of-band-drift, and
// subtle-bug-class acceptance tests for the core entities (location, network
// resource, credential, tag, log_destination) and the resource-tagging feature.
//
// Conventions (shared with the rest of the suite):
//   - Every object name is prefixed with acctestPrefix for isolation.
//   - TestCases that create objects set ProtoV6ProviderFactories +
//     CheckDestroy and call testAccPreCheck(t) first.
//   - Plan-time validator-error steps (PlanOnly + ExpectError) create nothing,
//     so their TestCase omits CheckDestroy.
//   - Locations hang under parent_id = 1; resources are resource_type="linux"
//     with an ssh_password credential and a {host,port} connector_data.

// ---------------------------------------------------------------------------
// Shared config helpers
// ---------------------------------------------------------------------------

// edgePreamble builds the shared location + ssh_password credential that the
// network-resource edge cases depend on. acctestPrefix-scoped so the generic
// CheckDestroy / sweepers find them.
func edgePreamble() string {
	return `
resource "mistershell_location" "test" {
  name      = "` + acctestPrefix + `edge-loc"
  kind      = "geo"
  parent_id = 1
}

resource "mistershell_credential" "test" {
  name            = "` + acctestPrefix + `edge-cred"
  credential_type = "ssh_password"

  credential_data = jsonencode({
    username = "testuser"
    password = "testpass123"
  })
}
`
}

// ---------------------------------------------------------------------------
// P0 — negative / error-path (ExpectError, plan-time validators)
// ---------------------------------------------------------------------------

// TestAccEdge_EnumValidators feeds a single invalid enum value into each
// constrained attribute, with the rest of the config otherwise valid, so the
// ONLY error is the OneOf validator. All steps are PlanOnly => nothing applies
// => no CheckDestroy needed.
func TestAccEdge_EnumValidators(t *testing.T) {
	testAccPreCheck(t)

	oneOf := regexp.MustCompile(`value must be one of`)

	// A resource with an invalid resource_type. Needs a valid location +
	// credential so only resource_type is wrong.
	resourceBadType := edgePreamble() + `
resource "mistershell_resource" "test" {
  name          = "` + acctestPrefix + `edge-badtype"
  resource_type = "not_a_type"
  external_id   = "` + acctestPrefix + `EDGE-BADTYPE"
  location_id   = mistershell_location.test.id
  credential_id = mistershell_credential.test.id

  connector_data = jsonencode({
    host = "192.168.99.99"
    port = 22
  })
}
`

	credBadType := `
resource "mistershell_credential" "bad" {
  name            = "` + acctestPrefix + `edge-badcred"
  credential_type = "not_a_cred"

  credential_data = jsonencode({
    username = "testuser"
    password = "testpass123"
  })
}
`

	logdestBadType := `
resource "mistershell_log_destination" "bad" {
  name         = "` + acctestPrefix + `edge-ld-badtype"
  type         = "not_a_type"
  streams      = ["security"]
  min_severity = "info"

  config = jsonencode({
    host = "192.168.99.99"
    port = 514
  })
}
`

	logdestBadSeverity := `
resource "mistershell_log_destination" "bad" {
  name         = "` + acctestPrefix + `edge-ld-badsev"
  type         = "syslog"
  streams      = ["security"]
  min_severity = "nope"

  config = jsonencode({
    host = "192.168.99.99"
    port = 514
  })
}
`

	logdestBadStream := `
resource "mistershell_log_destination" "bad" {
  name         = "` + acctestPrefix + `edge-ld-badstream"
  type         = "syslog"
  streams      = ["bogus"]
  min_severity = "info"

  config = jsonencode({
    host = "192.168.99.99"
    port = 514
  })
}
`

	// ACL with an invalid pattern type (the enum lives on patterns[].type).
	aclBadPatternType := `
resource "mistershell_session_policy_acl" "bad" {
  name = "` + acctestPrefix + `edge-acl-bad"

  patterns = [
    {
      pattern = "rm -rf*"
      type    = "bogus"
    },
  ]
}
`

	// Rule with an invalid action. Minimal required attributes only.
	ruleBadAction := `
resource "mistershell_session_policy_rule" "bad" {
  name   = "` + acctestPrefix + `edge-rule-bad"
  action = "bogus"
}
`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: resourceBadType, PlanOnly: true, ExpectError: oneOf},
			{Config: credBadType, PlanOnly: true, ExpectError: oneOf},
			{Config: logdestBadType, PlanOnly: true, ExpectError: oneOf},
			{Config: logdestBadSeverity, PlanOnly: true, ExpectError: oneOf},
			{Config: logdestBadStream, PlanOnly: true, ExpectError: oneOf},
			{Config: aclBadPatternType, PlanOnly: true, ExpectError: oneOf},
			{Config: ruleBadAction, PlanOnly: true, ExpectError: oneOf},
		},
	})
}

// TestAccEdge_DataSourceResolution covers the two data-source resolution
// failure modes: zero matches and more-than-one match.
func TestAccEdge_DataSourceResolution(t *testing.T) {
	testAccPreCheck(t)

	// --- 0-match: a tag data source for a name that cannot exist. No objects
	// are created, so CheckDestroy is unnecessary.
	t.Run("no_match", func(t *testing.T) {
		noMatch := `
data "mistershell_tag" "missing" {
  name = "` + acctestPrefix + `nonexistent"
}
`
		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			Steps: []resource.TestStep{
				{
					Config:      noMatch,
					ExpectError: regexp.MustCompile(`No matching tag found`),
				},
			},
		})
	})

	// --- >1-match: two resources of the same type under one location, then a
	// resource data source filtering by resource_type only (matches both).
	t.Run("multiple_match", func(t *testing.T) {
		twoResources := edgePreamble() + `
resource "mistershell_resource" "a" {
  name          = "` + acctestPrefix + `edge-dup-a"
  resource_type = "linux"
  external_id   = "` + acctestPrefix + `EDGE-DUP-A"
  location_id   = mistershell_location.test.id
  credential_id = mistershell_credential.test.id

  connector_data = jsonencode({
    host = "192.168.99.99"
    port = 22
  })
}

resource "mistershell_resource" "b" {
  name          = "` + acctestPrefix + `edge-dup-b"
  resource_type = "linux"
  external_id   = "` + acctestPrefix + `EDGE-DUP-B"
  location_id   = mistershell_location.test.id
  credential_id = mistershell_credential.test.id

  connector_data = jsonencode({
    host = "192.168.99.99"
    port = 22
  })
}
`
		withDataSource := twoResources + `
data "mistershell_resource" "ambiguous" {
  resource_type = "linux"

  depends_on = [mistershell_resource.a, mistershell_resource.b]
}
`
		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			CheckDestroy:             testAccCheckAllDestroyed,
			Steps: []resource.TestStep{
				// Create the two resources first.
				{Config: twoResources},
				// Then add the ambiguous data source -> Multiple resources.
				{
					Config:      withDataSource,
					ExpectError: regexp.MustCompile(`Multiple resources`),
				},
			},
		})
	})
}

// TestAccEdge_ImportErrors creates one tag (so the resource type is known to the
// import machinery), then exercises the two import failure modes: a non-integer
// import ID (caught by our import parser) and a well-formed but nonexistent ID
// (caught by the backend 404 on the post-import refresh).
func TestAccEdge_ImportErrors(t *testing.T) {
	testAccPreCheck(t)

	tagConfig := `
resource "mistershell_tag" "test" {
  name  = "` + acctestPrefix + `edge-import-tag"
  color = "blue"
}
`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			{Config: tagConfig},
			// Non-integer import ID -> our parser rejects it.
			{
				Config:        tagConfig,
				ResourceName:  "mistershell_tag.test",
				ImportState:   true,
				ImportStateId: "not-an-int",
				ExpectError:   regexp.MustCompile(`Invalid import ID`),
			},
			// Well-formed but nonexistent ID -> backend 404 on refresh.
			{
				Config:        tagConfig,
				ResourceName:  "mistershell_tag.test",
				ImportState:   true,
				ImportStateId: "999999999",
				ExpectError:   regexp.MustCompile(`(?i)(not found|cannot|404|no .* found)`),
			},
		},
	})
}

// TestAccEdge_BackendConstraints exercises apply-time backend rejections that
// the framework cannot catch at plan time. The regexes are intentionally
// lenient so they match regardless of the exact backend wording.
func TestAccEdge_BackendConstraints(t *testing.T) {
	testAccPreCheck(t)

	// --- windows resource paired with an rdp_password credential. The backend
	// requires SSH credentials for a windows resource, so create should fail.
	// (If this actually succeeds live, flip it to an expected-success test.)
	t.Run("windows_cred_mismatch", func(t *testing.T) {
		mismatch := `
resource "mistershell_location" "test" {
  name      = "` + acctestPrefix + `edge-mismatch-loc"
  kind      = "geo"
  parent_id = 1
}

resource "mistershell_credential" "test" {
  name            = "` + acctestPrefix + `edge-mismatch-cred"
  credential_type = "rdp_password"

  credential_data = jsonencode({
    username = "Administrator"
    password = "testpass123"
  })
}

resource "mistershell_resource" "test" {
  name          = "` + acctestPrefix + `edge-mismatch-res"
  resource_type = "windows"
  external_id   = "` + acctestPrefix + `EDGE-MISMATCH"
  location_id   = mistershell_location.test.id
  credential_id = mistershell_credential.test.id

  connector_data = jsonencode({
    host            = "192.168.99.99"
    port            = 22
    rdp_port        = 3389
    nla_required    = true
    keyboard_layout = "0x0000040C"
  })
}
`
		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			CheckDestroy:             testAccCheckAllDestroyed,
			Steps: []resource.TestStep{
				{
					Config:      mismatch,
					ExpectError: regexp.MustCompile(`(?i)(error creating|invalid|credential|not .*(allow|compat)|status 4\d\d)`),
				},
			},
		})
	})

	// --- two tags with the SAME name. The backend enforces unique tag names, so
	// the second create should conflict.
	t.Run("duplicate_tag_name", func(t *testing.T) {
		dup := `
resource "mistershell_tag" "a" {
  name  = "` + acctestPrefix + `edge-dup-name"
  color = "blue"
}

resource "mistershell_tag" "b" {
  name  = "` + acctestPrefix + `edge-dup-name"
  color = "green"
}
`
		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			CheckDestroy:             testAccCheckAllDestroyed,
			Steps: []resource.TestStep{
				{
					Config:      dup,
					ExpectError: regexp.MustCompile(`(?i)(already exists|409|conflict|error creating tag)`),
				},
			},
		})
	})
}

// ---------------------------------------------------------------------------
// P1 — out-of-band drift (PreConfig mutates server state between steps)
// ---------------------------------------------------------------------------

// edgeFindTagByName returns the id of the tag whose name exactly matches, or
// fails the test. Uses the live client built from the MISTERSHELL_* env.
func edgeFindTagByName(t *testing.T, name string) int64 {
	t.Helper()
	c := testAccClient()
	if c == nil {
		t.Fatal("edgeFindTagByName: MISTERSHELL_URL/MISTERSHELL_API_KEY not set")
	}
	tags, err := c.ListTags(context.Background(), client.TagListFilter{Search: name})
	if err != nil {
		t.Fatalf("listing tags for %q: %v", name, err)
	}
	for _, tg := range tags {
		if tg.Name == name {
			return tg.ID
		}
	}
	t.Fatalf("no tag found with exact name %q", name)
	return 0
}

// TestAccEdge_TagDeletedOutOfBand proves that a tag deleted out-of-band is
// detected on refresh (removed from state) and recreated by the next apply.
func TestAccEdge_TagDeletedOutOfBand(t *testing.T) {
	testAccPreCheck(t)

	name := acctestPrefix + "edge-oob-del"
	config := `
resource "mistershell_tag" "test" {
  name  = "` + name + `"
  color = "blue"
}
`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			// Step 1: create.
			{
				Config: config,
				Check:  resource.TestCheckResourceAttrSet("mistershell_tag.test", "id"),
			},
			// Step 2: delete it server-side, then re-apply the SAME config. The
			// refresh sees the 404, removes it from state, and recreates it.
			{
				PreConfig: func() {
					c := testAccClient()
					if c == nil {
						t.Fatal("PreConfig: client not configured")
					}
					id := edgeFindTagByName(t, name)
					if err := c.DeleteTag(context.Background(), id); err != nil {
						t.Fatalf("out-of-band delete of tag %d: %v", id, err)
					}
				},
				Config: config,
				Check:  resource.TestCheckResourceAttrSet("mistershell_tag.test", "id"),
			},
		},
	})
}

// TestAccEdge_TagMutatedOutOfBand proves that an out-of-band attribute change
// (color) is detected as drift and corrected back to the configured value.
func TestAccEdge_TagMutatedOutOfBand(t *testing.T) {
	testAccPreCheck(t)

	name := acctestPrefix + "edge-oob-mut"
	config := `
resource "mistershell_tag" "test" {
  name  = "` + name + `"
  color = "blue"
}
`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			// Step 1: create with color=blue.
			{
				Config: config,
				Check:  resource.TestCheckResourceAttr("mistershell_tag.test", "color", "blue"),
			},
			// Step 2: mutate color out-of-band, then re-apply -> drift corrected.
			{
				PreConfig: func() {
					c := testAccClient()
					if c == nil {
						t.Fatal("PreConfig: client not configured")
					}
					id := edgeFindTagByName(t, name)
					newColor := "red"
					if _, err := c.UpdateTag(context.Background(), id, client.TagUpdateInput{
						Color: &newColor,
					}); err != nil {
						t.Fatalf("out-of-band update of tag %d: %v", id, err)
					}
				},
				Config: config,
				Check:  resource.TestCheckResourceAttr("mistershell_tag.test", "color", "blue"),
			},
		},
	})
}

// TestAccEdge_TagExclusiveDrift proves that mistershell_resource.tag_ids takes
// exclusive ownership of a resource's tag membership: an extra tag added
// out-of-band is removed on the next apply, and removing all tags out-of-band
// re-adds the managed one.
func TestAccEdge_TagExclusiveDrift(t *testing.T) {
	testAccPreCheck(t)

	resName := acctestPrefix + "edge-excl-res"

	// Two tags (a, b) exist, but tag_ids manages only [a].
	config := edgePreamble() + `
resource "mistershell_tag" "a" {
  name  = "` + acctestPrefix + `edge-excl-a"
  color = "blue"
}

resource "mistershell_tag" "b" {
  name  = "` + acctestPrefix + `edge-excl-b"
  color = "green"
}

resource "mistershell_resource" "test" {
  name          = "` + resName + `"
  resource_type = "linux"
  external_id   = "` + acctestPrefix + `EDGE-EXCL"
  location_id   = mistershell_location.test.id
  credential_id = mistershell_credential.test.id

  tag_ids = [mistershell_tag.a.id]

  connector_data = jsonencode({
    host = "192.168.99.99"
    port = 22
  })
}
`

	// findResourceID resolves the managed resource's id by exact name.
	findResourceID := func() int64 {
		c := testAccClient()
		if c == nil {
			t.Fatal("PreConfig: client not configured")
		}
		items, err := c.ListNetworkResources(context.Background(), client.NetworkResourceListFilter{Search: resName})
		if err != nil {
			t.Fatalf("listing network resources for %q: %v", resName, err)
		}
		for _, it := range items {
			if it.Name == resName {
				return it.ID
			}
		}
		t.Fatalf("no network resource found with exact name %q", resName)
		return 0
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			// Step 1: only tag a managed.
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_resource.test", "tag_ids.#", "1"),
					resource.TestCheckResourceAttr("mistershell_resource.test", "tags.#", "1"),
				),
			},
			// Step 2: add tag b out-of-band -> exclusive management removes it.
			{
				PreConfig: func() {
					c := testAccClient()
					if c == nil {
						t.Fatal("PreConfig: client not configured")
					}
					resID := findResourceID()
					tagA := edgeFindTagByName(t, acctestPrefix+"edge-excl-a")
					tagB := edgeFindTagByName(t, acctestPrefix+"edge-excl-b")
					if _, err := c.SetResourceTags(context.Background(), resID, []int64{tagA, tagB}); err != nil {
						t.Fatalf("out-of-band SetResourceTags(%d, [a,b]): %v", resID, err)
					}
				},
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_resource.test", "tag_ids.#", "1"),
					resource.TestCheckResourceAttr("mistershell_resource.test", "tags.#", "1"),
				),
			},
			// Step 3: clear all tags out-of-band -> managed tag a is re-added.
			{
				PreConfig: func() {
					c := testAccClient()
					if c == nil {
						t.Fatal("PreConfig: client not configured")
					}
					resID := findResourceID()
					if _, err := c.SetResourceTags(context.Background(), resID, []int64{}); err != nil {
						t.Fatalf("out-of-band SetResourceTags(%d, []): %v", resID, err)
					}
				},
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_resource.test", "tag_ids.#", "1"),
					resource.TestCheckResourceAttr("mistershell_resource.test", "tags.#", "1"),
				),
			},
		},
	})
}

// ---------------------------------------------------------------------------
// P2 — subtle bug classes
// ---------------------------------------------------------------------------

// TestAccEdge_ClearToNull proves the explicit-null PATCH path: a credential
// created WITH a description, then re-applied WITHOUT one, must clear the
// description (not leave it set), and the cleared state must be stable (no
// perpetual diff).
func TestAccEdge_ClearToNull(t *testing.T) {
	testAccPreCheck(t)

	withDesc := `
resource "mistershell_tag" "test" {
  name        = "` + acctestPrefix + `edge-clear"
  color       = "blue"
  description = "initial description"
}
`
	withoutDesc := `
resource "mistershell_tag" "test" {
  name  = "` + acctestPrefix + `edge-clear"
  color = "blue"
}
`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			// Step 1: description set.
			{
				Config: withDesc,
				Check:  resource.TestCheckResourceAttr("mistershell_tag.test", "description", "initial description"),
			},
			// Step 2: description cleared -> explicit null PATCH -> unset. This
			// exercises the deliberate "omit omitempty on *UpdateInput" design so a
			// cleared value sends explicit null to the PATCH endpoint.
			{
				Config: withoutDesc,
				Check:  resource.TestCheckNoResourceAttr("mistershell_tag.test", "description"),
			},
			// Step 3: re-apply identical config -> no perpetual diff.
			{
				Config:             withoutDesc,
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
		},
	})
}

// TestAccEdge_NormalizedJSON is a CHARACTERIZATION test for the opaque-JSON
// fields. It documents two distinct, currently-true behaviors:
//
//  1. Re-applying the BYTE-IDENTICAL connector_data is a no-op (stored-from-config
//     Read-preserve does not drift) — asserted as an empty plan.
//  2. Re-applying a SEMANTICALLY-EQUAL but reformatted connector_data (extra
//     whitespace / reordered keys) currently DOES produce a cosmetic in-place
//     update. jsontypes.Normalized implements semantic equality, but for these
//     stored-from-config Optional attributes the framework is not suppressing the
//     reformat diff. Tracked as an API/provider follow-up; users avoid it by
//     writing connector_data with jsonencode() (deterministic output).
//
// If the reformat diff ever starts being suppressed, step 2 flips to a non-empty
// failure and this test must be updated — that is the intended early warning.
func TestAccEdge_NormalizedJSON(t *testing.T) {
	testAccPreCheck(t)

	base := edgePreamble() + `
resource "mistershell_resource" "test" {
  name          = "` + acctestPrefix + `edge-json-res"
  resource_type = "linux"
  external_id   = "` + acctestPrefix + `EDGE-JSON"
  location_id   = mistershell_location.test.id
  credential_id = mistershell_credential.test.id

  connector_data = jsonencode({
    host = "192.168.99.99"
    port = 22
  })
}
`
	// Same JSON, keys reordered + extra whitespace, as a raw string literal.
	reordered := edgePreamble() + `
resource "mistershell_resource" "test" {
  name          = "` + acctestPrefix + `edge-json-res"
  resource_type = "linux"
  external_id   = "` + acctestPrefix + `EDGE-JSON"
  location_id   = mistershell_location.test.id
  credential_id = mistershell_credential.test.id

  connector_data = "{\"host\": \"192.168.99.99\",  \"port\": 22}"
}
`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			{Config: base},
			// (1) Byte-identical connector_data -> empty plan (stored-from-config
			// Read-preserve does not drift).
			{
				Config:             base,
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
			// (2) Semantically equal but reformatted connector_data -> currently a
			// cosmetic in-place update (see the function comment). Characterized as a
			// non-empty plan so a future suppression change is caught here.
			{
				Config:             reordered,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// TestAccEdge_TagIDsTransitions walks tag_ids through its lifecycle to prove the
// distinction between "managed empty" (tags.#=0) and "released/unmanaged"
// (tags left in place), plus the release-while-set semantics.
func TestAccEdge_TagIDsTransitions(t *testing.T) {
	testAccPreCheck(t)

	// One tag (a) plus the managed resource. tagIDsBlock is the HCL line(s) to
	// splice in for tag_ids ("" means omit the attribute entirely / unmanaged).
	mk := func(tagIDsBlock string) string {
		return edgePreamble() + `
resource "mistershell_tag" "a" {
  name  = "` + acctestPrefix + `edge-trans-a"
  color = "blue"
}

resource "mistershell_resource" "test" {
  name          = "` + acctestPrefix + `edge-trans-res"
  resource_type = "linux"
  external_id   = "` + acctestPrefix + `EDGE-TRANS"
  location_id   = mistershell_location.test.id
  credential_id = mistershell_credential.test.id
` + tagIDsBlock + `
  connector_data = jsonencode({
    host = "192.168.99.99"
    port = 22
  })
}
`
	}

	withTagA := mk("  tag_ids = [mistershell_tag.a.id]\n")
	withEmpty := mk("  tag_ids = []\n")
	unmanaged := mk("")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			// Step 1: tag a managed.
			{
				Config: withTagA,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_resource.test", "tag_ids.#", "1"),
					resource.TestCheckResourceAttr("mistershell_resource.test", "tags.#", "1"),
				),
			},
			// Step 2: managed-empty -> exclusive clear, server has 0 tags.
			{
				Config: withEmpty,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_resource.test", "tag_ids.#", "0"),
					resource.TestCheckResourceAttr("mistershell_resource.test", "tags.#", "0"),
				),
			},
			// Step 3: remove tag_ids entirely (unmanaged). Server already has 0
			// tags, so tags stay 0 and tag_ids is unset.
			{
				Config: unmanaged,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_resource.test", "tags.#", "0"),
					resource.TestCheckNoResourceAttr("mistershell_resource.test", "tag_ids"),
				),
			},
			// Step 4: re-manage tag a (set again).
			{
				Config: withTagA,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_resource.test", "tag_ids.#", "1"),
					resource.TestCheckResourceAttr("mistershell_resource.test", "tags.#", "1"),
				),
			},
			// Step 5: release while a tag IS set (drop tag_ids). Release leaves the
			// existing tag in place; tag_ids is unset.
			{
				Config: unmanaged,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_resource.test", "tags.#", "1"),
					resource.TestCheckNoResourceAttr("mistershell_resource.test", "tag_ids"),
				),
			},
		},
	})
}
