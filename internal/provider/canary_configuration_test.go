package provider_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// These tests run Terraform's real plan/apply/import protocol against an isolated
// HTTP fixture. They do not use or require a running MisterShell installation.
func TestCanaryConfigurationProtocol(t *testing.T) {
	for _, tc := range []struct {
		name, api, attributes, change string
		listOnly                      bool
	}{
		{"canary", "/api/v1/canaries", `location_id=2
 is_enabled=false
 decoy_interface="eth1"
 ip_ignorelist=["192.0.2.0/24"]
 rate_limit_per_min=120`, `decoy_interface=null`, false},
		{"canary_service", "/api/v1/canaries/services", `service_type="ssh"
 location_ids=[2]
 settings={port=2222,banner="SSH-2.0-OpenSSH_9.6"}
 enabled=false`, `enabled=true`, false},
		{"canary_alert_rule", "/api/v1/canaries/rules", `location_ids=[2]
 service_types=["ssh"]
 log=true
 enabled=false`, `enabled=true`, false},
		{"syslog_rule", "/api/v1/collector/policy", `severities=[0,1,2,3]
 log=true
 msg_contains="initial"`, `msg_contains=null`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			var row map[string]any
			writes := 0
			cleared := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				if r.Header.Get("Authorization") != "Bearer protocol-test" {
					t.Error("missing auth")
				}
				send := func(v any) { _ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": v}) }
				switch r.Method {
				case "POST":
					row = map[string]any{}
					_ = json.NewDecoder(r.Body).Decode(&row)
					row["id"] = float64(9)
					if tc.name == "canary" {
						row["status"] = "offline"
						row["health"] = map[string]any{}
						row["counters"] = map[string]any{}
					}
					if tc.name == "canary_service" {
						row["seed_key"] = nil
						settings := row["settings"].(map[string]any)
						if _, ok := settings["port"].(float64); !ok {
							t.Error("port lost native numeric type")
						}
					}
					if tc.name == "canary_alert_rule" || tc.name == "syslog_rule" {
						row["position"] = float64(10)
					}
					copy := map[string]any{}
					for k, v := range row {
						copy[k] = v
					}
					if tc.name == "canary" {
						copy["token"] = "enroll-once"
					}
					writes++
					send(copy)
				case "PUT", "PATCH":
					var changes map[string]any
					_ = json.NewDecoder(r.Body).Decode(&changes)
					for k, v := range changes {
						row[k] = v
						if (k == "decoy_interface" || k == "msg_contains") && v == nil {
							cleared = true
						}
					}
					writes++
					send(row)
				case "GET":
					if r.URL.Path == tc.api {
						items := []any{}
						if row != nil {
							items = append(items, row)
						}
						send(items)
					} else if row == nil {
						w.WriteHeader(404)
						send(nil)
					} else if tc.listOnly {
						t.Error("nonexistent Syslog detail GET")
						w.WriteHeader(404)
						send(nil)
					} else {
						send(row)
					}
				case "DELETE":
					row = nil
					send(nil)
				default:
					t.Errorf("unexpected method %s", r.Method)
				}
			}))
			defer server.Close()
			address := "mistershell_" + tc.name + ".test"
			config := func(update bool) string {
				attrs := tc.attributes
				if update {
					switch tc.name {
					case "canary":
						attrs = `location_id=2
 is_enabled=false
 ip_ignorelist=["192.0.2.0/24"]
 rate_limit_per_min=120
 ` + tc.change
					case "canary_service":
						attrs = `service_type="ssh"
 location_ids=[2]
 settings={port=2222,banner="SSH-2.0-OpenSSH_9.6"}
 ` + tc.change
					case "canary_alert_rule":
						attrs = `location_ids=[2]
 service_types=["ssh"]
 log=true
 ` + tc.change
					case "syslog_rule":
						attrs = `severities=[0,1,2,3]
 log=true
 ` + tc.change
					}
				}
				return fmt.Sprintf(`provider "mistershell" {
 url=%q
 api_key="protocol-test"
}
resource %q "test" {
 name="protocol-example"
 %s
}
data %q "lookup" {
 name=%s.name
}
`, server.URL, "mistershell_"+tc.name, attrs, "mistershell_"+tc.name, address)
			}
			ignore := []string{}
			if tc.name == "canary" {
				ignore = append(ignore, "token")
			}
			resource.Test(t, resource.TestCase{IsUnitTest: true, ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, CheckDestroy: func(_ *terraform.State) error {
				mu.Lock()
				defer mu.Unlock()
				if row != nil {
					return fmt.Errorf("object was not deleted")
				}
				return nil
			}, Steps: []resource.TestStep{
				{Config: config(false), Check: resource.ComposeTestCheckFunc(resource.TestCheckResourceAttr(address, "id", "9"), resource.TestCheckResourceAttr("data.mistershell_"+tc.name+".lookup", "name", "protocol-example"))},
				{ResourceName: address, ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: ignore},
				{Config: config(true)},
				{Config: config(true), PlanOnly: true},
			}})
			mu.Lock()
			defer mu.Unlock()
			if writes < 2 {
				t.Fatal("update not exercised")
			}
			if (tc.name == "canary" || tc.name == "syslog_rule") && !cleared {
				t.Fatal("nullable field did not clear")
			}
		})
	}
}

