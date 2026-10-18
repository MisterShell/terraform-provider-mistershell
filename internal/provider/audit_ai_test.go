package provider_test

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"strings"
	"terraform-provider-mistershell/internal/client"
	"testing"
)

func TestAccAuditAIModelPromptAgentLifecycle(t *testing.T) {
	testAccPreCheck(t)
	build := func(settings bool, agentType string) string {
		modelConfig, promptSchema, agentConfig := "", "", ""
		if settings {
			modelConfig = `config = {base_url="http://localhost:11434", api_key="audit-test-secret"}`
			promptSchema = `variable_schema={type="object",properties={subject={type="string"}}}`
			agentConfig = `config={token_budget=321, temperature=0.5}`
		}
		return fmt.Sprintf(`
resource "mistershell_ai_model" "audit" {
 name="%saudit-model"
 model_provider="ollama"
 model_id="llama3"
 %s
}
resource "mistershell_ai_prompt" "audit" {
 name="%saudit-prompt"
 content="Hello"
 %s
}
resource "mistershell_ai_agent" "audit" {
 name="%saudit-agent"
 type=%q
 system_prompt_id=mistershell_ai_prompt.audit.id
 model_id=mistershell_ai_model.audit.id
 %s
}
data "mistershell_ai_prompt_variables" "audit" {}
`, acctestPrefix, modelConfig, acctestPrefix, promptSchema, acctestPrefix, agentType, agentConfig)
	}
	var initialAgentID string
	var liveModelID, livePromptID int64
	checkRemoteCleared := func(s *terraform.State) error {
		c := testAccClient()
		ctx := context.Background()
		modelID := s.RootModule().Resources["mistershell_ai_model.audit"].Primary.ID
		agentID := s.RootModule().Resources["mistershell_ai_agent.audit"].Primary.ID
		promptID := s.RootModule().Resources["mistershell_ai_prompt.audit"].Primary.ID
		if initialAgentID != "" && agentID != initialAgentID {
			return fmt.Errorf("agent was replaced on type change")
		}
		var m, a, p int64
		_, _ = fmt.Sscan(modelID, &m)
		_, _ = fmt.Sscan(agentID, &a)
		_, _ = fmt.Sscan(promptID, &p)
		model, e := c.GetAIModel(ctx, m)
		if e != nil {
			return e
		}
		var fields map[string]any
		_ = json.Unmarshal(model.Config, &fields)
		if _, ok := fields["base_url"]; ok {
			return fmt.Errorf("model nonsecret config not cleared")
		}
		agent, e := c.GetAIAgent(ctx, a)
		if e != nil {
			return e
		}
		if string(agent.Config) != "{}" || agent.Type != "background" {
			return fmt.Errorf("agent was not cleared and updated: %s %s", agent.Config, agent.Type)
		}
		prompt, e := c.GetAIPrompt(ctx, p)
		if e != nil {
			return e
		}
		if string(prompt.VariableSchema) != "null" {
			return fmt.Errorf("prompt schema was not cleared: %s", prompt.VariableSchema)
		}
		return nil
	}
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, CheckDestroy: testAccCheckAllDestroyed, Steps: []resource.TestStep{
		{Config: build(true, "chat"), Check: func(s *terraform.State) error {
			initialAgentID = s.RootModule().Resources["mistershell_ai_agent.audit"].Primary.ID
			_, _ = fmt.Sscan(s.RootModule().Resources["mistershell_ai_model.audit"].Primary.ID, &liveModelID)
			_, _ = fmt.Sscan(s.RootModule().Resources["mistershell_ai_prompt.audit"].Primary.ID, &livePromptID)
			return nil
		}},
		{ResourceName: "mistershell_ai_model.audit", ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: []string{"config"}},
		{ResourceName: "mistershell_ai_agent.audit", ImportState: true, ImportStateVerify: true},
		{ResourceName: "mistershell_ai_prompt.audit", ImportState: true, ImportStateVerify: true},

		{PreConfig: func() {
			c := testAccClient()
			ctx := context.Background()
			if _, e := c.UpdateAIModel(ctx, liveModelID, client.AIModelUpdateInput{Config: []byte(`{"base_url":"http://drift.invalid"}`)}); e != nil {
				t.Fatal(e)
			}
			if _, e := c.UpdateAIPrompt(ctx, livePromptID, client.AIPromptUpdateInput{VariableSchema: []byte(`{"type":"string"}`)}); e != nil {
				t.Fatal(e)
			}
			var id int64
			_, _ = fmt.Sscan(initialAgentID, &id)
			if _, e := c.UpdateAIAgent(ctx, id, client.AIAgentUpdateInput{Config: []byte(`{"token_budget":999,"temperature":1.0}`), ModelID: &liveModelID}); e != nil {
				t.Fatal(e)
			}
		}, Config: build(true, "chat"), PlanOnly: true, ExpectNonEmptyPlan: true},
		{Config: build(true, "chat"), Check: resource.ComposeAggregateTestCheckFunc(resource.TestCheckResourceAttr("mistershell_ai_model.audit", "config.base_url", "http://localhost:11434"), resource.TestCheckResourceAttr("mistershell_ai_model.audit", "config.api_key", "audit-test-secret"), resource.TestCheckResourceAttr("mistershell_ai_agent.audit", "config.token_budget", "321"), resource.TestCheckResourceAttr("mistershell_ai_prompt.audit", "variable_schema.type", "object"))},
		{Config: build(false, "background"), Check: checkRemoteCleared},
		{Config: build(false, "background"), PlanOnly: true},
	}})
}

