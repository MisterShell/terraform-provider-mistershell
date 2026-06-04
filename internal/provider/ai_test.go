package provider_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"terraform-provider-mistershell/internal/client"
)

// AI entity acceptance tests: models, prompts, agents, skills, and (data-source
// only) tools.
//
// Models use the "ollama" provider so no secret/api_key is required. Builtins
// (builtin agents/prompts/skills, backend tools) are never created — tools are
// covered by a runtime-probe data-source test that skips explicitly when the
// instance has none.
//
// All names are prefixed with acctestPrefix so reruns / parallel runs do not
// collide and the sweepers can target them. Each case uses CheckDestroy.

// ---------------------------------------------------------------------------
// mistershell_ai_model — create (ollama), update model_id + name, import.
// config is stored-from-config / masked, so it is excluded from
// ImportStateVerify.
// ---------------------------------------------------------------------------

func TestAccAIModel_basic(t *testing.T) {
	testAccPreCheck(t)

	name := acctestPrefix + "model"
	updated := acctestPrefix + "model-updated"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccAIModelConfig(name, "llama3"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_ai_model.test", "name", name),
					resource.TestCheckResourceAttr("mistershell_ai_model.test", "model_provider", "ollama"),
					resource.TestCheckResourceAttr("mistershell_ai_model.test", "model_id", "llama3"),
					resource.TestCheckResourceAttr("mistershell_ai_model.test", "is_default", "false"),
					resource.TestCheckResourceAttrSet("mistershell_ai_model.test", "id"),
					resource.TestCheckResourceAttrSet("mistershell_ai_model.test", "config"),
				),
			},
			// Update model_id + name.
			{
				Config: testAccAIModelConfig(updated, "llama3.1"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_ai_model.test", "name", updated),
					resource.TestCheckResourceAttr("mistershell_ai_model.test", "model_id", "llama3.1"),
				),
			},
			// Import: config is stored-from-config / masked, not round-tripped.
			{
				ResourceName:            "mistershell_ai_model.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"config"},
			},
		},
	})
}

func TestAccAIModel_dataSource(t *testing.T) {
	testAccPreCheck(t)

	name := acctestPrefix + "model-ds"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccAIModelDataSourceConfig(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					// by id
					resource.TestCheckResourceAttrPair("data.mistershell_ai_model.by_id", "id", "mistershell_ai_model.test", "id"),
					resource.TestCheckResourceAttr("data.mistershell_ai_model.by_id", "name", name),
					resource.TestCheckResourceAttr("data.mistershell_ai_model.by_id", "model_provider", "ollama"),
					resource.TestCheckResourceAttr("data.mistershell_ai_model.by_id", "model_id", "llama3"),
					// by name
					resource.TestCheckResourceAttrPair("data.mistershell_ai_model.by_name", "id", "mistershell_ai_model.test", "id"),
					resource.TestCheckResourceAttr("data.mistershell_ai_model.by_name", "model_id", "llama3"),
				),
			},
		},
	})
}

// ---------------------------------------------------------------------------
// mistershell_ai_prompt — create (type defaults to "user"), update content +
// description, import. variable_schema is stored-from-config.
// ---------------------------------------------------------------------------

func TestAccAIPrompt_basic(t *testing.T) {
	testAccPreCheck(t)

	name := acctestPrefix + "prompt"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccAIPromptConfig(name, "Summarize {{resource_id}}.", "first prompt"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_ai_prompt.test", "name", name),
					resource.TestCheckResourceAttr("mistershell_ai_prompt.test", "type", "user"),
					resource.TestCheckResourceAttr("mistershell_ai_prompt.test", "content", "Summarize {{resource_id}}."),
					resource.TestCheckResourceAttr("mistershell_ai_prompt.test", "description", "first prompt"),
					resource.TestCheckResourceAttrSet("mistershell_ai_prompt.test", "id"),
				),
			},
			// Update content + description.
			{
				Config: testAccAIPromptConfig(name, "Describe {{resource_id}} in detail.", "second prompt"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_ai_prompt.test", "content", "Describe {{resource_id}} in detail."),
					resource.TestCheckResourceAttr("mistershell_ai_prompt.test", "description", "second prompt"),
				),
			},
			// Import: variable_schema is stored-from-config, not round-tripped.
			{
				ResourceName:            "mistershell_ai_prompt.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"variable_schema"},
			},
		},
	})
}

