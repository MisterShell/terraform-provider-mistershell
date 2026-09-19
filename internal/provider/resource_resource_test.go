package provider_test

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func testAccSSHResourcePreCheck(t *testing.T) {
	t.Helper()
	testAccPreCheck(t)
	for _, name := range []string{"MISTERSHELL_ACC_SSH_HOST", "MISTERSHELL_ACC_SSH_USERNAME", "MISTERSHELL_ACC_SSH_PASSWORD"} {
		if os.Getenv(name) == "" {
			t.Skipf("live SSH onboarding test requires %s", name)
		}
	}
	if value := os.Getenv("MISTERSHELL_ACC_SSH_PORT"); value != "" {
		if _, err := strconv.Atoi(value); err != nil {
			t.Fatalf("MISTERSHELL_ACC_SSH_PORT must be an integer: %v", err)
		}
	}
}

func TestAccNetworkResource_automaticSSHOnboarding(t *testing.T) {
	testAccSSHResourcePreCheck(t)
	name := acctestPrefix + "auto"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccSSHResourceConfig(name, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_resource.test", "engine_id", "ssh"),
					resource.TestCheckResourceAttr("mistershell_resource.test", "resource_type", "linux"),
					resource.TestCheckResourceAttrSet("mistershell_resource.test", "external_id"),
					resource.TestCheckResourceAttrSet("mistershell_resource.test", "id"),
					resource.TestCheckResourceAttrSet("mistershell_resource.test", "connector_id"),
				),
			},
			{
				Config:             testAccSSHResourceConfig(name, ""),
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
			{
				ResourceName:      "mistershell_resource.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"connector_data", "status", "updated_at", "last_connectivity_check",
					"last_connectivity_result", "last_collection_at", "next_collection_at",
				},
			},
		},
	})
}

func TestAccNetworkResource_explicitGenericSSH(t *testing.T) {
	testAccSSHResourcePreCheck(t)
	name := acctestPrefix + "explicit-generic"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{{
			Config: testAccSSHResourceConfig(name, "generic_ssh"),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("mistershell_resource.test", "engine_id", "ssh"),
				resource.TestCheckResourceAttr("mistershell_resource.test", "resource_type", "generic_ssh"),
				resource.TestCheckNoResourceAttr("mistershell_resource.test", "external_id"),
			),
		}},
	})
}

func TestAccNetworkResource_automaticGenericSSHFallback(t *testing.T) {
	testAccPreCheck(t)

	host, port, username, password := startGenericSSHFixture(t)
	name := acctestPrefix + "auto-generic"
	config := testAccSSHTargetConfig(name, "", discoverRootLocationID(t), host, port, username, password)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_resource.test", "engine_id", "ssh"),
					resource.TestCheckResourceAttr("mistershell_resource.test", "resource_type", "generic_ssh"),
					resource.TestCheckNoResourceAttr("mistershell_resource.test", "external_id"),
				),
			},
			{Config: config, PlanOnly: true, ExpectNonEmptyPlan: false},
		},
	})
}

func TestAccNetworkResource_explicitLinux(t *testing.T) {
	testAccSSHResourcePreCheck(t)
	name := acctestPrefix + "exact-linux"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{{
			Config: testAccSSHResourceConfig(name, "linux"),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("mistershell_resource.test", "engine_id", "ssh"),
				resource.TestCheckResourceAttr("mistershell_resource.test", "resource_type", "linux"),
				resource.TestCheckResourceAttrSet("mistershell_resource.test", "external_id"),
			),
		}},
	})
}

func TestAccNetworkResource_explicitTypeMismatch(t *testing.T) {
	testAccSSHResourcePreCheck(t)
	name := acctestPrefix + "exact-mismatch"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{{
			Config:      testAccSSHResourceConfig(name, "cisco_ios"),
			ExpectError: regexp.MustCompile(`Resource discovery\s+returned type_mismatch`),
		}},
	})
}

func TestAccNetworkResource_mutableUpdate(t *testing.T) {
	testAccSSHResourcePreCheck(t)
	name := acctestPrefix + "mutable"
	var createdID string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccSSHResourceUpdateConfig(name, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					captureResourceID("mistershell_resource.test", &createdID),
					resource.TestCheckResourceAttr("mistershell_resource.test", "name", name),
					resource.TestCheckResourceAttr("mistershell_resource.test", "is_enabled", "true"),
					resource.TestCheckResourceAttrPair("mistershell_resource.test", "location_id", "mistershell_location.a", "id"),
					resource.TestCheckResourceAttrPair("mistershell_resource.test", "credential_id", "mistershell_credential.a", "id"),
					resource.TestCheckTypeSetElemAttrPair("mistershell_resource.test", "tag_ids.*", "mistershell_tag.a", "id"),
				),
			},
			{
				Config: testAccSSHResourceUpdateConfig(name+"-updated", true),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkResourceID("mistershell_resource.test", &createdID),
					resource.TestCheckResourceAttr("mistershell_resource.test", "name", name+"-updated"),
					resource.TestCheckResourceAttr("mistershell_resource.test", "is_enabled", "false"),
					resource.TestCheckResourceAttrPair("mistershell_resource.test", "location_id", "mistershell_location.b", "id"),
					resource.TestCheckResourceAttrPair("mistershell_resource.test", "credential_id", "mistershell_credential.b", "id"),
					resource.TestCheckTypeSetElemAttrPair("mistershell_resource.test", "tag_ids.*", "mistershell_tag.b", "id"),
				),
			},
		},
	})
}

