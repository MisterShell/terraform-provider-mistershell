package provider_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// Wave 2 acceptance tests: mistershell_log_destination / mistershell_setting
// resources, and the mistershell_log_destination, _log_destination_presets and
// _setting data sources. All object names are prefixed with acctestPrefix so
// reruns / parallel runs do not collide and the sweeper can target them. Each
// resource case uses CheckDestroy.
//
// Setting key chosen for the live test: "notes_edit_lock_ttl_seconds".
// It is the heartbeat TTL (seconds) for the collaborative Notes edit-lock — a
// purely cosmetic UI/collaboration timing knob (range 30-600, default 120). It
// is NOT auth/security/session-policy/credential-related, its live value on the
// instance equals its registry default (so reset-on-destroy fully restores it),
// and a transient change cannot disrupt the running instance or other tests.
const settingTestKey = "notes_edit_lock_ttl_seconds"

// ---------------------------------------------------------------------------
// mistershell_log_destination — syslog: create, update, import
// ---------------------------------------------------------------------------

func TestAccLogDestination_syslog(t *testing.T) {
	testAccPreCheck(t)

	name := acctestPrefix + "ld-syslog"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			// Create: enabled=false, two streams, min_severity=medium, full syslog config.
			{
				Config: testAccLogDestinationSyslogConfig(name, false, "medium", `["security", "app"]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_log_destination.test", "name", name),
					resource.TestCheckResourceAttr("mistershell_log_destination.test", "type", "syslog"),
					resource.TestCheckResourceAttr("mistershell_log_destination.test", "enabled", "false"),
					resource.TestCheckResourceAttr("mistershell_log_destination.test", "min_severity", "medium"),
					resource.TestCheckResourceAttr("mistershell_log_destination.test", "streams.#", "2"),
					resource.TestCheckTypeSetElemAttr("mistershell_log_destination.test", "streams.*", "security"),
					resource.TestCheckTypeSetElemAttr("mistershell_log_destination.test", "streams.*", "app"),
					resource.TestCheckResourceAttrSet("mistershell_log_destination.test", "id"),
					resource.TestCheckResourceAttrSet("mistershell_log_destination.test", "created_at"),
				),
			},
			// Update: change min_severity and streams.
			{
				Config: testAccLogDestinationSyslogConfig(name, true, "high", `["security", "policy", "api"]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_log_destination.test", "enabled", "true"),
					resource.TestCheckResourceAttr("mistershell_log_destination.test", "min_severity", "high"),
					resource.TestCheckResourceAttr("mistershell_log_destination.test", "streams.#", "3"),
					resource.TestCheckTypeSetElemAttr("mistershell_log_destination.test", "streams.*", "policy"),
				),
			},
			// Import. config is stored-from-config (the server reorders keys and
			// injects per-type defaults), so it is not recoverable on import —
			// ignore it, matching the credential_data pattern.
			{
				ResourceName:            "mistershell_log_destination.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"config"},
			},
		},
	})
}

// ---------------------------------------------------------------------------
// mistershell_log_destination — webhook with bearer-token (secret) auth
// ---------------------------------------------------------------------------

func TestAccLogDestination_webhookSecret(t *testing.T) {
	testAccPreCheck(t)

	name := acctestPrefix + "ld-webhook"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			// Create with a webhook + bearer-token auth (a secret the API masks).
			{
				Config: testAccLogDestinationWebhookConfig(name, "POST"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_log_destination.test", "name", name),
					resource.TestCheckResourceAttr("mistershell_log_destination.test", "type", "webhook"),
					resource.TestCheckResourceAttr("mistershell_log_destination.test", "min_severity", "info"),
					resource.TestCheckResourceAttr("mistershell_log_destination.test", "streams.#", "1"),
					resource.TestCheckTypeSetElemAttr("mistershell_log_destination.test", "streams.*", "security"),
					// config is preserved verbatim from plan (secret not read back).
					resource.TestCheckResourceAttrSet("mistershell_log_destination.test", "config"),
					resource.TestCheckResourceAttrSet("mistershell_log_destination.test", "id"),
				),
			},
			// Re-apply identical config: plan must be empty (no perpetual diff from
			// the masked secret — proves the preserve-config-from-plan rule).
			{
				Config:             testAccLogDestinationWebhookConfig(name, "POST"),
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
			// Update a non-secret field (method POST -> PUT).
			{
				Config: testAccLogDestinationWebhookConfig(name, "PUT"),
			},
			// Import: webhook auth secret is masked by the API, so config cannot be
			// verified on import — ignore it (same as the credential_data pattern).
			{
				ResourceName:            "mistershell_log_destination.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"config"},
			},
		},
	})
}

// ---------------------------------------------------------------------------
// mistershell_log_destination data source — by id and by name
// ---------------------------------------------------------------------------

func TestAccLogDestinationDataSource_byIDAndName(t *testing.T) {
	testAccPreCheck(t)

	name := acctestPrefix + "ld-ds"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccLogDestinationDataSourceConfig(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					// by id
					resource.TestCheckResourceAttrPair("data.mistershell_log_destination.by_id", "id", "mistershell_log_destination.test", "id"),
					resource.TestCheckResourceAttr("data.mistershell_log_destination.by_id", "name", name),
					resource.TestCheckResourceAttr("data.mistershell_log_destination.by_id", "type", "syslog"),
					resource.TestCheckResourceAttr("data.mistershell_log_destination.by_id", "min_severity", "low"),
					resource.TestCheckTypeSetElemAttr("data.mistershell_log_destination.by_id", "streams.*", "api"),
					// by name
					resource.TestCheckResourceAttrPair("data.mistershell_log_destination.by_name", "id", "mistershell_log_destination.test", "id"),
					resource.TestCheckResourceAttr("data.mistershell_log_destination.by_name", "type", "syslog"),
					resource.TestCheckTypeSetElemAttr("data.mistershell_log_destination.by_name", "streams.*", "api"),
				),
			},
		},
	})
}

