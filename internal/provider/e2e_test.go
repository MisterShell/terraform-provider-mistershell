package provider_test

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"terraform-provider-mistershell/internal/client"
)

// resourceConfig describes how to render one mistershell_resource block in the
// full-graph config: the credential it references and its connector_data.
type resourceConfig struct {
	credRef       string // terraform reference to the credential resource (e.g. "mistershell_credential.ssh_password")
	connectorData string // jsonencode({...}) body (without the jsonencode wrapper)
}

// e2eResourceConfigs maps every supported resource_type to its connector_data
// and the credential it pairs with. Keys MUST cover client.SupportedResourceTypes
// exactly — the exhaustiveness guard in the test enforces this.
//
// Pairing (by connector compatibility, all 7 credential types used):
//   - linux                         -> ssh_key
//   - other SSH-family + windows    -> ssh_password
//   - generic_rdp                   -> rdp_password
//   - database                      -> db_password
//   - aws_account                   -> aws_credentials
//   - azure_subscription            -> azure_service_principal
//   - kubernetes_cluster            -> kubeconfig
func e2eResourceConfigs() map[string]resourceConfig {
	ssh := `{ host = "10.0.0.10", port = 22 }`
	sshCred := "mistershell_credential.ssh_password.id"

	return map[string]resourceConfig{
		// SSH family using ssh_password
		"cisco_ios":         {sshCred, ssh},
		"cisco_iosxe":       {sshCred, ssh},
		"cisco_iosxe_sdwan": {sshCred, ssh},
		"cisco_nxos":        {sshCred, ssh},
		"cisco_ise":         {sshCred, ssh},
		"cisco_vbond":       {sshCred, ssh},
		"cisco_vmanage":     {sshCred, ssh},
		"cisco_vsmart":      {sshCred, ssh},
		"infoblox_nios":     {sshCred, ssh},
		"generic_ssh":       {sshCred, ssh},
		"panos_ssh":         {sshCred, ssh},

		// SSH family using ssh_key
		"linux": {"mistershell_credential.ssh_key.id", ssh},

		// windows (ssh connector) using ssh_password
		"windows": {sshCred, `{ host = "10.0.0.20", port = 22, rdp_port = 3389, nla_required = true, keyboard_layout = "" }`},

		// generic_rdp using rdp_password
		"generic_rdp": {"mistershell_credential.rdp_password.id", `{ host = "10.0.0.21", rdp_port = 3389, nla_required = true, keyboard_layout = "" }`},

		// database using db_password
		"database": {"mistershell_credential.db_password.id", `{ engine = "postgres", host = "10.0.0.30", port = 5432 }`},

		// cloud / k8s
		"aws_account":        {"mistershell_credential.aws_credentials.id", `{ hostname = "my-aws-alias" }`},
		"azure_subscription": {"mistershell_credential.azure_service_principal.id", `{ tenant_id = "00000000-0000-0000-0000-000000000002", subscription_id = "00000000-0000-0000-0000-000000000003", cloud = "AzureCloud" }`},
		"kubernetes_cluster": {"mistershell_credential.kubeconfig.id", `{ context = "", namespace = "default" }`},
	}
}

// e2eCredentialData maps every supported credential_type to its credential_data
// body. Keys MUST cover client.SupportedCredentialTypes exactly.
func e2eCredentialData() map[string]string {
	return map[string]string{
		"ssh_password":            `{ username = "admin", password = "Passw0rd!", enable_password = "En4ble!" }`,
		"ssh_key":                 `{ username = "admin", private_key = "-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAFAKEKEY\n-----END OPENSSH PRIVATE KEY-----", passphrase = "" }`,
		"aws_credentials":         `{ access_key = "AKIAIOSFODNN7EXAMPLE", secret_key = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY" }`,
		"azure_service_principal": `{ client_id = "00000000-0000-0000-0000-000000000001", client_secret = "fake-secret" }`,
		"kubeconfig":              `{ kubeconfig = "apiVersion: v1\nkind: Config\nclusters: []\ncontexts: []\nusers: []\n" }`,
		"rdp_password":            `{ username = "Administrator", domain = "CORP", password = "Passw0rd!" }`,
		"db_password":             `{ username = "dbadmin", password = "Passw0rd!" }`,
	}
}

