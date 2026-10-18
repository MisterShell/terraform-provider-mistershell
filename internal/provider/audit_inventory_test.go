package provider_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"terraform-provider-mistershell/internal/client"
)

func TestAccAuditInventoryWorkerRefreshClearImport(t *testing.T) {
	testAccPreCheck(t)
	name := acctestPrefix + "audit-worker"
	root := discoverRootLocationID(t)
	config := func(data string) string {
		return fmt.Sprintf(`resource "mistershell_worker" "audit" {
 name=%q
 location_id=%d
 %s
 }`, name, root, data)
	}
	var id int64
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, CheckDestroy: testAccCheckAllDestroyed, Steps: []resource.TestStep{
		{Config: config(`config={audit="configured"}
 config_schema={type="object"}`), Check: func(s *terraform.State) error {
			var err error
			id, err = strconv.ParseInt(s.RootModule().Resources["mistershell_worker.audit"].Primary.ID, 10, 64)
			return err
		}},
		{ResourceName: "mistershell_worker.audit", ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: []string{"token"}},
		{PreConfig: func() {
			if _, err := testAccClient().UpdateWorker(context.Background(), id, client.WorkerUpdateInput{Config: json.RawMessage(`{"audit":"drifted"}`)}); err != nil {
				t.Fatal(err)
			}
		}, Config: config(`config={audit="configured"}
 config_schema={type="object"}`), Check: func(_ *terraform.State) error {
			v, err := testAccClient().GetWorker(context.Background(), id)
			if err != nil {
				return err
			}
			if string(v.Config) != `{"audit":"configured"}` && !jsonRawEqual(v.Config, json.RawMessage(`{"audit":"configured"}`)) {
				return fmt.Errorf("worker drift was not repaired")
			}
			return nil
		}},
		{Config: config(""), Check: func(_ *terraform.State) error {
			v, err := testAccClient().GetWorker(context.Background(), id)
			if err != nil {
				return err
			}
			if len(v.Config) > 0 && string(v.Config) != "null" {
				return fmt.Errorf("config not cleared: %s", v.Config)
			}
			if len(v.ConfigSchema) > 0 && string(v.ConfigSchema) != "null" {
				return fmt.Errorf("config_schema not cleared: %s", v.ConfigSchema)
			}
			return nil
		}},
	}})
}

func TestAccAuditInventoryCredentialReadableDriftAndType(t *testing.T) {
	testAccPreCheck(t)
	name := acctestPrefix + "audit-credential"
	var id int64
	config := func(kind, data string) string {
		return fmt.Sprintf(`resource "mistershell_credential" "audit" {
 name=%q
 credential_type=%q
 credential_data=%s
 }`, name, kind, data)
	}
	initial := config("username_password", `{username="configured",password="audit-secret"}`)
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, CheckDestroy: testAccCheckAllDestroyed, Steps: []resource.TestStep{
		{Config: initial, Check: func(s *terraform.State) error {
			var err error
			id, err = strconv.ParseInt(s.RootModule().Resources["mistershell_credential.audit"].Primary.ID, 10, 64)
			return err
		}},
		{PreConfig: func() {
			if _, err := testAccClient().UpdateCredential(context.Background(), id, client.CredentialUpdateInput{CredentialData: json.RawMessage(`{"username":"remote","password":"****"}`)}); err != nil {
				t.Fatal(err)
			}
		}, Config: initial, Check: func(_ *terraform.State) error {
			v, err := testAccClient().GetCredential(context.Background(), id)
			if err != nil {
				return err
			}
			var data map[string]any
			json.Unmarshal(v.CredentialData, &data)
			if data["username"] != "configured" {
				return fmt.Errorf("readable credential drift not repaired")
			}
			return nil
		}},
		{Config: config("aws_credentials", `{access_key="audit-access",secret_key="audit-secret"}`), Check: func(s *terraform.State) error {
			current := s.RootModule().Resources["mistershell_credential.audit"].Primary.ID
			if current != strconv.FormatInt(id, 10) {
				return fmt.Errorf("credential type change replaced ID %d with %s", id, current)
			}
			return nil
		}},
		{ResourceName: "mistershell_credential.audit", ImportState: true, ImportStateCheck: func(states []*terraform.InstanceState) error {
			for _, s := range states {
				if s.Attributes["credential_data.access_key"] == "audit-access" {
					return nil
				}
			}
			return fmt.Errorf("import omitted readable credential fields")
		}},
	}})
}

