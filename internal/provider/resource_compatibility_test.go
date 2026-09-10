package provider_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"terraform-provider-mistershell/internal/client"
)

// Exercise the real backend's proof-derived pins and guarded PATCH behavior;
// a mock that preserves pins itself cannot establish API compatibility.
func TestAccNetworkResource_collectionAndSSHTrust(t *testing.T) {
	testAccPreCheck(t)
	for _, strict := range []bool{true, false} {
		t.Run(fmt.Sprintf("strict_%t", strict), func(t *testing.T) {
			host, port, username, password := startGenericSSHFixture(t)
			name := fmt.Sprintf("%strust-%t", acctestPrefix, strict)
			base := testAccSSHTargetConfig(name, "generic_ssh", 1, host, port, username, password)
			base = strings.Replace(base, "  connector_data = {", fmt.Sprintf("  is_enabled = false\n  connector_data = {\n    strict_host_key = %t", strict), 1)
			updated := strings.Replace(base, "    port =", "    timeout = 45\n    port =", 1)
			var savedPin, savedID string
			check := func(initial bool) resource.TestCheckFunc {
				return func(state *terraform.State) error {
					rs := state.RootModule().Resources["mistershell_resource.test"]
					id, err := strconv.ParseInt(rs.Primary.ID, 10, 64)
					if err != nil {
						return err
					}
					c := testAccClient()
					actual, err := c.GetNetworkResource(context.Background(), id)
					if err != nil {
						return err
					}
					var connector map[string]any
					if err := json.Unmarshal(actual.ConnectorData, &connector); err != nil {
						return err
					}
					pin, _ := connector["host_key_fingerprint"].(string)
					if !strings.HasPrefix(pin, "SHA256:") || actual.IsEnabled || connector["strict_host_key"] != strict {
						return fmt.Errorf("unexpected live collection/trust state: enabled=%t connector=%v", actual.IsEnabled, connector)
					}
					if initial {
						savedPin, savedID = pin, rs.Primary.ID
					} else if pin != savedPin || rs.Primary.ID != savedID || connector["timeout"] != float64(45) {
						return fmt.Errorf("connector edit did not preserve ID/pin or apply timeout")
					}
					if _, present := rs.Primary.Attributes["connector_data.host_key_fingerprint"]; present {
						return fmt.Errorf("server-owned pin leaked into configured connector_data")
					}
					// A guessed replacement must be rejected even in non-strict mode.
					connector["host_key_fingerprint"] = "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
					payload, err := json.Marshal(connector)
					if err != nil {
						return err
					}
					_, err = c.UpdateNetworkResource(context.Background(), id, client.NetworkResourceUpdateInput{ConnectorData: payload})
					if err == nil || !strings.Contains(err.Error(), "explicit acceptance of a resource check") {
						return fmt.Errorf("expected evidence-required error for pin change, got %v", err)
					}
					return nil
				}
			}
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				CheckDestroy:             testAccCheckAllDestroyed,
				Steps: []resource.TestStep{
					{Config: base, Check: check(true)},
					{Config: updated, Check: check(false)},
					{Config: updated, PlanOnly: true, ExpectNonEmptyPlan: false},
				},
			})
		})
	}
}
