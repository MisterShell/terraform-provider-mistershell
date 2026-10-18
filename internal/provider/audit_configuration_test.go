package provider_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"terraform-provider-mistershell/internal/client"
)

type auditConfigurationFixture struct {
	name, itemPath, listPath string
	attrs                    map[string]string
	support                  string
	field                    string
	initial, updated, drift  any
	importIgnore             []string
}

func auditHCL(v any) string          { raw, _ := json.Marshal(v); return string(raw) }
func auditDynamic(raw string) string { return "jsondecode(" + strconv.Quote(raw) + ")" }

func TestAccAuditConfigurationFamilies(t *testing.T) {
	testAccPreCheck(t)
	for _, fixture := range auditConfigurationFixtures(t) {
		t.Run(fixture.name, func(t *testing.T) { testAccAuditConfiguration(t, fixture) })
	}
}

func testAccAuditConfiguration(t *testing.T, f auditConfigurationFixture) {
	var id string
	address := "mistershell_" + f.name + ".audit"
	config := func(value any, nameLookup bool) string {
		attrs := map[string]string{}
		for key, v := range f.attrs {
			attrs[key] = v
		}
		attrs[f.field] = auditHCL(value)
		keys := make([]string, 0, len(attrs))
		for key := range attrs {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		var b strings.Builder
		b.WriteString(f.support)
		fmt.Fprintf(&b, "\nresource %q \"audit\" {\n", "mistershell_"+f.name)
		for _, key := range keys {
			fmt.Fprintf(&b, "  %s = %s\n", key, attrs[key])
		}
		b.WriteString("}\n")
		fmt.Fprintf(&b, "data %q \"by_id\" {\n id = %s.id\n}\n", "mistershell_"+f.name, address)
		if _, ok := attrs["name"]; ok && nameLookup {
			fmt.Fprintf(&b, "data %q \"by_name\" {\n name = %s.name\n}\n", "mistershell_"+f.name, address)
		}
		return b.String()
	}
	endpoint := func() string {
		if f.name == "configure_resource_vars" {
			parts := strings.Split(id, ":")
			if len(parts) != 2 {
				return f.itemPath
			}
			return f.itemPath + "?resource_id=" + parts[0] + "&stack_id=" + parts[1]
		}
		return strings.ReplaceAll(f.itemPath, "{id}", url.PathEscape(id))
	}
	listParams := url.Values{}
	if f.name == "command" {
		var resourceType string
		if err := json.Unmarshal([]byte(f.attrs["resource_type"]), &resourceType); err != nil {
			t.Fatal(err)
		}
		listParams.Set("resource_type", resourceType)
	}
	read := func() (map[string]any, error) {
		var raw json.RawMessage
		var err error
		if auditListBacked(f.name) {
			rows, e := testAccClient().ConfigurationList(context.Background(), f.listPath, listParams)
			if e != nil {
				return nil, e
			}
			for _, r := range rows {
				var row map[string]any
				if e = json.Unmarshal(r, &row); e != nil {
					return nil, e
				}
				if fmt.Sprint(row["id"]) == id {
					return row, nil
				}
			}
			return nil, fmt.Errorf("configuration object %s missing", id)
		}
		raw, err = testAccClient().ConfigurationRequest(context.Background(), http.MethodGet, endpoint(), nil)
		if err != nil {
			return nil, err
		}
		var out map[string]any
		err = json.Unmarshal(raw, &out)
		return out, err
	}
	verify := func(value any) resource.TestCheckFunc {
		return func(s *terraform.State) error {
			rs := s.RootModule().Resources[address]
			if rs == nil {
				return fmt.Errorf("missing %s", address)
			}
			if id != "" && id != rs.Primary.ID {
				return fmt.Errorf("ordinary update replaced %s", address)
			}
			id = rs.Primary.ID
			remote, err := read()
			if err != nil {
				return err
			}
			if f.name == "ca_certificate" {
				var configured string
				if err := json.Unmarshal([]byte(f.attrs["pem"]), &configured); err != nil {
					return err
				}
				wantBlock, _ := pem.Decode([]byte(configured))
				gotBlock, _ := pem.Decode([]byte(fmt.Sprint(remote["pem"])))
				if wantBlock == nil || gotBlock == nil || !bytes.Equal(wantBlock.Bytes, gotBlock.Bytes) {
					return fmt.Errorf("CA certificate identity changed")
				}
			}
			if auditHCL(remote[f.field]) != auditHCL(value) {
				return fmt.Errorf("%s API %s=%s, want %s", f.name, f.field, auditHCL(remote[f.field]), auditHCL(value))
			}
			return nil
		}
	}
	checks := func(value any) resource.TestCheckFunc {
		list := []resource.TestCheckFunc{verify(value), resource.TestCheckResourceAttrPair("data.mistershell_"+f.name+".by_id", "id", address, "id")}
		if _, ok := f.attrs["name"]; ok {
			list = append(list, resource.TestCheckResourceAttrPair("data.mistershell_"+f.name+".by_name", "id", address, "id"))
		}
		return resource.ComposeAggregateTestCheckFunc(list...)
	}
	drift := func() {
		method, path, body := http.MethodPatch, endpoint(), map[string]any{f.field: f.drift}
		if f.name == "configure_resource_vars" {
			parts := strings.Split(id, ":")
			rid, _ := strconv.ParseInt(parts[0], 10, 64)
			sid, _ := strconv.ParseInt(parts[1], 10, 64)
			method = http.MethodPut
			path = f.itemPath
			body["resource_id"] = rid
			body["stack_id"] = sid
		}
		if f.name == "sensor_source" {
			method = http.MethodPost
			path += "/disable"
			body = nil
		}
		if _, err := testAccClient().ConfigurationRequest(context.Background(), method, path, body); err != nil {
			t.Fatal(err)
		}
	}
	destroy := func(s *terraform.State) error {
		if id == "" {
			return nil
		}
		if auditListBacked(f.name) {
			rows, err := testAccClient().ConfigurationList(context.Background(), f.listPath, listParams)
			if err != nil {
				return err
			}
			for _, row := range rows {
				var item map[string]any
				if err = json.Unmarshal(row, &item); err != nil {
					return err
				}
				if fmt.Sprint(item["id"]) == id {
					return fmt.Errorf("configuration object %s survived destruction", id)
				}
			}
			return nil
		}
		raw, err := testAccClient().ConfigurationRequest(context.Background(), http.MethodGet, endpoint(), nil)
		if client.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if f.name == "configure_resource_vars" {
			var row struct {
				Values map[string]any `json:"values"`
			}
			if err = json.Unmarshal(raw, &row); err != nil {
				return err
			}
			if len(row.Values) == 0 {
				return nil
			}
		}
		return fmt.Errorf("%s %s survived destruction", f.name, id)
	}
	steps := []resource.TestStep{
		{Config: config(f.initial, true), Check: checks(f.initial)},
		{Config: config(f.updated, true), Check: checks(f.updated)},
		{ResourceName: address, ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: f.importIgnore},
		{Config: config(f.updated, false), PreConfig: drift, PlanOnly: true, ExpectNonEmptyPlan: true},
		{Config: config(f.updated, false), Check: verify(f.updated)},
	}
	if f.name == "configure_resource_vars" {
		steps = append(steps, resource.TestStep{Config: f.support, Check: func(_ *terraform.State) error {
			remote, err := read()
			if err != nil {
				return err
			}
			values, ok := remote["values"].(map[string]any)
			if !ok || len(values) != 0 {
				return fmt.Errorf("removing resource vars did not reset values to an empty object")
			}
			return nil
		}})
	}
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, CheckDestroy: destroy, Steps: steps})
}