func TestAccAuditAIOverridesAndGuardrail(t *testing.T) {
	testAccPreCheck(t)
	c := testAccClient()
	ctx := context.Background()
	agents, err := c.ListAIAgents(ctx, client.AIAgentListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	var agent *client.AIAgentResponse
	for _, a := range agents {
		if a.IsBuiltin {
			agent, err = c.GetAIAgent(ctx, a.ID)
			break
		}
	}
	if err != nil || agent == nil {
		t.Fatalf("built-in agent fixture missing: %v", err)
	}
	skills, err := c.ListAISkills(ctx, client.AISkillListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	var skill *client.AISkillResponse
	for _, s := range skills {
		if s.IsBuiltin {
			skill, err = c.GetAISkill(ctx, s.ID)
			break
		}
	}
	if err != nil || skill == nil {
		t.Fatalf("built-in skill fixture missing: %v", err)
	}
	guardrails, err := c.ListAIGuardrails(ctx)
	if err != nil || len(guardrails) == 0 {
		t.Fatalf("guardrail fixture missing: %v", err)
	}
	guard := guardrails[0]
	var initialConfig map[string]any
	_ = json.Unmarshal(agent.Config, &initialConfig)
	build := func(temp float64, enabled bool) string {
		return fmt.Sprintf(`
resource "mistershell_ai_agent_override" "audit" {
 agent_id=%d
 config={temperature=%g}
}
resource "mistershell_ai_skill_override" "audit" {
 skill_id=%d
 is_enabled=%t
}
resource "mistershell_ai_guardrail" "audit" {
 resource_type=%q
 allowlist_enabled=false
}
data "mistershell_ai_guardrail" "audit" {
 resource_type=mistershell_ai_guardrail.audit.resource_type
}
data "mistershell_ai_prompt_variables" "audit" {}
`, agent.ID, temp, skill.ID, enabled, guard.ResourceType)
	}
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, CheckDestroy: func(_ *terraform.State) error {
		a, e := c.GetAIAgent(ctx, agent.ID)
		if e != nil {
			return e
		}
		var fields map[string]any
		_ = json.Unmarshal(a.Config, &fields)
		if fields["temperature"] != initialConfig["temperature"] {
			return fmt.Errorf("agent temperature not restored")
		}
		if a.Name != agent.Name || a.Type != agent.Type {
			return fmt.Errorf("built-in identity changed")
		}
		s, e := c.GetAISkill(ctx, skill.ID)
		if e != nil {
			return e
		}
		if s.IsEnabled != skill.IsEnabled {
			return fmt.Errorf("skill setting not restored")
		}
		rows, e := c.ListAIGuardrails(ctx)
		if e != nil {
			return e
		}
		for _, r := range rows {
			if r.ResourceType == guard.ResourceType && !r.AllowlistEnabled {
				return fmt.Errorf("guardrail not reset")
			}
		}
		return nil
	}, Steps: []resource.TestStep{
		{Config: build(0.4, !skill.IsEnabled), Check: resource.ComposeAggregateTestCheckFunc(resource.TestCheckResourceAttr("mistershell_ai_agent_override.audit", "config.temperature", "0.4"), resource.TestCheckResourceAttr("data.mistershell_ai_guardrail.audit", "allowlist_enabled", "false"), resource.TestCheckResourceAttrSet("data.mistershell_ai_prompt_variables.audit", "id"))},
		{ResourceName: "mistershell_ai_guardrail.audit", ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: []string{"original_allowlist_enabled"}},
		{ResourceName: "mistershell_ai_agent_override.audit", ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: []string{"config", "original_settings"}},
		{ResourceName: "mistershell_ai_skill_override.audit", ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: []string{"original_enabled"}},
		{PreConfig: func() {
			if _, e := c.UpdateAIAgentSettings(ctx, agent.ID, map[string]any{"config": map[string]any{"temperature": 0.9}}); e != nil {
				t.Fatal(e)
			}
			if _, e := c.UpdateAISkillEnabled(ctx, skill.ID, skill.IsEnabled); e != nil {
				t.Fatal(e)
			}
			if _, e := c.SetAIGuardrail(ctx, guard.ResourceType, true); e != nil {
				t.Fatal(e)
			}
		}, Config: build(0.6, !skill.IsEnabled), Check: resource.TestCheckResourceAttr("mistershell_ai_agent_override.audit", "config.temperature", "0.6")},
		{Config: strings.Replace(build(0.6, !skill.IsEnabled), "config={temperature=0.6}", "", 1), Check: func(_ *terraform.State) error {
			a, e := c.GetAIAgent(ctx, agent.ID)
			if e != nil {
				return e
			}
			var fields map[string]any
			_ = json.Unmarshal(a.Config, &fields)
			if fields["temperature"] != initialConfig["temperature"] {
				return fmt.Errorf("removing config did not restore temperature")
			}
			return nil
		}},
		{Config: build(0.4, !skill.IsEnabled)},
	}})
}

