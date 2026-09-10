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

func TestAccRole_currentPermissions(t *testing.T) {
	testAccPreCheck(t)
	config := testAccRolePermissionsConfig(acctestPrefix+"current-permissions", `["app.fabric.read", "app.fabric.write", "app.fabric.delete", "app.resources.execute", "app.ai.execute", "app.facts.write", "app.changelog.delete"]`)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			{Config: config, Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("mistershell_role.test", "permissions.#", "7"),
				func(state *terraform.State) error {
					id, err := strconv.ParseInt(state.RootModule().Resources["mistershell_role.test"].Primary.ID, 10, 64)
					if err != nil {
						return err
					}
					for _, retired := range []string{"app.workers.read", "app.workers.write", "app.workers.delete", "app.proxies.read", "app.proxies.write", "app.proxies.delete", "app.resources.test", "app.resources.session", "app.snapshots.create", "app.ai.chat", "app.ai.quick_assist", "app.facts.manage", "app.changelog.write"} {
						err := testAccClient().AddRolePermission(context.Background(), id, retired)
						if err == nil || !strings.Contains(err.Error(), "Invalid permission") {
							return fmt.Errorf("retired permission %q: expected rejection, got %v", retired, err)
						}
					}
					return nil
				},
			)},
			{Config: config, PlanOnly: true, ExpectNonEmptyPlan: false},
		},
	})
}

func TestAccSetting_sessionInactivityLimit(t *testing.T) {
	testAccPreCheck(t)
	const key = "session_inactivity_timeout_minutes"
	c := testAccClient()
	original, err := c.GetSetting(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	originalValue := append(json.RawMessage(nil), original.Value...)
	t.Cleanup(func() {
		restored, err := c.UpdateSetting(context.Background(), key, client.SettingUpdateInput{Value: originalValue})
		if err != nil {
			t.Errorf("restoring original inactivity timeout: %v", err)
		} else if !jsonRawEqual(restored.Value, originalValue) {
			t.Errorf("restored inactivity timeout = %s, want %s", restored.Value, originalValue)
		}
	})
	config := `resource "mistershell_setting" "test" {
  key = "session_inactivity_timeout_minutes"
  value = 240
}`
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			{Config: config, Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("mistershell_setting.test", "value", "240"),
				func(_ *terraform.State) error {
					_, err := c.UpdateSetting(context.Background(), key, client.SettingUpdateInput{Value: json.RawMessage(`241`)})
					if err == nil || !strings.Contains(err.Error(), "422") {
						return fmt.Errorf("expected HTTP 422 above 240 minutes, got %v", err)
					}
					return nil
				},
			)},
			{Config: config, PlanOnly: true, ExpectNonEmptyPlan: false},
		},
	})
}