// resourceTFName sanitizes a resource_type into a valid Terraform resource
// instance name (already valid identifiers, but keep this explicit).
func resourceTFName(resourceType string) string {
	return resourceType
}

// e2eConfig renders the full dependency graph: 2 locations, 7 credentials,
// 18 network resources, and a handful of data sources reading objects back.
func e2eConfig(prefix string, rootID int64) string {
	var b strings.Builder

	// --- Locations: parent + child. The parent hangs beneath the existing
	// root (rootID) because MisterShell refuses to delete root locations
	// (parent_id == null), which would otherwise leave an undeletable orphan.
	fmt.Fprintf(&b, `
resource "mistershell_location" "parent" {
  name      = "%[1]sparent"
  kind      = "geo"
  parent_id = %[2]d
}

resource "mistershell_location" "child" {
  name      = "%[1]schild"
  kind      = "geo"
  parent_id = mistershell_location.parent.id
}
`, prefix, rootID)

	// --- Credentials: one per credential_type -----------------------------
	credData := e2eCredentialData()
	for _, ct := range sortedKeys(credData) {
		fmt.Fprintf(&b, `
resource "mistershell_credential" %[1]q {
  name            = "%[2]s%[3]s"
  credential_type = %[3]q
  credential_data = jsonencode(%[4]s)
}
`, ct, prefix, ct, credData[ct])
	}

	// --- Network resources: one per resource_type -------------------------
	resCfg := e2eResourceConfigs()
	for _, rt := range sortedKeys(resCfg) {
		cfg := resCfg[rt]
		fmt.Fprintf(&b, `
resource "mistershell_resource" %[1]q {
  name          = "%[2]s%[3]s"
  resource_type = %[3]q
  external_id   = "%[2]sxid-%[3]s"
  location_id   = mistershell_location.child.id
  credential_id = %[4]s
  connector_data = jsonencode(%[5]s)
}
`, resourceTFName(rt), prefix, rt, cfg.credRef, cfg.connectorData)
	}

	// --- Data sources reading representatives back ------------------------
	fmt.Fprintf(&b, `
data "mistershell_location" "by_name" {
  name      = mistershell_location.child.name
  parent_id = mistershell_location.parent.id
}

data "mistershell_resource" "by_id" {
  id = mistershell_resource.linux.id
}

data "mistershell_resource" "by_name" {
  name = mistershell_resource.generic_ssh.name
}

data "mistershell_credential" "by_id" {
  id = mistershell_credential.ssh_password.id
}

data "mistershell_credential" "by_type" {
  name            = mistershell_credential.kubeconfig.name
  credential_type = "kubeconfig"
}
`)

	return b.String()
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// TestAccEndToEnd_fullGraph applies one Terraform config that exercises every
// supported resource type and credential type in a single dependency graph,
// reads representatives back via data sources, asserts no perpetual diff, and
// verifies (via testAccCheckAllDestroyed) that everything is gone after destroy.
func TestAccEndToEnd_fullGraph(t *testing.T) {
	testAccPreCheck(t)

	// Exhaustiveness guard: every generated supported type MUST be represented
	// in the config, so adding a new type to the codegen forces test coverage.
	assertExhaustive(t)

	// MisterShell forbids deleting root locations, so attach the test tree
	// beneath the instance's existing root (discovered dynamically).
	rootID := discoverRootLocationID(t)
	config := e2eConfig(acctestPrefix, rootID)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			// Apply the whole graph.
			{
				Config: config,
				Check:  e2eChecks(),
			},
			// Re-plan with the same config — must be empty (no drift / perpetual diff).
			{
				Config:             config,
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
		},
	})
}

