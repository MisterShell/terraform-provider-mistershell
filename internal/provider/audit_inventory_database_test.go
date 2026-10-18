package provider_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"terraform-provider-mistershell/internal/client"
)

// The two disposable PostgreSQL targets have different host:port identities,
// allowing the real worker to offer a changed external identity.
func TestAccAuditInventoryDatabaseMetadataAndIdentity(t *testing.T) {
	testAccPreCheck(t)
	host, firstPort := auditPostgresFixture(t, "first")
	_, secondPort := auditPostgresFixture(t, "second")
	name := acctestPrefix + "audit-database"
	root := discoverRootLocationID(t)
	c := testAccClient()
	ctx := context.Background()
	config := func(port int, metadata, evidence string) string {
		return fmt.Sprintf(`
resource "mistershell_location" "audit_db" {
 name=%q
 parent_id=%d
}
resource "mistershell_credential" "audit_db" {
 name=%q
 credential_type="username_password"
 credential_data={username="tfacc",password="tfacc-database-password"}
}
resource "mistershell_resource" "audit_db" {
 name=%q
 resource_type="database_postgres"
 location_id=mistershell_location.audit_db.id
 credential_id=mistershell_credential.audit_db.id
 connector_data={host=%q,port=%d,database="tfacc"}
 extra_data=%s
 is_enabled=false
 %s
}
`, name, root, name, name, host, port, metadata, evidence)
	}
	var id int64
	var originalIdentity string
	var observedIdentity string
	first := config(firstPort, `{databases=["tfacc"]}`, "")
	updatedMetadata := config(firstPort, `{databases=["postgres"]}`, "")
	steps := []resource.TestStep{{Config: first}, {Config: updatedMetadata}, {Config: updatedMetadata}, {Config: updatedMetadata, PlanOnly: true}, {Config: updatedMetadata}, {Config: updatedMetadata}}
	metadataCheck := func(want []string) error {
		current, err := c.GetNetworkResource(ctx, id)
		if err != nil {
			return err
		}
		var extra struct {
			Databases []string `json:"databases"`
		}
		if err := json.Unmarshal(current.ExtraData, &extra); err != nil {
			return err
		}
		if strings.Join(extra.Databases, "|") != strings.Join(want, "|") {
			return fmt.Errorf("writable metadata was not stored: got %v want %v", extra.Databases, want)
		}
		return nil
	}
	steps[0].Check = func(s *terraform.State) error {
		var err error
		id, err = strconv.ParseInt(s.RootModule().Resources["mistershell_resource.audit_db"].Primary.ID, 10, 64)
		if err != nil {
			return err
		}
		current, err := c.GetNetworkResource(ctx, id)
		if err != nil {
			return err
		}
		if current.ExternalID == nil || *current.ExternalID == "" {
			return fmt.Errorf("PostgreSQL discovery did not establish an identity")
		}
		originalIdentity = *current.ExternalID
		return metadataCheck([]string{"tfacc"})
	}
	steps[1].Check = func(_ *terraform.State) error {
		if err := metadataCheck([]string{"postgres"}); err != nil {
			return err
		}
		current, err := c.GetNetworkResource(ctx, id)
		if err != nil {
			return err
		}
		payload, _ := json.Marshal(map[string]any{"resource_name": name, "credential_id": *current.CredentialID, "location_id": *current.LocationID, "connector_data": map[string]any{"host": host, "port": secondPort, "database": "tfacc"}})
		var dispatched struct {
			TaskID int64 `json:"task_id"`
		}
		if err := auditInventoryRequest(c, "POST", fmt.Sprintf("/api/v1/resources/%d/verify", id), payload, &dispatched); err != nil {
			return err
		}
		var task client.WorkerTaskDetailResponse
		deadline := time.Now().Add(90 * time.Second)
		for {
			if err := auditInventoryRequest(c, "GET", fmt.Sprintf("/api/v1/worker-tasks/%d", dispatched.TaskID), nil, &task); err != nil {
				return err
			}
			if task.Status == "success" || task.Status == "failed" {
				break
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("database edit check did not finish")
			}
			time.Sleep(time.Second)
		}
		var result struct {
			ExternalID   string `json:"external_id"`
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
		for _, issue := range result.Verification.Issues {
			if issue.Action.Kind == "accept_identity" && issue.Action.Field == "external_id" {
				observedIdentity = issue.Observed
			}
		}
		if observedIdentity == "" || observedIdentity == originalIdentity || observedIdentity != result.ExternalID {
			return fmt.Errorf("completed PostgreSQL check did not offer the new database endpoint identity")
		}
		before, err := c.GetNetworkResource(ctx, id)
		if err != nil {
			return err
		}
		if before.ExternalID == nil || *before.ExternalID != originalIdentity {
			return fmt.Errorf("check automatically accepted changed database endpoint identity")
		}
		evidence := fmt.Sprintf("check_task_id=%d\naccepted_identity=true", dispatched.TaskID)
		final := config(secondPort, `{databases=["postgres"]}`, evidence)
		steps[2].Config = final
		steps[3].Config = final
		steps[4].Config = config(secondPort, `{databases=[]}`, evidence)
		steps[5].Config = config(secondPort, `{}`, evidence)
		return nil
	}
	steps[2].Check = func(s *terraform.State) error {
		if s.RootModule().Resources["mistershell_resource.audit_db"].Primary.ID != strconv.FormatInt(id, 10) {
			return fmt.Errorf("accepting identity replaced the resource")
		}
		current, err := c.GetNetworkResource(ctx, id)
		if err != nil {
			return err
		}
		if current.ExternalID == nil || *current.ExternalID != observedIdentity {
			return fmt.Errorf("explicit identity acceptance did not save reviewed database endpoint identity")
		}
		return metadataCheck([]string{"postgres"})
	}
	steps[4].Check = func(_ *terraform.State) error { return metadataCheck(nil) }
	steps[5].Check = func(_ *terraform.State) error {
		current, err := c.GetNetworkResource(ctx, id)
		if err != nil {
			return err
		}
		var extra map[string]any
		if err := json.Unmarshal(current.ExtraData, &extra); err != nil {
			return err
		}
		if _, ok := extra["databases"]; ok {
			return fmt.Errorf("empty metadata did not remove writable databases key")
		}
		return nil
	}
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, CheckDestroy: testAccCheckAllDestroyed, Steps: steps})
}