func TestAccAuditInventoryLocationRejectOrgCoordinates(t *testing.T) {
	testAccPreCheck(t)
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, Steps: []resource.TestStep{{Config: fmt.Sprintf(`resource "mistershell_location" "audit" {
 name=%q
 kind="org"
 parent_id=%d
 latitude=12
 longitude=34
 }`, acctestPrefix+"audit-org", discoverRootLocationID(t)), PlanOnly: true, ExpectError: regexp.MustCompile("Invalid organizational coordinates")}}})
}

func TestAccAuditInventoryTagUnmanagedAndConnectorImport(t *testing.T) {
	testAccSSHResourcePreCheck(t)
	name := acctestPrefix + "audit-inventory"
	var tagID, resourceID int64
	config := func(suffix string) string {
		return strings.Replace(testAccSSHResourceConfig(name, "generic_ssh"), `resource "mistershell_resource" "test" {`, `resource "mistershell_resource" "test" {`+"\n extra_data = {}\n", 1) + fmt.Sprintf(`
resource "mistershell_tag" "audit" {
 name=%q
 }`, name+suffix)
	}
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, CheckDestroy: testAccCheckAllDestroyed, Steps: []resource.TestStep{
		{Config: config("-tag"), Check: func(s *terraform.State) error {
			var err error
			tagID, err = strconv.ParseInt(s.RootModule().Resources["mistershell_tag.audit"].Primary.ID, 10, 64)
			if err != nil {
				return err
			}
			resourceID, err = strconv.ParseInt(s.RootModule().Resources["mistershell_resource.test"].Primary.ID, 10, 64)
			return err
		}},
		{PreConfig: func() {
			if err := testAccClient().SetTagAssignments(context.Background(), tagID, []int64{resourceID}); err != nil {
				t.Fatal(err)
			}
			current, err := testAccClient().GetNetworkResource(context.Background(), resourceID)
			if err != nil {
				t.Fatal(err)
			}
			var connector map[string]any
			if err := json.Unmarshal(current.ConnectorData, &connector); err != nil {
				t.Fatal(err)
			}
			delete(connector, "host_key_fingerprint")
			connector["port"] = 1
			raw, _ := json.Marshal(connector)
			if _, err := testAccClient().UpdateNetworkResource(context.Background(), resourceID, client.NetworkResourceUpdateInput{ConnectorData: raw}); err != nil {
				t.Fatal(err)
			}
		}, Config: config("-renamed"), Check: func(_ *terraform.State) error {
			ids, err := testAccClient().GetTagResourceIDs(context.Background(), tagID)
			if err != nil {
				return err
			}
			if len(ids) != 1 || ids[0] != resourceID {
				return fmt.Errorf("unmanaged membership cleared")
			}
			current, err := testAccClient().GetNetworkResource(context.Background(), resourceID)
			if err != nil {
				return err
			}
			var connector map[string]any
			json.Unmarshal(current.ConnectorData, &connector)
			expected, _ := strconv.Atoi(testAccSSHPort())
			if connector["port"] != float64(expected) {
				return fmt.Errorf("readable connector drift was not repaired")
			}
			return nil
		}},
		{ResourceName: "mistershell_resource.test", ImportState: true, ImportStateCheck: func(states []*terraform.InstanceState) error {
			for _, s := range states {
				if s.Attributes["connector_data.host"] != "" {
					return nil
				}
			}
			return fmt.Errorf("import omitted readable connector_data")
		}},
	}})
}