// sshgw_allow_api_key round-trips through the API (not just the plan): the
// live read-back check proves MisterShell stored the flag.
func TestAccNetworkResource_sshgwAllowApiKey(t *testing.T) {
	testAccSSHResourcePreCheck(t)
	name := acctestPrefix + "sshgw-api-key"
	var createdID string

	liveFlag := func(want bool) resource.TestCheckFunc {
		return func(state *terraform.State) error {
			rs, ok := state.RootModule().Resources["mistershell_resource.test"]
			if !ok {
				return fmt.Errorf("mistershell_resource.test is missing from state")
			}
			id, err := strconv.ParseInt(rs.Primary.ID, 10, 64)
			if err != nil {
				return err
			}
			actual, err := testAccClient().GetNetworkResource(context.Background(), id)
			if err != nil {
				return err
			}
			if actual.SshgwAllowAPIKey != want {
				return fmt.Errorf("live sshgw_allow_api_key=%t, want %t", actual.SshgwAllowAPIKey, want)
			}
			return nil
		}
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccSSHResourceSshgwConfig(name, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					captureResourceID("mistershell_resource.test", &createdID),
					resource.TestCheckResourceAttr("mistershell_resource.test", "sshgw_allow_api_key", "true"),
					liveFlag(true),
				),
			},
			{
				Config: testAccSSHResourceSshgwConfig(name, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkResourceID("mistershell_resource.test", &createdID),
					resource.TestCheckResourceAttr("mistershell_resource.test", "sshgw_allow_api_key", "false"),
					liveFlag(false),
				),
			},
			{
				Config:             testAccSSHResourceSshgwConfig(name, false),
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
		},
	})
}

// MisterShell rejects sshgw_allow_api_key = true on graphical-only types with
// HTTP 400. Onboarding connects for real, so each subtest needs a live target
// of that type and skips without one.
func TestAccNetworkResource_sshgwAllowApiKeyRejectedOnGraphicalTypes(t *testing.T) {
	testAccPreCheck(t)
	for _, tc := range []struct {
		resourceType, envPrefix, portField string
		defaultPort                        int
	}{
		{"generic_rdp", "MISTERSHELL_ACC_RDP", "rdp_port", 3389},
		{"generic_vnc", "MISTERSHELL_ACC_VNC", "vnc_port", 5900},
	} {
		t.Run(tc.resourceType, func(t *testing.T) {
			host, username, password := os.Getenv(tc.envPrefix+"_HOST"), os.Getenv(tc.envPrefix+"_USERNAME"), os.Getenv(tc.envPrefix+"_PASSWORD")
			if host == "" || username == "" || password == "" {
				t.Skipf("live %s onboarding test requires %s_HOST, %s_USERNAME and %s_PASSWORD", tc.resourceType, tc.envPrefix, tc.envPrefix, tc.envPrefix)
			}
			port := tc.defaultPort
			if value := os.Getenv(tc.envPrefix + "_PORT"); value != "" {
				parsed, err := strconv.Atoi(value)
				if err != nil {
					t.Fatalf("%s_PORT must be an integer: %v", tc.envPrefix, err)
				}
				port = parsed
			}
			name := acctestPrefix + "sshgw-" + tc.resourceType
			config := fmt.Sprintf(`
resource "mistershell_location" "test" {
  name      = %q
  kind      = "geo"
  parent_id = 1
}

resource "mistershell_credential" "test" {
  name            = %q
  credential_type = "username_password"
  credential_data = { username = %q, password = %q }
}

resource "mistershell_resource" "test" {
  name                = %q
  resource_type       = %q
  location_id         = mistershell_location.test.id
  credential_id       = mistershell_credential.test.id
  sshgw_allow_api_key = true
  connector_data = {
    host = %q
    %s   = %d
  }
}
`, name+"-location", name+"-credential", username, password, name, tc.resourceType, host, tc.portField, port)
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				CheckDestroy:             testAccCheckAllDestroyed,
				Steps: []resource.TestStep{{
					Config:      config,
					ExpectError: regexp.MustCompile(`(?s)Error creating network resource.*status 400`),
				}},
			})
		})
	}
}

func testAccSSHResourceSshgwConfig(name string, allow bool) string {
	base := testAccSSHResourceConfig(name, "")
	return strings.Replace(base, "  connector_data = {", fmt.Sprintf("  sshgw_allow_api_key = %t\n  connector_data = {", allow), 1)
}