func auditConfigurationFixtures(t *testing.T) []auditConfigurationFixture {
	name := func(family string) string { return acctestPrefix + "configuration-" + family }
	basic := func(family, path string, attrs map[string]string, field string) auditConfigurationFixture {
		if attrs == nil {
			attrs = map[string]string{}
		}
		if family != "motd" && family != "configure_resource_vars" {
			attrs["name"] = auditHCL(name(family))
		}
		return auditConfigurationFixture{name: family, itemPath: path + "/{id}", listPath: path, attrs: attrs, field: field, initial: "initial", updated: "updated", drift: "drift"}
	}
	location := func(family string) string {
		return fmt.Sprintf(`
resource "mistershell_location" "support" {
 name=%q
 kind="geo"
 parent_id=1
}
`, name(family)+"-location")
	}
	aws := fmt.Sprintf(`
resource "mistershell_credential" "support" {
 name=%q
 credential_type="aws_credentials"
 credential_data={access_key="audit-access",secret_key="audit-secret"}
}
`, name("recording-credential"))
	ca := basic("ca_certificate", "/api/v1/ca-certificates", map[string]string{"pem": auditHCL(auditCACertificate(t))}, "name")
	ca.importIgnore = []string{"pem"}
	ca.initial = name("ca-initial")
	ca.updated = name("ca-updated")
	ca.drift = name("ca-drift")
	recordingStore := basic("recording_store", "/api/v1/session-policy/recording-stores", map[string]string{"backend": `"s3"`, "credential_id": "mistershell_credential.support.id", "params": `{s3_bucket="audit-bucket",s3_region="eu-west-1"}`}, "description")
	recordingStore.support = aws
	recordingStore.importIgnore = []string{"params"}
	recordingRule := basic("recording_rule", "/api/v1/session-policy/recording-rules", map[string]string{"action": `"skip"`, "enabled": "false"}, "comment")
	fileStore := basic("file_store", "/api/v1/file-policy/stores", map[string]string{"backend": `"local"`, "location_id": "mistershell_location.support.id"}, "description")
	fileStore.support = location("files")
	fileStore.importIgnore = []string{"params"}
	transfer := basic("file_transfer_rule", "/api/v1/file-policy/rules", map[string]string{"action": `"deny"`, "enabled": "false"}, "comment")
	fact := basic("fact_check", "/api/v1/fact-policy/checks", map[string]string{"fact_type": `"system_ntp"`, "assertions": auditDynamic(`[{"id":"synchronized","description":"NTP synchronized","match_mode":"exists","field_conditions":[{"field":"sync_status","operator":"eq","value":"synchronized"}]}]`)}, "description")
	fact.importIgnore = []string{"assertions"}
	factRule := basic("fact_policy_rule", "/api/v1/fact-policy/rules", map[string]string{"enabled": "false"}, "comment")
	playbook := basic("automation_playbook", "/api/v1/automation/playbooks", map[string]string{"enabled": "false", "graph": auditDynamic(`{"trigger":{"event_name":"resource.session.recorded"},"nodes":[],"edges":[]}`)}, "description")
	playbook.importIgnore = []string{"graph"}
	template := basic("configure_template", "/api/v1/configure/templates", map[string]string{"body": `"hostname {{ vars.hostname }}"`}, "description")
	stack := basic("configure_stack", "/api/v1/configure/stacks", nil, "description")
	stackSupport := fmt.Sprintf(`
resource "mistershell_configure_stack" "support" {
 name=%q
}
`, name("support-stack"))
	policy := basic("configure_policy", "/api/v1/configure/policies", map[string]string{"stack_id": "mistershell_configure_stack.support.id", "enabled": "false"}, "name")
	policy.support = stackSupport
	policy.initial = name("policy-initial")
	policy.updated = name("policy-updated")
	policy.drift = name("policy-drift")
	vars := basic("configure_resource_vars", "/api/v1/configure/resource-vars", map[string]string{"resource_id": "mistershell_resource.support.id", "stack_id": "mistershell_configure_stack.support.id"}, "values")
	vars.itemPath = "/api/v1/configure/resource-vars"
	vars.initial = map[string]any{"hostname": "initial"}
	vars.updated = map[string]any{"hostname": "updated"}
	vars.drift = map[string]any{"hostname": "drift"}
	vars.attrs["resource_id"] = "mistershell_resource.test.id"
	vars.support = fmt.Sprintf(`
resource "mistershell_configure_template" "support" {
 name=%q
 body="hostname {{ vars.hostname }}"
 variables_schema=[{name="hostname", type="string", required=true}]
}
resource "mistershell_configure_stack" "support" {
 name=%q
 members=[{template_id=tonumber(mistershell_configure_template.support.id), position=0}]
}
`, name("vars-template"), name("vars-stack")) + testAccSSHResourceConfig(name("vars-resource"), "linux")

	proxy := basic("proxy", "/api/v1/proxies", map[string]string{"location_id": "mistershell_location.support.id", "is_enabled": "false"}, "description")
	proxy.support = location("proxy")
	proxy.importIgnore = []string{"token"}
	sensor := basic("sensor", "/api/v1/sensors", map[string]string{"location_id": "mistershell_location.support.id", "is_enabled": "false", "config": `{interfaces=["eth0"],home_net=["192.0.2.0/24"]}`}, "description")
	sensor.support = location("sensor")
	sensor.importIgnore = []string{"config", "token"}
	custom := basic("sensor_custom_rule", "/api/v1/sensors/custom-rules", map[string]string{"kind": `"local"`, "enabled": "false", "content": auditHCL(`alert icmp any any -> any any (msg:"Terraform audit"; sid:9900001; rev:1;)`)}, "name")
	custom.initial = name("custom-initial")
	custom.updated = name("custom-updated")
	custom.drift = name("custom-drift")
	alert := basic("sensor_alert_rule", "/api/v1/sensors/alert-rules", map[string]string{"enabled": "false", "suppress": "true"}, "name")
	alert.initial = name("alert-initial")
	alert.updated = name("alert-updated")
	alert.drift = name("alert-drift")
	source := basic("sensor_source", "/api/v1/sensors/sources", map[string]string{"url": `"https://example.com/audit.rules"`}, "enabled")
	source.initial = false
	source.updated = true
	source.drift = false
	source.importIgnore = []string{"parameters"}
	report := basic("report_template", "/api/v1/reporting/templates", map[string]string{"report_type": `"inventory_posture"`, "access_mode": `"public"`, "ttl_hours": "24"}, "description")
	report.importIgnore = []string{"filters"}
	command := basic("command", "/api/v1/commands", map[string]string{"resource_type": `"cisco_ios"`, "category": `"Audit"`, "mode": `"read"`, "definition": `{command="show version"}`, "parse_source": `{kind="no_parse"}`}, "description")
	command.importIgnore = []string{"definition", "parse_source"}
	motd := basic("motd", "/api/v1/motds", map[string]string{"title": auditHCL(name("motd")), "is_enabled": "false"}, "content")
	return []auditConfigurationFixture{ca, recordingStore, recordingRule, fileStore, transfer, fact, factRule, playbook, template, stack, policy, vars, proxy, sensor, custom, alert, source, report, command, motd}
}

func auditCACertificate(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cert := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: acctestPrefix + "audit-ca"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func auditListBacked(name string) bool {
	switch name {
	case "sensor_source", "sensor_custom_rule", "sensor_alert_rule", "command":
		return true
	}
	return false
}