func TestAccAuditInventoryDefaultWorkerConfigDestroyPreserves(t *testing.T) {
	testAccPreCheck(t)
	ctx := context.Background()
	c := testAccClient()
	workers, err := c.ListWorkers(ctx, client.WorkerListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	var original *client.WorkerResponse
	for _, w := range workers {
		if w.IsDefault {
			original, err = c.GetWorker(ctx, w.ID)
			if err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	if original == nil {
		t.Fatal("backend has no default worker")
	}
	restore := original.Config
	if len(restore) == 0 {
		restore = json.RawMessage(`null`)
	}
	defer func() {
		if _, err := c.UpdateWorker(ctx, original.ID, client.WorkerUpdateInput{Config: restore}); err != nil {
			t.Errorf("restore default worker config: %v", err)
		}
	}()
	settings := map[string]any{}
	if len(original.Config) > 0 && string(original.Config) != "null" {
		if err := json.Unmarshal(original.Config, &settings); err != nil {
			t.Fatal(err)
		}
	}
	settings["terraform_audit_marker"] = acctestPrefix
	changed, _ := json.Marshal(settings)
	description := "null"
	if original.Description != nil {
		description = strconv.Quote(*original.Description)
	}
	schemaJSON := original.ConfigSchema
	if len(schemaJSON) == 0 {
		schemaJSON = json.RawMessage(`null`)
	}
	config := fmt.Sprintf(`resource "mistershell_worker" "audit_default" {
 name=%q
 description=%s
 location_id=%d
 is_enabled=%t
 config=jsondecode(%q)
 config_schema=jsondecode(%q)
 }
 import {
 to=mistershell_worker.audit_default
 id=%q
 }`, original.Name, description, original.LocationID, original.IsEnabled, string(changed), string(schemaJSON), strconv.FormatInt(original.ID, 10))
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, CheckDestroy: func(_ *terraform.State) error {
		w, err := c.GetWorker(ctx, original.ID)
		if err != nil {
			return err
		}
		if !w.IsDefault || !jsonRawEqual(w.Config, changed) {
			return fmt.Errorf("default worker or its configured value did not survive destroy")
		}
		return nil
	}, Steps: []resource.TestStep{{Config: config, Check: resource.TestCheckResourceAttr("mistershell_worker.audit_default", "is_default", "true")}}})
}

func TestAccAuditInventoryExplicitTrustEvidence(t *testing.T) {
	testAccPreCheck(t)
	ctx := context.Background()
	c := testAccClient()
	host, port, user, password := startGenericSSHFixture(t)
	nextHost, nextPort, _, _ := startGenericSSHFixture(t)
	name := acctestPrefix + "audit-trust"
	root := discoverRootLocationID(t)
	initial := testAccSSHTargetConfig(name, "generic_ssh", root, host, port, user, password)
	changed := testAccSSHTargetConfig(name, "generic_ssh", root, nextHost, nextPort, user, password)
	var id int64
	var originalPin string
	steps := []resource.TestStep{{Config: initial}, {Config: changed}, {Config: changed, PlanOnly: true}}
	steps[0].Check = func(s *terraform.State) error {
		var err error
		id, err = strconv.ParseInt(s.RootModule().Resources["mistershell_resource.test"].Primary.ID, 10, 64)
		if err != nil {
			return err
		}
		current, err := c.GetNetworkResource(ctx, id)
		if err != nil {
			return err
		}
		var connector map[string]any
		json.Unmarshal(current.ConnectorData, &connector)
		originalPin, _ = connector["host_key_fingerprint"].(string)
		payload, _ := json.Marshal(map[string]any{"resource_name": name, "credential_id": *current.CredentialID, "location_id": *current.LocationID, "connector_data": map[string]any{"host": nextHost, "port": nextPort}})
		var dispatched struct {
			TaskID int64 `json:"task_id"`
		}
		if err := auditInventoryRequest(c, "POST", fmt.Sprintf("/api/v1/resources/%d/verify", id), payload, &dispatched); err != nil {
			return err
		}
		var task client.WorkerTaskDetailResponse
		deadline := time.Now().Add(time.Minute)
		for {
			if err := auditInventoryRequest(c, "GET", fmt.Sprintf("/api/v1/worker-tasks/%d", dispatched.TaskID), nil, &task); err != nil {
				return err
			}
			if task.Status == "success" || task.Status == "failed" {
				break
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("edit check did not finish")
			}
			time.Sleep(time.Second)
		}
		// Review the completed immutable observation: accepting this test's new host
		// key is explicit, and the old saved key must remain unchanged before apply.
		var result struct {
			Verification struct {
				Issues []struct {
					Observed string `json:"observed"`
					Action   struct {
						Kind  string `json:"kind"`
						Field string `json:"field"`
					} `json:"action"`
				} `json:"issues"`
			} `json:"verification"`
		}
		if err := json.Unmarshal(task.Result, &result); err != nil {
			return err
		}
		observed := ""
		for _, issue := range result.Verification.Issues {
			if issue.Action.Kind == "accept_trust" && issue.Action.Field == "host_key_fingerprint" {
				observed = issue.Observed
			}
		}
		if observed == "" || observed == originalPin {
			return fmt.Errorf("check did not offer the expected changed host-key observation")
		}
		before, err := c.GetNetworkResource(ctx, id)
		if err != nil {
			return err
		}
		json.Unmarshal(before.ConnectorData, &connector)
		if connector["host_key_fingerprint"] != originalPin {
			return fmt.Errorf("check automatically trusted changed key")
		}
		evidence := fmt.Sprintf("  check_task_id = %d\n  accepted_trust_fields = [\"host_key_fingerprint\"]\n", dispatched.TaskID)
		final := strings.Replace(changed, `resource "mistershell_resource" "test" {`, `resource "mistershell_resource" "test" {`+"\n"+evidence, 1)
		steps[1].Config = final
		steps[2].Config = final
		steps[1].Check = func(_ *terraform.State) error {
			after, err := c.GetNetworkResource(ctx, id)
			if err != nil {
				return err
			}
			var got map[string]any
			json.Unmarshal(after.ConnectorData, &got)
			if got["host_key_fingerprint"] != observed {
				return fmt.Errorf("explicit acceptance did not save reviewed host key")
			}
			return nil
		}
		return nil
	}
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, CheckDestroy: testAccCheckAllDestroyed, Steps: steps})
}