func TestAccAIPrompt_dataSource(t *testing.T) {
	testAccPreCheck(t)

	name := acctestPrefix + "prompt-ds"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccAIPromptDataSourceConfig(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					// by id
					resource.TestCheckResourceAttrPair("data.mistershell_ai_prompt.by_id", "id", "mistershell_ai_prompt.test", "id"),
					resource.TestCheckResourceAttr("data.mistershell_ai_prompt.by_id", "name", name),
					resource.TestCheckResourceAttr("data.mistershell_ai_prompt.by_id", "type", "user"),
					resource.TestCheckResourceAttr("data.mistershell_ai_prompt.by_id", "content", "Summarize {{resource_id}}."),
					// by name
					resource.TestCheckResourceAttrPair("data.mistershell_ai_prompt.by_name", "id", "mistershell_ai_prompt.test", "id"),
					resource.TestCheckResourceAttr("data.mistershell_ai_prompt.by_name", "type", "user"),
				),
			},
		},
	})
}

// ---------------------------------------------------------------------------
// mistershell_ai_agent — create (model + user prompt + agent), update name +
// description + config, import. config is stored-from-config / merged. tool_ids
// omitted on create -> all tools.
// ---------------------------------------------------------------------------

func TestAccAIAgent_basic(t *testing.T) {
	testAccPreCheck(t)

	name := acctestPrefix + "agent"
	updated := acctestPrefix + "agent-updated"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccAIAgentConfig(name, "first agent", 1000),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_ai_agent.test", "name", name),
					resource.TestCheckResourceAttr("mistershell_ai_agent.test", "type", "chat"),
					resource.TestCheckResourceAttr("mistershell_ai_agent.test", "description", "first agent"),
					resource.TestCheckResourceAttr("mistershell_ai_agent.test", "is_builtin", "false"),
					resource.TestCheckResourceAttrSet("mistershell_ai_agent.test", "is_functional"),
					resource.TestCheckResourceAttrSet("mistershell_ai_agent.test", "id"),
					resource.TestCheckResourceAttrPair("mistershell_ai_agent.test", "model_id", "mistershell_ai_model.test", "id"),
					resource.TestCheckResourceAttrPair("mistershell_ai_agent.test", "system_prompt_id", "mistershell_ai_prompt.test", "id"),
				),
			},
			// Update name + description + config.
			{
				Config: testAccAIAgentConfig(updated, "second agent", 2000),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_ai_agent.test", "name", updated),
					resource.TestCheckResourceAttr("mistershell_ai_agent.test", "description", "second agent"),
				),
			},
			// Import: config is stored-from-config / merged, not round-tripped.
			{
				ResourceName:            "mistershell_ai_agent.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"config"},
			},
		},
	})
}

func TestAccAIAgent_dataSource(t *testing.T) {
	testAccPreCheck(t)

	name := acctestPrefix + "agent-ds"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccAIAgentDataSourceConfig(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					// by id
					resource.TestCheckResourceAttrPair("data.mistershell_ai_agent.by_id", "id", "mistershell_ai_agent.test", "id"),
					resource.TestCheckResourceAttr("data.mistershell_ai_agent.by_id", "name", name),
					resource.TestCheckResourceAttr("data.mistershell_ai_agent.by_id", "type", "chat"),
					resource.TestCheckResourceAttr("data.mistershell_ai_agent.by_id", "is_builtin", "false"),
					// by name
					resource.TestCheckResourceAttrPair("data.mistershell_ai_agent.by_name", "id", "mistershell_ai_agent.test", "id"),
					resource.TestCheckResourceAttr("data.mistershell_ai_agent.by_name", "type", "chat"),
				),
			},
		},
	})
}

// ---------------------------------------------------------------------------
// mistershell_ai_skill — create (chat, enabled), update body + disable +
// agent_types, import. resource_types omitted on create.
// ---------------------------------------------------------------------------

