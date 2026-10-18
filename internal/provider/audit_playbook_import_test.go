package provider_test

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"terraform-provider-mistershell/internal/client"
)

func TestAccAuditPlaybookImportedWebhookSecretPreserved(t *testing.T) {
	testAccPreCheck(t)
	for _, kind := range []string{"bearer", "basic", "header"} {
		t.Run(kind, func(t *testing.T) {
			name := acctestPrefix + "import-webhook-" + kind
			field := map[string]string{"bearer": "token", "basic": "password", "header": "header_value"}[kind]
			graph := func(secret any, omit, disabled bool, path string) map[string]any {
				auth := map[string]any{"type": kind}
				if !omit {
					auth[field] = secret
				}
				if kind == "basic" {
					auth["username"] = "audit"
				}
				if kind == "header" {
					auth["header_name"] = "X-Audit-Key"
				}
				if disabled {
					auth = map[string]any{"type": "none"}
				}
				return map[string]any{
					"trigger": map[string]any{"event_name": "resource.session.recorded"},
					"nodes":   []any{map[string]any{"id": "webhook", "action_id": "webhook", "position": map[string]any{"x": 200, "y": 0}, "config": map[string]any{"url": "https://example.com/" + path, "auth": auth}}},
					"edges":   []any{map[string]any{"from": "trigger", "to": "webhook", "when": "always"}},
				}
			}
			c := testAccClient()
			body, _ := json.Marshal(map[string]any{"name": name, "enabled": false, "graph": graph("initial-secret", false, false, "initial")})
			raw, err := c.ConfigurationRequest(context.Background(), "POST", "/api/v1/automation/playbooks", json.RawMessage(body))
			if err != nil {
				t.Fatal(err)
			}
			var created struct {
				ID int64 `json:"id"`
			}
			if err = json.Unmarshal(raw, &created); err != nil || created.ID == 0 {
				t.Fatalf("create identity: %v", err)
			}
			id := fmt.Sprint(created.ID)
			endpoint := "/api/v1/automation/playbooks/" + id
			t.Cleanup(func() {
				if _, err := c.ConfigurationRequest(context.Background(), "DELETE", endpoint, nil); err != nil && !client.IsNotFound(err) {
					t.Error(err)
				}
			})
			config := func(secret any, omit, disabled bool, path string) string {
				return fmt.Sprintf("resource \"mistershell_automation_playbook\" \"audit\" {\n name=%q\n enabled=false\n graph=jsondecode(%q)\n}\n", name, auditHCL(graph(secret, omit, disabled, path)))
			}
			check := func(secret string, disabled bool, path string) resource.TestCheckFunc {
				return func(s *terraform.State) error {
					state := s.RootModule().Resources["mistershell_automation_playbook.audit"].Primary
					if state.ID != id {
						return fmt.Errorf("playbook identity changed")
					}
					if state.Attributes["graph.nodes.0.config.auth."+field] != secret {
						return fmt.Errorf("unexpected Terraform secret state; imported secrets must remain null")
					}
					raw, err := c.ConfigurationRequest(context.Background(), "GET", endpoint, nil)
					if err != nil {
						return err
					}
					var remote struct {
						Graph struct {
							Nodes []struct {
								Config struct {
									URL  string         `json:"url"`
									Auth map[string]any `json:"auth"`
								} `json:"config"`
							} `json:"nodes"`
						} `json:"graph"`
					}
					if err = json.Unmarshal(raw, &remote); err != nil {
						return err
					}
					if len(remote.Graph.Nodes) != 1 {
						return fmt.Errorf("missing webhook node")
					}
					node := remote.Graph.Nodes[0].Config
					if node.URL != "https://example.com/"+path {
						return fmt.Errorf("nonsecret graph edit was not stored")
					}
					if disabled {
						if node.Auth["type"] != "none" || node.Auth[field] != nil {
							return fmt.Errorf("explicit auth disable retained authentication")
						}
					} else if node.Auth[field] != "****" {
						return fmt.Errorf("graph edit erased the stored webhook secret")
					}
					return nil
				}
			}
			resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, CheckDestroy: func(_ *terraform.State) error {
				_, err := c.ConfigurationRequest(context.Background(), "GET", endpoint, nil)
				if client.IsNotFound(err) {
					return nil
				}
				if err != nil {
					return err
				}
				return fmt.Errorf("playbook survived deletion")
			}, Steps: []resource.TestStep{
				{Config: config(nil, false, false, "initial"), ResourceName: "mistershell_automation_playbook.audit", ImportStateId: id, ImportState: true, ImportStatePersist: true, ImportStateCheck: func(states []*terraform.InstanceState) error {
					if len(states) != 1 || states[0].Attributes["graph.nodes.0.config.auth."+field] != "" {
						return fmt.Errorf("import must retain an unknown secret as null")
					}
					return nil
				}},
				{Config: config(nil, false, false, "edited"), Check: check("", false, "edited")},
				{Config: config(nil, true, false, "omitted"), Check: check("", false, "omitted")},
				{Config: config(nil, true, false, "omitted-again"), Check: check("", false, "omitted-again")},
				{Config: config(nil, true, false, "omitted-again"), PlanOnly: true, ExpectNonEmptyPlan: false},
				{Config: config("rotated-secret", false, false, "rotated"), Check: check("rotated-secret", false, "rotated")},
				{Config: config(nil, false, false, "cannot-clear"), ExpectError: regexp.MustCompile("cannot remove a known webhook authentication secret")},
				{Config: config(nil, true, true, "disabled"), Check: check("", true, "disabled")},
				{Config: config(nil, true, true, "disabled"), PlanOnly: true, ExpectNonEmptyPlan: false},
			}})
		})
	}
}