func auditInventoryRequest(c *client.Client, method, path string, body []byte, target any) error {
	req, err := http.NewRequestWithContext(context.Background(), method, c.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s: HTTP %d", method, path, resp.StatusCode)
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return err
	}
	return json.Unmarshal(envelope.Data, target)
}

func TestAccAuditInventoryUserCredentialReadableDriftAndImport(t *testing.T) {
	testAccPreCheck(t)
	name := acctestPrefix + "audit-user-literals"
	var id int64
	config := fmt.Sprintf(`resource "mistershell_external_vault" "audit" {
 name=%q
 mnemonic=%q
 vault_type="hashicorp"
 config={address="https://vault.example.invalid",auth_method="token",token="audit-token"}
 }
 resource "mistershell_credential" "audit" {
 name=%q
 credential_type="username_password"
 credential_data={username="platform",password="platform-secret"}
 user_source="external"
 user_external_vault_id=mistershell_external_vault.audit.id
 user_external_vault_ref={base={path="audit"},fields={password="password"}}
 user_credential_data={username="configured-user"}
 }`, name, name, name)
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, CheckDestroy: testAccCheckAllDestroyed, Steps: []resource.TestStep{
		{Config: config, Check: func(s *terraform.State) error {
			var err error
			id, err = strconv.ParseInt(s.RootModule().Resources["mistershell_credential.audit"].Primary.ID, 10, 64)
			return err
		}},
		{PreConfig: func() {
			if _, err := testAccClient().UpdateCredential(context.Background(), id, client.CredentialUpdateInput{UserCredentialData: json.RawMessage(`{"username":"changed-user"}`)}); err != nil {
				t.Fatal(err)
			}
		}, Config: config, Check: func(_ *terraform.State) error {
			cred, err := testAccClient().GetCredential(context.Background(), id)
			if err != nil {
				return err
			}
			var data map[string]any
			json.Unmarshal(cred.UserCredentialData, &data)
			if data["username"] != "configured-user" {
				return fmt.Errorf("user audience readable drift was not repaired")
			}
			return nil
		}},
		{ResourceName: "mistershell_credential.audit", ImportState: true, ImportStateCheck: func(states []*terraform.InstanceState) error {
			for _, s := range states {
				if s.Attributes["user_credential_data.username"] == "configured-user" {
					return nil
				}
			}
			return fmt.Errorf("import omitted user audience literal fields")
		}},
	}})
}