func TestAccAISkill_basic(t *testing.T) {
	testAccPreCheck(t)

	name := acctestPrefix + "skill"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccAISkillConfig(name, "# First skill body", "true", `["chat"]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_ai_skill.test", "name", name),
					resource.TestCheckResourceAttr("mistershell_ai_skill.test", "body", "# First skill body"),
					resource.TestCheckResourceAttr("mistershell_ai_skill.test", "is_enabled", "true"),
					resource.TestCheckResourceAttr("mistershell_ai_skill.test", "is_builtin", "false"),
					resource.TestCheckResourceAttr("mistershell_ai_skill.test", "agent_types.#", "1"),
					resource.TestCheckTypeSetElemAttr("mistershell_ai_skill.test", "agent_types.*", "chat"),
					resource.TestCheckResourceAttrSet("mistershell_ai_skill.test", "id"),
				),
			},
			// Update body + disable + add a second agent_type.
			{
				Config: testAccAISkillConfig(name, "# Second skill body", "false", `["chat", "background"]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_ai_skill.test", "body", "# Second skill body"),
					resource.TestCheckResourceAttr("mistershell_ai_skill.test", "is_enabled", "false"),
					resource.TestCheckResourceAttr("mistershell_ai_skill.test", "agent_types.#", "2"),
				),
			},
			// Import by integer id; agent_types round-trip cleanly.
			{
				ResourceName:      "mistershell_ai_skill.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccAISkill_dataSource(t *testing.T) {
	testAccPreCheck(t)

	name := acctestPrefix + "skill-ds"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccAISkillDataSourceConfig(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					// by id
					resource.TestCheckResourceAttrPair("data.mistershell_ai_skill.by_id", "id", "mistershell_ai_skill.test", "id"),
					resource.TestCheckResourceAttr("data.mistershell_ai_skill.by_id", "name", name),
					resource.TestCheckResourceAttr("data.mistershell_ai_skill.by_id", "is_builtin", "false"),
					resource.TestCheckResourceAttr("data.mistershell_ai_skill.by_id", "body", "# Data source skill"),
					// by name
					resource.TestCheckResourceAttrPair("data.mistershell_ai_skill.by_name", "id", "mistershell_ai_skill.test", "id"),
					resource.TestCheckResourceAttr("data.mistershell_ai_skill.by_name", "is_enabled", "true"),
				),
			},
		},
	})
}

// ---------------------------------------------------------------------------
// mistershell_ai_tool — backend-builtin, cannot be created. Probe the instance
// at runtime; skip explicitly if no tools exist. Otherwise look up the first
// tool by name (assert id) and by id (assert name).
// ---------------------------------------------------------------------------

func TestAccAITool_dataSource(t *testing.T) {
	testAccPreCheck(t)

	c := testAccClient()
	if c == nil {
		t.Fatal("TestAccAITool_dataSource: MISTERSHELL_URL/MISTERSHELL_API_KEY not set")
	}
	tools, err := c.ListAITools(context.Background(), client.AIToolListFilter{})
	if err != nil {
		t.Fatalf("listing AI tools: %v", err)
	}
	if len(tools) == 0 {
		t.Skip("no AI tools on this instance; mistershell_ai_tool is backend-builtin and cannot be created")
	}
	tool := tools[0]

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccAIToolDataSourceConfig(tool.Name, tool.ID),
				Check: resource.ComposeAggregateTestCheckFunc(
					// looked up by name -> id matches
					resource.TestCheckResourceAttr("data.mistershell_ai_tool.by_name", "name", tool.Name),
					resource.TestCheckResourceAttr("data.mistershell_ai_tool.by_name", "id", fmt.Sprintf("%d", tool.ID)),
					// looked up by id -> name matches
					resource.TestCheckResourceAttr("data.mistershell_ai_tool.by_id", "id", fmt.Sprintf("%d", tool.ID)),
					resource.TestCheckResourceAttr("data.mistershell_ai_tool.by_id", "name", tool.Name),
				),
			},
		},
	})
}

// ---------------------------------------------------------------------------
// Config builders
// ---------------------------------------------------------------------------

