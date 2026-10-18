package provider_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"regexp"
	"terraform-provider-mistershell/internal/client"
	"terraform-provider-mistershell/internal/resources"
	"testing"
)

// Uploading/activating a catalog is allowed only with the explicit disposable
// fixture flag. Existing audit catalog rows can be reused on later runs.
func testAccAuditSensorCatalog(t *testing.T, c *client.Client) {
	t.Helper()
	ctx := context.Background()
	row, err := c.GetSensorCatalogRule(ctx, 1, 9900001)
	if err == nil {
		if row.Message == nil || *row.Message != "Terraform audit catalog rule" {
			t.Fatal("sensor audit SID is already owned by another rule")
		}
		return
	}
	if !client.IsNotFound(err) {
		t.Fatal(err)
	}
	if os.Getenv("MISTERSHELL_ACC_SENSOR_RULESET_UPLOAD") != "1" {
		t.Fatal("sensor audit catalog missing: set MISTERSHELL_ACC_SENSOR_RULESET_UPLOAD=1 only on a disposable backend to upload/activate the test ruleset")
	}
	rules := []byte("alert tcp any any -> any any (msg:\"Terraform audit catalog rule\"; sid:9900001; rev:1;)\nalert udp any any -> any any (msg:\"Terraform audit unrelated rule\"; sid:9900002; rev:1;)\n")
	testAccUploadSensorRules(t, c, rules)
	if _, err = c.GetSensorCatalogRule(ctx, 1, 9900001); err != nil {
		t.Fatal(err)
	}
}
func testAccSensorHTTP(t *testing.T, c *client.Client, method, path string, body io.Reader, contentType string) json.RawMessage {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), method, c.BaseURL+"/api/v1/sensors"+path, body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", contentType)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode >= 300 {
		t.Fatalf("sensor fixture %s %s returned %d: %s", method, path, resp.StatusCode, raw)
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err = json.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(err)
	}
	return envelope.Data
}
func TestAccAuditSensorRuleOverrideLifecycle(t *testing.T) {
	testAccPreCheck(t)
	c := testAccClient()
	testAccAuditSensorCatalog(t, c)
	ctx := context.Background()
	originalActive := testAccSensorHTTP(t, c, http.MethodGet, "/rulesets", nil, "application/json")
	if err := c.SetSensorRuleOverride(ctx, 1, 9900002, "disable"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := c.ResetSensorRuleOverride(ctx, 1, 9900002); err != nil {
			t.Error(err)
		}
	})
	config := func(action string) string {
		return fmt.Sprintf(`
resource "mistershell_sensor_rule_override" "audit" {
 sid=9900001
 action=%q
}
data "mistershell_sensor_rule_override" "audit" {
 gid=mistershell_sensor_rule_override.audit.gid
 sid=mistershell_sensor_rule_override.audit.sid
}
`, action)
	}
	checkStaged := func(_ *terraform.State) error {
		row, err := c.GetSensorCatalogRule(ctx, 1, 9900002)
		if err != nil {
			return err
		}
		if row.OverrideAction == nil || *row.OverrideAction != "disable" || !row.Pending {
			return fmt.Errorf("unrelated staged override was changed or applied")
		}
		active := testAccSensorHTTP(t, c, http.MethodGet, "/rulesets", nil, "application/json")
		if !bytes.Equal(originalActive, active) {
			return fmt.Errorf("staging unexpectedly changed active ruleset")
		}
		return nil
	}
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, CheckDestroy: func(s *terraform.State) error {
		row, err := c.GetSensorCatalogRule(ctx, 1, 9900001)
		if err != nil {
			return err
		}
		if row.OverrideAction != nil || !row.EffectiveEnabled {
			return fmt.Errorf("destroy did not reset override to default")
		}
		return checkStaged(s)
	}, Steps: []resource.TestStep{
		{Config: config("disable"), Check: resource.ComposeAggregateTestCheckFunc(resource.TestCheckResourceAttr("mistershell_sensor_rule_override.audit", "id", "1:9900001"), resource.TestCheckResourceAttr("mistershell_sensor_rule_override.audit", "pending", "true"), resource.TestCheckResourceAttr("data.mistershell_sensor_rule_override.audit", "action", "disable"), resource.TestCheckResourceAttr("data.mistershell_sensor_rule_override.audit", "effective_enabled", "false"), checkStaged)},
		{ResourceName: "mistershell_sensor_rule_override.audit", ImportState: true, ImportStateVerify: true},
		{Config: config("enable"), Check: resource.TestCheckResourceAttr("mistershell_sensor_rule_override.audit", "effective_enabled", "true")},
		{PreConfig: func() {
			if err := c.SetSensorRuleOverride(ctx, 1, 9900001, "disable"); err != nil {
				t.Fatal(err)
			}
		}, Config: config("enable"), PlanOnly: true, ExpectNonEmptyPlan: true},
		{Config: config("enable"), Check: checkStaged},
		{PreConfig: func() {
			if err := c.ResetSensorRuleOverride(ctx, 1, 9900001); err != nil {
				t.Fatal(err)
			}
		}, Config: config("disable"), Check: resource.TestCheckResourceAttr("mistershell_sensor_rule_override.audit", "action", "disable")},
	}})
}

