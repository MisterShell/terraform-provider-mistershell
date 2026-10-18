package provider_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"testing"

	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"terraform-provider-mistershell/internal/apimanaged"
	"terraform-provider-mistershell/internal/client"
)

func TestAccAuditResourceVarsRequireImport(t *testing.T) {
	testAccPreCheck(t)
	var fixture auditConfigurationFixture
	for _, f := range auditConfigurationFixtures(t) {
		if f.name == "configure_resource_vars" {
			fixture = f
		}
	}
	c := testAccClient()
	ctx := context.Background()
	var rid, sid string
	config := fixture.support + `
resource "mistershell_configure_resource_vars" "audit" {
 resource_id=mistershell_resource.test.id
 stack_id=tonumber(mistershell_configure_stack.support.id)
 values={hostname="terraform"}
}`
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, Steps: []resource.TestStep{
		{Config: fixture.support, Check: func(s *terraform.State) error {
			rid = s.RootModule().Resources["mistershell_resource.test"].Primary.ID
			sid = s.RootModule().Resources["mistershell_configure_stack.support"].Primary.ID
			var r, st int64
			fmt.Sscan(rid, &r)
			fmt.Sscan(sid, &st)
			_, err := c.ConfigurationRequest(ctx, http.MethodPut, "/api/v1/configure/resource-vars", map[string]any{"resource_id": r, "stack_id": st, "values": map[string]any{"hostname": "operator-owned"}})
			return err
		}},
		{Config: config, ExpectError: regexp.MustCompile("import them before managing")},
		{Config: fixture.support, Check: func(_ *terraform.State) error {
			raw, err := c.ConfigurationRequest(ctx, http.MethodGet, "/api/v1/configure/resource-vars?resource_id="+rid+"&stack_id="+sid, nil)
			if err != nil {
				return err
			}
			var result struct {
				Values map[string]string `json:"values"`
			}
			if err = json.Unmarshal(raw, &result); err != nil {
				return err
			}
			if result.Values["hostname"] != "operator-owned" {
				return fmt.Errorf("existing variables were overwritten")
			}
			return nil
		}},
	}})
}

func TestAccAuditSourceParameterRemovalRejected(t *testing.T) {
	testAccPreCheck(t)
	config := func(params string) string {
		return fmt.Sprintf(`
resource "mistershell_sensor_source" "audit" {
 name=%q
 url="https://example.com/terraform-owned.rules"
 enabled=false
 parameters=%s
}`, acctestPrefix+"secret-source", params)
	}
	check := func(s *terraform.State) error {
		id := s.RootModule().Resources["mistershell_sensor_source.audit"].Primary.ID
		rows, err := testAccClient().ConfigurationList(context.Background(), "/api/v1/sensors/sources", nil)
		if err != nil {
			return err
		}
		for _, raw := range rows {
			var row struct {
				ID         int64             `json:"id"`
				Parameters map[string]string `json:"parameters"`
			}
			if err = json.Unmarshal(raw, &row); err != nil {
				return err
			}
			if fmt.Sprint(row.ID) == id {
				if row.Parameters["code"] != "****" || len(row.Parameters) != 1 {
					return fmt.Errorf("stored parameter key set changed")
				}
				return nil
			}
		}
		return fmt.Errorf("source missing")
	}
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, Steps: []resource.TestStep{
		{Config: config(`{code="initial-fixture-secret"}`), Check: check},
		{Config: config(`{code="updated-fixture-secret"}`), Check: check},
		{Config: strings.Replace(config(`{code="updated-fixture-secret"}`), `parameters={code="updated-fixture-secret"}`, "", 1), ExpectError: regexp.MustCompile("parameters cannot be omitted")},
		{Config: config(`{}`), ExpectError: regexp.MustCompile("parameter keys cannot be removed")},
		{Config: config(`{code=""}`), ExpectError: regexp.MustCompile("non-blank secret values")},
		{Config: config(`{code="updated-fixture-secret"}`), Check: check},
	}})
}