func testAccAIModelConfig(name, modelID string) string {
	return fmt.Sprintf(`
resource "mistershell_ai_model" "test" {
  name     = %q
  model_provider = "ollama"
  model_id = %q

  config = jsonencode({
    base_url = "http://localhost:11434"
  })
}
`, name, modelID)
}

func testAccAIModelDataSourceConfig(name string) string {
	return fmt.Sprintf(`
resource "mistershell_ai_model" "test" {
  name     = %q
  model_provider = "ollama"
  model_id = "llama3"

  config = jsonencode({
    base_url = "http://localhost:11434"
  })
}

data "mistershell_ai_model" "by_id" {
  id = mistershell_ai_model.test.id
}

data "mistershell_ai_model" "by_name" {
  name = mistershell_ai_model.test.name
}
`, name)
}

func testAccAIPromptConfig(name, content, description string) string {
	return fmt.Sprintf(`
resource "mistershell_ai_prompt" "test" {
  name        = %q
  content     = %q
  description = %q
}
`, name, content, description)
}

func testAccAIPromptDataSourceConfig(name string) string {
	return fmt.Sprintf(`
resource "mistershell_ai_prompt" "test" {
  name    = %q
  content = "Summarize {{resource_id}}."
}

data "mistershell_ai_prompt" "by_id" {
  id = mistershell_ai_prompt.test.id
}

data "mistershell_ai_prompt" "by_name" {
  name = mistershell_ai_prompt.test.name
}
`, name)
}

func testAccAIAgentConfig(name, description string, tokenBudget int) string {
	return fmt.Sprintf(`
resource "mistershell_ai_model" "test" {
  name     = %q
  model_provider = "ollama"
  model_id = "llama3"

  config = jsonencode({
    base_url = "http://localhost:11434"
  })
}

resource "mistershell_ai_prompt" "test" {
  name    = %q
  content = "You are a helpful assistant."
}

resource "mistershell_ai_agent" "test" {
  name             = %q
  type             = "chat"
  description      = %q
  model_id         = mistershell_ai_model.test.id
  system_prompt_id = mistershell_ai_prompt.test.id

  config = jsonencode({
    token_budget = %d
  })
}
`, acctestPrefix+"agent-model", acctestPrefix+"agent-prompt", name, description, tokenBudget)
}

func testAccAIAgentDataSourceConfig(name string) string {
	return fmt.Sprintf(`
resource "mistershell_ai_model" "test" {
  name     = %q
  model_provider = "ollama"
  model_id = "llama3"

  config = jsonencode({
    base_url = "http://localhost:11434"
  })
}

resource "mistershell_ai_prompt" "test" {
  name    = %q
  content = "You are a helpful assistant."
}

resource "mistershell_ai_agent" "test" {
  name             = %q
  type             = "chat"
  model_id         = mistershell_ai_model.test.id
  system_prompt_id = mistershell_ai_prompt.test.id

  config = jsonencode({
    token_budget = 1000
  })
}

data "mistershell_ai_agent" "by_id" {
  id = mistershell_ai_agent.test.id
}

data "mistershell_ai_agent" "by_name" {
  name = mistershell_ai_agent.test.name
}
`, acctestPrefix+"agent-ds-model", acctestPrefix+"agent-ds-prompt", name)
}

func testAccAISkillConfig(name, body, isEnabled, agentTypes string) string {
	return fmt.Sprintf(`
resource "mistershell_ai_skill" "test" {
  name        = %q
  body        = %q
  is_enabled  = %s
  agent_types = %s
}
`, name, body, isEnabled, agentTypes)
}

func testAccAISkillDataSourceConfig(name string) string {
	return fmt.Sprintf(`
resource "mistershell_ai_skill" "test" {
  name        = %q
  body        = "# Data source skill"
  is_enabled  = true
  agent_types = ["chat"]
}

data "mistershell_ai_skill" "by_id" {
  id = mistershell_ai_skill.test.id
}

data "mistershell_ai_skill" "by_name" {
  name = mistershell_ai_skill.test.name
}
`, name)
}

func testAccAIToolDataSourceConfig(name string, id int64) string {
	return fmt.Sprintf(`
data "mistershell_ai_tool" "by_name" {
  name = %q
}

data "mistershell_ai_tool" "by_id" {
  id = %d
}
`, name, id)
}