func testAccUploadSensorRules(t *testing.T, c *client.Client, rules []byte) int64 {
	t.Helper()

	var bundle bytes.Buffer
	gz := gzip.NewWriter(&bundle)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "audit.rules", Mode: 0600, Size: int64(len(rules))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(rules); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", "audit.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(bundle.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	data := testAccSensorHTTP(t, c, http.MethodPost, "/rulesets/upload", &body, mw.FormDataContentType())
	var uploaded struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(data, &uploaded); err != nil {
		t.Fatal(err)
	}
	testAccSensorHTTP(t, c, http.MethodPost, fmt.Sprintf("/rulesets/%d/activate", uploaded.ID), nil, "application/json")
	return uploaded.ID
}

// This calls the real resource lifecycle against two real uploaded rulesets.
// Framework state is inspected directly because refresh deliberately returns an
// error; a normal Terraform acceptance step cannot then run automatic cleanup.
func TestAccAuditSensorRuleCatalogDisappearance(t *testing.T) {
	testAccPreCheck(t)
	c := testAccClient()
	testAccAuditSensorCatalog(t, c)
	ctx := context.Background()
	var rulesets []struct {
		ID     int64 `json:"id"`
		Active bool  `json:"is_active"`
	}
	if err := json.Unmarshal(testAccSensorHTTP(t, c, http.MethodGet, "/rulesets", nil, "application/json"), &rulesets); err != nil {
		t.Fatal(err)
	}
	var originalID int64
	for _, row := range rulesets {
		if row.Active {
			originalID = row.ID
			break
		}
	}
	if originalID == 0 {
		t.Fatal("no active test ruleset")
	}
	t.Cleanup(func() {
		testAccSensorHTTP(t, c, http.MethodPost, fmt.Sprintf("/rulesets/%d/activate", originalID), nil, "application/json")
		if err := c.ResetSensorRuleOverride(ctx, 1, 9900001); err != nil {
			t.Error(err)
		}
	})
	r := resources.NewSensorRuleOverrideResource()
	var configured frameworkresource.ConfigureResponse
	r.(frameworkresource.ResourceWithConfigure).Configure(ctx, frameworkresource.ConfigureRequest{ProviderData: c}, &configured)
	if configured.Diagnostics.HasError() {
		t.Fatal(configured.Diagnostics)
	}
	var schemaResponse frameworkresource.SchemaResponse
	r.Schema(ctx, frameworkresource.SchemaRequest{}, &schemaResponse)
	plan := tfsdk.Plan{Schema: schemaResponse.Schema}
	model := resources.SensorRuleOverrideModel{GID: types.Int64Value(1), SID: types.Int64Value(9900001), Action: types.StringValue("disable")}
	if d := plan.Set(ctx, &model); d.HasError() {
		t.Fatal(d)
	}
	created := frameworkresource.CreateResponse{State: tfsdk.State{Schema: schemaResponse.Schema}}
	r.Create(ctx, frameworkresource.CreateRequest{Plan: plan}, &created)
	if created.Diagnostics.HasError() {
		t.Fatal(created.Diagnostics)
	}
	testAccUploadSensorRules(t, c, []byte("alert udp any any -> any any (msg:\"Terraform audit replacement catalog\"; sid:9900002; rev:2;)\n"))
	if _, err := c.GetSensorCatalogRule(ctx, 1, 9900001); !client.IsNotFound(err) {
		t.Fatalf("catalog rule did not disappear: %v", err)
	}
	refreshed := frameworkresource.ReadResponse{State: created.State}
	r.Read(ctx, frameworkresource.ReadRequest{State: created.State}, &refreshed)
	if !refreshed.Diagnostics.HasError() {
		t.Fatal("missing catalog did not report uncertainty")
	}
	if refreshed.State.Raw.IsNull() || !refreshed.State.Raw.Equal(created.State.Raw) {
		t.Fatal("missing catalog discarded or changed override ownership")
	}
	var deleted frameworkresource.DeleteResponse
	r.Delete(ctx, frameworkresource.DeleteRequest{State: refreshed.State}, &deleted)
	if deleted.Diagnostics.HasError() {
		t.Fatal(deleted.Diagnostics)
	}
	testAccSensorHTTP(t, c, http.MethodPost, fmt.Sprintf("/rulesets/%d/activate", originalID), nil, "application/json")
	row, err := c.GetSensorCatalogRule(ctx, 1, 9900001)
	if err != nil {
		t.Fatal(err)
	}
	if row.OverrideAction != nil {
		t.Fatal("reset while rule was absent left the durable override behind")
	}
}

func TestAccAuditSensorRuleOverrideRequiresImport(t *testing.T) {
	testAccPreCheck(t)
	c := testAccClient()
	testAccAuditSensorCatalog(t, c)
	ctx := context.Background()
	if err := c.SetSensorRuleOverride(ctx, 1, 9900001, "enable"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := c.ResetSensorRuleOverride(ctx, 1, 9900001); err != nil {
			t.Error(err)
		}
	})
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, Steps: []resource.TestStep{{Config: `
resource "mistershell_sensor_rule_override" "audit" {
 gid=1
 sid=9900001
 action="disable"
}
`, ExpectError: regexp.MustCompile("already has an override; import")}}})
	row, err := c.GetSensorCatalogRule(ctx, 1, 9900001)
	if err != nil {
		t.Fatal(err)
	}
	if row.OverrideAction == nil || *row.OverrideAction != "enable" {
		t.Fatalf("failed create changed existing override: %+v", row)
	}
}