func TestAccAuditCustomRuleAutoModeRejected(t *testing.T) {
	testAccPreCheck(t)
	c := testAccClient()
	ctx := context.Background()
	testAccAuditSensorCatalog(t, c)
	old, err := c.GetSetting(ctx, "sensor_ruleset_update_mode")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, e := c.UpdateSetting(ctx, "sensor_ruleset_update_mode", client.SettingUpdateInput{Value: old.Value}); e != nil {
			t.Error(e)
		}
		if e := c.ResetSensorRuleOverride(ctx, 1, 9900002); e != nil {
			t.Error(e)
		}
	})
	if err = c.SetSensorRuleOverride(ctx, 1, 9900002, "disable"); err != nil {
		t.Fatal(err)
	}
	if _, err = c.UpdateSetting(ctx, "sensor_ruleset_update_mode", client.SettingUpdateInput{Value: json.RawMessage(`"auto"`)}); err != nil {
		t.Fatal(err)
	}
	before := testAccSensorHTTP(t, c, http.MethodGet, "/rulesets", nil, "application/json")
	config := fmt.Sprintf(`resource "mistershell_sensor_custom_rule" "audit" {
 name=%q
 kind="local"
 enabled=false
 content="alert tcp any any -> any any (msg:\"Terraform ownership test\"; sid:9900015; rev:1;)"
}`, acctestPrefix+"automatic-rejection")
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, Steps: []resource.TestStep{{Config: config, ExpectError: regexp.MustCompile("automatic or unknown ruleset mode")}}})
	setMode := func(mode string) {
		if _, err := c.UpdateSetting(ctx, "sensor_ruleset_update_mode", client.SettingUpdateInput{Value: json.RawMessage(auditHCL(mode))}); err != nil {
			t.Fatal(err)
		}
	}
	setMode("manual")
	var managedID string
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, Steps: []resource.TestStep{
		{Config: config, Check: func(s *terraform.State) error {
			managedID = s.RootModule().Resources["mistershell_sensor_custom_rule.audit"].Primary.ID
			return nil
		}},
		{Config: strings.Replace(config, "automatic-rejection", "automatic-updated", 1), PreConfig: func() { setMode("auto") }, ExpectError: regexp.MustCompile("automatic or unknown ruleset mode")},
		{Config: config, Destroy: true, ExpectError: regexp.MustCompile("automatic or unknown ruleset mode")},
		{Config: config, PreConfig: func() {
			rows, err := c.ConfigurationList(ctx, "/api/v1/sensors/custom-rules", nil)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, raw := range rows {
				var row struct {
					ID   int64  `json:"id"`
					Name string `json:"name"`
				}
				if err := json.Unmarshal(raw, &row); err != nil {
					t.Fatal(err)
				}
				if fmt.Sprint(row.ID) == managedID {
					found = true
					if row.Name != acctestPrefix+"automatic-rejection" {
						t.Fatal("rejected update changed custom rule")
					}
				}
			}
			if !found {
				t.Fatal("rejected destroy deleted custom rule")
			}
			setMode("manual")
		}},
	}})
	after := testAccSensorHTTP(t, c, http.MethodGet, "/rulesets", nil, "application/json")
	if !jsonRawEqual(before, after) {
		t.Fatal("rejected custom rule activated a ruleset")
	}
	row, err := c.GetSensorCatalogRule(ctx, 1, 9900002)
	if err != nil {
		t.Fatal(err)
	}
	if row.OverrideAction == nil || *row.OverrideAction != "disable" {
		t.Fatal("unrelated pending override changed")
	}
	rows, err := c.ConfigurationList(ctx, "/api/v1/sensors/custom-rules", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range rows {
		var row struct {
			Name string `json:"name"`
		}
		_ = json.Unmarshal(raw, &row)
		if row.Name == acctestPrefix+"automatic-rejection" {
			t.Fatal("rejected custom rule was created")
		}
	}
}

func TestAccAuditBuiltinImportsRejected(t *testing.T) {
	testAccPreCheck(t)
	c := testAccClient()
	ctx := context.Background()
	for _, d := range apimanaged.Definitions() {
		if d.Name != "command" && d.Name != "sensor_source" && d.Name != "recording_store" && d.Name != "file_store" && d.Name != "fact_check" {
			continue
		}
		t.Run(d.Name, func(t *testing.T) {
			rows, err := c.ConfigurationList(ctx, d.ListPath, nil)
			if err != nil {
				t.Fatal(err)
			}
			id := ""
			for _, raw := range rows {
				var obj struct {
					ID         int64  `json:"id"`
					Builtin    bool   `json:"is_builtin"`
					SourceType string `json:"source_type"`
				}
				if err = json.Unmarshal(raw, &obj); err != nil {
					t.Fatal(err)
				}
				if obj.Builtin || d.Name == "sensor_source" && obj.SourceType != "custom" {
					id = fmt.Sprint(obj.ID)
					break
				}
			}
			if id == "" {
				t.Fatal("expected backend-owned catalog fixture")
			}
			r := apimanaged.NewResource(d)
			var schema frameworkresource.SchemaResponse
			r.Schema(ctx, frameworkresource.SchemaRequest{}, &schema)
			var configured frameworkresource.ConfigureResponse
			r.(frameworkresource.ResourceWithConfigure).Configure(ctx, frameworkresource.ConfigureRequest{ProviderData: c}, &configured)
			response := frameworkresource.ImportStateResponse{State: tfsdk.State{Schema: schema.Schema}}
			r.(frameworkresource.ResourceWithImportState).ImportState(ctx, frameworkresource.ImportStateRequest{ID: id}, &response)
			if !response.Diagnostics.HasError() {
				t.Fatal("backend-owned object was imported")
			}
			if !regexp.MustCompile("backend-owned objects").MatchString(response.Diagnostics.Errors()[0].Detail()) {
				t.Fatalf("unexpected import error: %s", response.Diagnostics)
			}
		})
	}
}