// e2eChecks asserts the created objects and the data-source readbacks.
func e2eChecks() resource.TestCheckFunc {
	checks := []resource.TestCheckFunc{
		// Location hierarchy.
		resource.TestCheckResourceAttrPair("mistershell_location.child", "parent_id", "mistershell_location.parent", "id"),
		resource.TestCheckResourceAttrSet("mistershell_location.parent", "id"),

		// Data source: location by name -> created child.
		resource.TestCheckResourceAttrPair("data.mistershell_location.by_name", "id", "mistershell_location.child", "id"),
		resource.TestCheckResourceAttr("data.mistershell_location.by_name", "kind", "geo"),

		// Data source: resource by id -> linux.
		resource.TestCheckResourceAttrPair("data.mistershell_resource.by_id", "id", "mistershell_resource.linux", "id"),
		resource.TestCheckResourceAttr("data.mistershell_resource.by_id", "resource_type", "linux"),

		// Data source: resource by name -> generic_ssh.
		resource.TestCheckResourceAttrPair("data.mistershell_resource.by_name", "id", "mistershell_resource.generic_ssh", "id"),
		resource.TestCheckResourceAttr("data.mistershell_resource.by_name", "resource_type", "generic_ssh"),

		// Data source: credential by id -> ssh_password.
		resource.TestCheckResourceAttrPair("data.mistershell_credential.by_id", "id", "mistershell_credential.ssh_password", "id"),
		resource.TestCheckResourceAttr("data.mistershell_credential.by_id", "credential_type", "ssh_password"),

		// Data source: credential by type -> kubeconfig.
		resource.TestCheckResourceAttrPair("data.mistershell_credential.by_type", "id", "mistershell_credential.kubeconfig", "id"),
		resource.TestCheckResourceAttr("data.mistershell_credential.by_type", "credential_type", "kubeconfig"),
	}

	// Per-resource assertions: each created and wired to the child location.
	for rt := range e2eResourceConfigs() {
		addr := "mistershell_resource." + resourceTFName(rt)
		checks = append(checks,
			resource.TestCheckResourceAttr(addr, "resource_type", rt),
			resource.TestCheckResourceAttrSet(addr, "id"),
			resource.TestCheckResourceAttrSet(addr, "connector_id"),
			resource.TestCheckResourceAttrPair(addr, "location_id", "mistershell_location.child", "id"),
		)
	}

	// Per-credential assertions: each created.
	for ct := range e2eCredentialData() {
		addr := "mistershell_credential." + ct
		checks = append(checks,
			resource.TestCheckResourceAttr(addr, "credential_type", ct),
			resource.TestCheckResourceAttrSet(addr, "id"),
		)
	}

	return resource.ComposeAggregateTestCheckFunc(checks...)
}

// assertExhaustive fails loudly if any generated supported type is not covered
// by the e2e config, keeping the suite exhaustive as new types are added.
func assertExhaustive(t *testing.T) {
	t.Helper()

	resCfg := e2eResourceConfigs()
	var missingRes []string
	for _, rt := range client.SupportedResourceTypes {
		if _, ok := resCfg[rt]; !ok {
			missingRes = append(missingRes, rt)
		}
	}
	// Also catch stale entries that no longer exist in the generated list.
	supportedRes := toSet(client.SupportedResourceTypes)
	var staleRes []string
	for rt := range resCfg {
		if !supportedRes[rt] {
			staleRes = append(staleRes, rt)
		}
	}

	credData := e2eCredentialData()
	var missingCred []string
	for _, ct := range client.SupportedCredentialTypes {
		if _, ok := credData[ct]; !ok {
			missingCred = append(missingCred, ct)
		}
	}
	supportedCred := toSet(client.SupportedCredentialTypes)
	var staleCred []string
	for ct := range credData {
		if !supportedCred[ct] {
			staleCred = append(staleCred, ct)
		}
	}

	if len(missingRes) > 0 {
		t.Fatalf("e2e config missing coverage for resource_type(s): %v", sorted(missingRes))
	}
	if len(staleRes) > 0 {
		t.Fatalf("e2e config references unknown resource_type(s) not in client.SupportedResourceTypes: %v", sorted(staleRes))
	}
	if len(missingCred) > 0 {
		t.Fatalf("e2e config missing coverage for credential_type(s): %v", sorted(missingCred))
	}
	if len(staleCred) > 0 {
		t.Fatalf("e2e config references unknown credential_type(s) not in client.SupportedCredentialTypes: %v", sorted(staleCred))
	}
}

func toSet(items []string) map[string]bool {
	m := make(map[string]bool, len(items))
	for _, it := range items {
		m[it] = true
	}
	return m
}

func sorted(items []string) []string {
	out := append([]string(nil), items...)
	sort.Strings(out)
	return out
}