// ---------------------------------------------------------------------------
// mistershell_log_destination_presets data source
// ---------------------------------------------------------------------------

func TestAccLogDestinationPresetsDataSource_basic(t *testing.T) {
	testAccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `data "mistershell_log_destination_presets" "all" {}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.mistershell_log_destination_presets.all", "id", "log_destination_presets"),
					// Non-empty list.
					resource.TestCheckResourceAttrWith("data.mistershell_log_destination_presets.all", "presets.#", func(v string) error {
						if v == "0" || v == "" {
							return fmt.Errorf("expected a non-empty presets list, got %q", v)
						}
						return nil
					}),
					// Must include a syslog and a webhook preset (table-completeness guard).
					resource.TestCheckTypeSetElemNestedAttrs("data.mistershell_log_destination_presets.all", "presets.*", map[string]string{
						"key":  "custom_syslog",
						"type": "syslog",
					}),
					resource.TestCheckTypeSetElemNestedAttrs("data.mistershell_log_destination_presets.all", "presets.*", map[string]string{
						"key":  "custom_webhook",
						"type": "webhook",
					}),
				),
			},
		},
	})
}

// ---------------------------------------------------------------------------
// mistershell_setting — set a benign key, assert round-trip, reset-on-destroy,
// plus a data source read of the same key.
// ---------------------------------------------------------------------------

func TestAccSetting_basic(t *testing.T) {
	testAccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		// CheckDestroy asserts the key was reset to its registry default.
		CheckDestroy: testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			// Set the benign integer key to a clearly test-y value.
			{
				Config: testAccSettingConfig(300),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_setting.test", "key", settingTestKey),
					resource.TestCheckResourceAttr("mistershell_setting.test", "id", settingTestKey),
					resource.TestCheckResourceAttr("mistershell_setting.test", "value", "300"),
					resource.TestCheckResourceAttr("mistershell_setting.test", "is_secret", "false"),
					resource.TestCheckResourceAttr("mistershell_setting.test", "default", "120"),
					resource.TestCheckResourceAttrSet("mistershell_setting.test", "updated_at"),
					// data source read of the same key
					resource.TestCheckResourceAttr("data.mistershell_setting.test", "key", settingTestKey),
					resource.TestCheckResourceAttr("data.mistershell_setting.test", "value", "300"),
					resource.TestCheckResourceAttr("data.mistershell_setting.test", "is_secret", "false"),
					resource.TestCheckResourceAttr("data.mistershell_setting.test", "default", "120"),
				),
			},
			// Re-apply identical config: plan must be empty.
			{
				Config:             testAccSettingConfig(300),
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
			// Update the value in place (PUT).
			{
				Config: testAccSettingConfig(240),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_setting.test", "value", "240"),
				),
			},
			// Import by key.
			{
				ResourceName:      "mistershell_setting.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// ---------------------------------------------------------------------------
// Config builders
// ---------------------------------------------------------------------------

func testAccLogDestinationSyslogConfig(name string, enabled bool, minSeverity, streams string) string {
	return fmt.Sprintf(`
resource "mistershell_log_destination" "test" {
  name         = %q
  enabled      = %t
  type         = "syslog"
  streams      = %s
  min_severity = %q

  config = jsonencode({
    type       = "syslog"
    host       = "syslog.example.com"
    port       = 514
    protocol   = "TLS"
    format     = "RFC5424"
    facility   = "local1"
    tls_verify = true
  })
}
`, name, enabled, streams, minSeverity)
}

func testAccLogDestinationWebhookConfig(name, method string) string {
	return fmt.Sprintf(`
resource "mistershell_log_destination" "test" {
  name    = %q
  type    = "webhook"
  streams = ["security"]

  config = jsonencode({
    type            = "webhook"
    url             = "https://hooks.example.com/ingest"
    method          = %q
    body_format     = "raw"
    timeout_seconds = 5
    tls_verify      = true
    auth = {
      type  = "bearer"
      token = "tfacc-secret-bearer-token"
    }
  })
}
`, name, method)
}

func testAccLogDestinationDataSourceConfig(name string) string {
	return fmt.Sprintf(`
resource "mistershell_log_destination" "test" {
  name         = %q
  type         = "syslog"
  streams      = ["api"]
  min_severity = "low"

  config = jsonencode({
    type     = "syslog"
    host     = "syslog.example.com"
    port     = 514
    protocol = "UDP"
    format   = "RFC5424"
  })
}

data "mistershell_log_destination" "by_id" {
  id = mistershell_log_destination.test.id
}

data "mistershell_log_destination" "by_name" {
  name = mistershell_log_destination.test.name
}
`, name)
}

func testAccSettingConfig(value int) string {
	return fmt.Sprintf(`
resource "mistershell_setting" "test" {
  key   = %q
  value = jsonencode(%d)
}

data "mistershell_setting" "test" {
  key        = %q
  depends_on = [mistershell_setting.test]
}
`, settingTestKey, value, settingTestKey)
}
