package provider_test

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"strconv"
	"terraform-provider-mistershell/internal/client"
	"testing"
)

func TestAccAuditVaultImportUnknownSecretPreserved(t *testing.T) {
	testAccPreCheck(t)
	name := acctestPrefix + "vault-import-secret"
	c := testAccClient()
	row, err := c.CreateExternalVault(context.Background(), client.ExternalVaultCreateInput{Name: name, Mnemonic: name, VaultType: "hashicorp", Config: []byte(`{"address":"https://vault.example.invalid","auth_method":"token","token":"audit-import-token","namespace":"old"}`)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := c.DeleteExternalVault(context.Background(), row.ID); err != nil && !client.IsNotFound(err) {
			t.Error(err)
		}
	})

	config := func(token, namespace, description string) string {
		return fmt.Sprintf(`
resource "mistershell_external_vault" "audit" {
 name=%q
 mnemonic=%q
 vault_type="hashicorp"
 description=%q
 config={address="https://vault.example.invalid",auth_method="token",token=%s,namespace=%q}
}
`, name, name, description, token, namespace)
	}
	checkPreserved := func(s *terraform.State) error {
		state := s.RootModule().Resources["mistershell_external_vault.audit"].Primary
		if state.Attributes["config.token"] != "" {
			return fmt.Errorf("unknown imported secret must remain null in Terraform state")
		}
		id, err := strconv.ParseInt(state.ID, 10, 64)
		if err != nil {
			return err
		}
		row, err := testAccClient().GetExternalVault(context.Background(), id)
		if err != nil {
			return err
		}
		var fields map[string]any
		if err = json.Unmarshal(row.Config, &fields); err != nil {
			return err
		}
		if fields["token"] != "****" {
			return fmt.Errorf("unrelated update cleared the stored secret")
		}
		return nil
	}
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, CheckDestroy: testAccCheckAllDestroyed, Steps: []resource.TestStep{
		{Config: config("null", "old", ""), ResourceName: "mistershell_external_vault.audit", ImportStateId: strconv.FormatInt(row.ID, 10), ImportState: true, ImportStatePersist: true, ImportStateCheck: func(states []*terraform.InstanceState) error {
			if len(states) != 1 {
				return fmt.Errorf("expected one imported vault")
			}
			if states[0].Attributes["config.token"] != "" {
				return fmt.Errorf("import stored a mask instead of null")
			}
			if states[0].Attributes["config.address"] != "https://vault.example.invalid" {
				return fmt.Errorf("import lost readable config")
			}
			return nil
		}},
		{Config: config("null", "old", "metadata update"), Check: checkPreserved},
		{Config: config("null", "new", "metadata update"), Check: checkPreserved},
		{Config: config("null", "new", "metadata update"), PlanOnly: true, ExpectNonEmptyPlan: false},
	}})
}