func auditPostgresFixture(t *testing.T, suffix string) (string, int) {
	t.Helper()
	image := os.Getenv("MISTERSHELL_ACC_POSTGRES_IMAGE")
	if image == "" {
		image = "postgres:16-alpine"
	}
	bindHost := "127.0.0.1"
	if address := os.Getenv("MISTERSHELL_ACC_GENERIC_SSH_BIND_ADDRESS"); address != "" {
		var err error
		bindHost, _, err = net.SplitHostPort(address)
		if err != nil {
			t.Fatal(err)
		}
	}
	host := os.Getenv("MISTERSHELL_ACC_GENERIC_SSH_HOST")
	if host == "" {
		host = "127.0.0.1"
	}
	name := acctestPrefix + "audit-postgres-" + suffix
	args := []string{"run", "--rm", "-d", "--name", name, "-p", net.JoinHostPort(bindHost, "") + ":5432", "-e", "POSTGRES_USER=tfacc", "-e", "POSTGRES_PASSWORD=tfacc-database-password", "-e", "POSTGRES_DB=tfacc"}
	if owner := os.Getenv("MISTERSHELL_ACC_RUN_OWNER"); owner != "" {
		args = append(args, "--label", "mistershell.terraform.acceptance.owner="+owner)
	}
	args = append(args, image)
	command := exec.Command("docker", args...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("start isolated PostgreSQL target: %v: %s", err, output)
	}
	t.Cleanup(func() {
		if output, err := exec.Command("docker", "rm", "-f", name).CombinedOutput(); err != nil {
			t.Errorf("remove PostgreSQL fixture: %v: %s", err, output)
		}
	})
	output, err := exec.Command("docker", "port", name, "5432/tcp").Output()
	if err != nil {
		t.Fatal(err)
	}
	_, portString, err := net.SplitHostPort(strings.Split(strings.TrimSpace(string(output)), "\n")[0])
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portString)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Minute)
	for {
		cmd := exec.Command("docker", "exec", "-e", "PGPASSWORD=tfacc-database-password", name, "psql", "-h", "127.0.0.1", "-U", "tfacc", "-d", "tfacc", "-c", "SELECT 1")
		if err := cmd.Run(); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("PostgreSQL fixture did not become ready")
		}
		time.Sleep(time.Second)
	}
	return host, port
}