func TestAccAuditAIGuardrailRestoresPreexistingDisabledSetting(t *testing.T) {
	testAccPreCheck(t)
	c := testAccClient()
	ctx := context.Background()
	rows, err := c.ListAIGuardrails(ctx)
	if err != nil || len(rows) == 0 {
		t.Fatalf("guardrail fixture unavailable: %v", err)
	}
	original := rows[0]
	if _, err = c.SetAIGuardrail(ctx, original.ResourceType, false); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := c.SetAIGuardrail(ctx, original.ResourceType, original.AllowlistEnabled); err != nil {
			t.Error(err)
		}
	})
	config := func(enabled bool) string {
		return fmt.Sprintf(`
resource "mistershell_ai_guardrail" "baseline" {
 resource_type=%q
 allowlist_enabled=%t
}
`, original.ResourceType, enabled)
	}
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, CheckDestroy: func(_ *terraform.State) error {
		rows, err := c.ListAIGuardrails(ctx)
		if err != nil {
			return err
		}
		for _, row := range rows {
			if row.ResourceType == original.ResourceType {
				if row.AllowlistEnabled {
					return fmt.Errorf("destroy did not restore pre-existing disabled guardrail")
				}
				return nil
			}
		}
		return fmt.Errorf("guardrail disappeared")
	}, Steps: []resource.TestStep{
		{Config: config(true), Check: resource.ComposeAggregateTestCheckFunc(resource.TestCheckResourceAttr("mistershell_ai_guardrail.baseline", "allowlist_enabled", "true"), resource.TestCheckResourceAttr("mistershell_ai_guardrail.baseline", "original_allowlist_enabled", "false"))},
		{ResourceName: "mistershell_ai_guardrail.baseline", ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: []string{"original_allowlist_enabled"}},
		{Config: config(false), Check: resource.TestCheckResourceAttr("mistershell_ai_guardrail.baseline", "original_allowlist_enabled", "false")},
		{Config: config(true), Check: resource.TestCheckResourceAttr("mistershell_ai_guardrail.baseline", "original_allowlist_enabled", "false")},
	}})
}