func TestCanaryServiceOverrideProtocol(t *testing.T) {
	var mu sync.Mutex
	enabled := false
	writes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case "GET":
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"id": 1, "seed_key": "canary:ssh", "enabled": enabled}})
		case "PUT":
			var body map[string]bool
			_ = json.NewDecoder(r.Body).Decode(&body)
			if len(body) != 1 {
				t.Error("override changed unrelated fields")
			}
			enabled = body["enabled"]
			writes++
			_, _ = w.Write([]byte(`{"success":true,"data":null}`))
		default:
			t.Errorf("override must never delete builtin: %s", r.Method)
		}
	}))
	defer server.Close()
	config := func(on bool) string {
		return fmt.Sprintf(`provider "mistershell" {
 url=%q
 api_key="protocol-test"
}
resource "mistershell_canary_service_override" "test" {
 service_id=1
 enabled=%t
}`, server.URL, on)
	}
	resource.Test(t, resource.TestCase{IsUnitTest: true, ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, CheckDestroy: func(_ *terraform.State) error {
		mu.Lock()
		defer mu.Unlock()
		if enabled {
			return fmt.Errorf("destroy did not restore original disabled state")
		}
		return nil
	}, Steps: []resource.TestStep{
		{Config: config(true), Check: resource.TestCheckResourceAttr("mistershell_canary_service_override.test", "original_enabled", "false")},
		// Import captures the currently enabled flag as its own baseline. The main
		// state keeps its create baseline and must still restore false on destroy.
		{ResourceName: "mistershell_canary_service_override.test", ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: []string{"original_enabled"}},
		{Config: config(false), Check: resource.TestCheckResourceAttr("mistershell_canary_service_override.test", "original_enabled", "false")},
		{Config: config(true)},
		{Config: config(true), PlanOnly: true},
	}})
	mu.Lock()
	defer mu.Unlock()
	if writes != 4 {
		t.Errorf("expected create, two updates and baseline restoration, got %d writes", writes)
	}
}

func TestCanaryAndSyslogOrderProtocol(t *testing.T) {
	for _, name := range []string{"canary_rule_order", "syslog_rule_order"} {
		t.Run(name, func(t *testing.T) {
			var mu sync.Mutex
			ids := []int64{1, 2}
			writes := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				switch r.Method {
				case "GET":
					rows := []map[string]int64{}
					for i, id := range ids {
						rows = append(rows, map[string]int64{"id": id, "position": int64((i + 1) * 10)})
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": rows})
				case "PUT":
					var body struct {
						IDs []int64 `json:"ordered_ids"`
					}
					_ = json.NewDecoder(r.Body).Decode(&body)
					ids = body.IDs
					writes++
					_, _ = w.Write([]byte(`{"success":true,"data":null}`))
				default:
					t.Errorf("order must never delete rules: %s", r.Method)
				}
			}))
			defer server.Close()
			config := func(order string) string {
				return fmt.Sprintf(`provider "mistershell" {
 url=%q
 api_key="protocol-test"
}
resource %q "test" {
 ordered_ids=%s
}`, server.URL, "mistershell_"+name, order)
			}
			address := "mistershell_" + name + ".test"
			resource.Test(t, resource.TestCase{IsUnitTest: true, ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, Steps: []resource.TestStep{
				{Config: config("[2,1]"), Check: resource.TestCheckResourceAttr(address, "ordered_ids.0", "2")},
				{ResourceName: address, ImportState: true, ImportStateId: "policy", ImportStateVerify: true},
				{Config: config("[1,2]")},
				{Config: config("[1,2]"), PlanOnly: true},
			}})
			mu.Lock()
			defer mu.Unlock()
			if writes != 2 || ids[0] != 1 {
				t.Fatalf("unexpected order lifecycle: %v, writes %d", ids, writes)
			}
		})
	}
}

// The fixture applies the backend's stable selector deduplication. Duplicate
// known values must fail validation before that normalization can commit; a
// Terraform dependency makes the second case unknown until apply.
func TestCanaryDuplicateNormalizationProtocol(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		t.Run(fmt.Sprint("unknown_", unknown), func(t *testing.T) {
			var mu sync.Mutex
			writes := 0
			var row map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				send := func(v any) { _ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": v}) }
				switch r.Method {
				case "POST", "PUT":
					writes++
					_ = json.NewDecoder(r.Body).Decode(&row)
					row["id"] = 1
					row["position"] = 10
					for _, name := range []string{"service_types", "event_codes", "src_cidrs", "canary_ids", "location_ids"} {
						if vals, ok := row[name].([]any); ok {
							seen := map[any]bool{}
							unique := []any{}
							for _, v := range vals {
								if !seen[v] {
									unique = append(unique, v)
									seen[v] = true
								}
							}
							row[name] = unique
						}
					}
					send(row)
				case "GET":
					send(row)
				case "DELETE":
					row = nil
					send(nil)
				default:
					t.Errorf("unexpected %s", r.Method)
				}
			}))
			defer server.Close()
			extra := ""
			selectors := `["ssh","ssh"]`
			if unknown {
				extra = `resource "terraform_data" "service_type" { input="ssh" }`
				selectors = `["ssh",terraform_data.service_type.output]`
			}
			config := fmt.Sprintf(`provider "mistershell" {
 url=%q
 api_key="protocol-test"
}
%s
resource "mistershell_canary_alert_rule" "duplicates" {
 name="duplicate-selector"
 log=true
 service_types=%s
}`, server.URL, extra, selectors)
			resource.Test(t, resource.TestCase{IsUnitTest: true, ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, Steps: []resource.TestStep{{Config: config, ExpectError: regexp.MustCompile("Duplicate selector")}}})
			mu.Lock()
			defer mu.Unlock()
			if writes != 0 {
				t.Errorf("API normalization committed duplicate input %d times", writes)
			}
		})
	}
}