func captureResourceID(address string, target *string) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		instance, ok := state.RootModule().Resources[address]
		if !ok || instance.Primary.ID == "" {
			return fmt.Errorf("%s has no state ID", address)
		}
		*target = instance.Primary.ID
		return nil
	}
}

func checkResourceID(address string, expected *string) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		instance, ok := state.RootModule().Resources[address]
		if !ok {
			return fmt.Errorf("%s is missing from state", address)
		}
		if *expected == "" || instance.Primary.ID != *expected {
			return fmt.Errorf("%s ID changed from %q to %q", address, *expected, instance.Primary.ID)
		}
		return nil
	}
}

func testAccSSHTargetConfig(name, resourceType string, rootLocationID int64, host string, port int, username, password string) string {
	typeLine := `  engine_id = "ssh"`
	if resourceType != "" {
		typeLine = "  resource_type = " + strconv.Quote(resourceType)
	}

	return fmt.Sprintf(`
resource "mistershell_location" "test" {
  name      = %q
  kind      = "geo"
  parent_id = %d
}

resource "mistershell_credential" "test" {
  name            = %q
  credential_type = "username_password"
  credential_data = {
    username = %q
    password = %q
  }
}

resource "mistershell_resource" "test" {
  name          = %q
%s
  location_id   = mistershell_location.test.id
  credential_id = mistershell_credential.test.id
  connector_data = {
    host = %q
    port = %d
  }
}
`, name+"-location", rootLocationID, name+"-credential", username, password, name, typeLine, host, port)
}

func testAccSSHResourceUpdateConfig(name string, updated bool) string {
	locationRef := "mistershell_location.a.id"
	credentialRef := "mistershell_credential.a.id"
	tagRef := "mistershell_tag.a.id"
	host := os.Getenv("MISTERSHELL_ACC_SSH_HOST")
	enabled := "true"
	if updated {
		locationRef = "mistershell_location.b.id"
		credentialRef = "mistershell_credential.b.id"
		tagRef = "mistershell_tag.b.id"
		enabled = "false"
		if updateHost := os.Getenv("MISTERSHELL_ACC_SSH_UPDATE_HOST"); updateHost != "" {
			host = updateHost
		}
	}

	return fmt.Sprintf(`
resource "mistershell_location" "a" {
  name      = %q
  kind      = "geo"
  parent_id = 1
}

resource "mistershell_location" "b" {
  name      = %q
  kind      = "geo"
  parent_id = 1
}

resource "mistershell_credential" "a" {
  name            = %q
  credential_type = "username_password"
  credential_data = { username = %q, password = %q }
}

resource "mistershell_credential" "b" {
  name            = %q
  credential_type = "username_password"
  credential_data = { username = %q, password = %q }
}

resource "mistershell_tag" "a" { name = %q }
resource "mistershell_tag" "b" { name = %q }

resource "mistershell_resource" "test" {
  name          = %q
  resource_type = "linux"
  location_id   = %s
  credential_id = %s
  is_enabled    = %s
  tag_ids       = [%s]
  connector_data = {
    host = %q
    port = %s
  }
}
`, acctestPrefix+"mutable-loc-a", acctestPrefix+"mutable-loc-b",
		acctestPrefix+"mutable-cred-a", os.Getenv("MISTERSHELL_ACC_SSH_USERNAME"), os.Getenv("MISTERSHELL_ACC_SSH_PASSWORD"),
		acctestPrefix+"mutable-cred-b", os.Getenv("MISTERSHELL_ACC_SSH_USERNAME"), os.Getenv("MISTERSHELL_ACC_SSH_PASSWORD"),
		acctestPrefix+"mutable-tag-a", acctestPrefix+"mutable-tag-b", name, locationRef, credentialRef, enabled, tagRef,
		host, testAccSSHPort())
}

func testAccSSHResourceConfig(name, resourceType string) string {
	port, _ := strconv.Atoi(testAccSSHPort())
	typeLine := "  engine_id = \"ssh\""
	if resourceType != "" {
		typeLine = "  resource_type = " + strconv.Quote(resourceType)
	}

	return fmt.Sprintf(`
resource "mistershell_location" "test" {
  name      = %q
  kind      = "geo"
  parent_id = 1
}

resource "mistershell_credential" "test" {
  name            = %q
  credential_type = "username_password"
  credential_data = {
    username = %q
    password = %q
  }
}

resource "mistershell_resource" "test" {
  name          = %q
%s
  location_id   = mistershell_location.test.id
  credential_id = mistershell_credential.test.id
  connector_data = {
    host = %q
    port = %d
  }
}
`, name+"-location", name+"-credential", os.Getenv("MISTERSHELL_ACC_SSH_USERNAME"), os.Getenv("MISTERSHELL_ACC_SSH_PASSWORD"), name, typeLine, os.Getenv("MISTERSHELL_ACC_SSH_HOST"), port)
}

func testAccSSHPort() string {
	if value := os.Getenv("MISTERSHELL_ACC_SSH_PORT"); value != "" {
		return value
	}
	return "22"
}
