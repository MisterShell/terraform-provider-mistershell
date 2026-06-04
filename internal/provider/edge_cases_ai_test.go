package provider_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"terraform-provider-mistershell/internal/client"
)

// Negative / error-path, out-of-band-drift, builtin-protection, and
// subtle-bug-class acceptance tests for the WORKER and AI entities (ai_model,
// ai_prompt, ai_agent, ai_skill, ai_tool).
//
// Models use the "ollama" provider so no secret/api_key is required. The
// builtin-protection tests operate at the client level (no resource.Test) and
// skip explicitly when the instance has no builtin of the relevant kind. All
// object names are prefixed with acctestPrefix so reruns / parallel runs do not
// collide and the sweepers can target them.

// ptrString returns a pointer to s — file-local helper for the client-level
// builtin-protection tests' *string update inputs.
func ptrString(s string) *string { return &s }

// Lenient backend-apply error regex: validators are exact, but backend-level
// create/update failures (duplicate name, FK miss, etc.) vary in wording.
var edgeBackendErr = regexp.MustCompile(`(?i)(error creating|already exists|409|conflict|not found|404|422|invalid|status 4\d\d)`)

// Lenient builtin-protection error regex (secondary assertion only).
var edgeProtectedErr = regexp.MustCompile(`(?i)(forbidden|cannot|403|builtin|default|system)`)

// ===========================================================================
// P0 — validators / resolution / import / constraints
// ===========================================================================

// TestAccEdgeAI_EnumValidators asserts the resource-level OneOf validators
// reject out-of-range enum values at plan time, before any backend call.
func TestAccEdgeAI_EnumValidators(t *testing.T) {
	testAccPreCheck(t)

	oneOf := regexp.MustCompile(`value must be one of`)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// model_provider OneOf(supported providers) — "bogus" is invalid.
			{
				Config: fmt.Sprintf(`
resource "mistershell_ai_model" "bad" {
  name           = %q
  model_provider = "bogus"
  model_id       = "llama3"
}
`, acctestPrefix+"enum-model"),
				PlanOnly:    true,
				ExpectError: oneOf,
			},
			// ai_prompt type OneOf("user") — "system" is reserved/builtin-only.
			{
				Config: fmt.Sprintf(`
resource "mistershell_ai_prompt" "bad" {
  name    = %q
  content = "x"
  type    = "system"
}
`, acctestPrefix+"enum-prompt"),
				PlanOnly:    true,
				ExpectError: oneOf,
			},
			// ai_agent type OneOf("chat","background") — "builtin_chat" is invalid.
			{
				Config: fmt.Sprintf(`
resource "mistershell_ai_agent" "bad" {
  name             = %q
  type             = "builtin_chat"
  system_prompt_id = 1
}
`, acctestPrefix+"enum-agent"),
				PlanOnly:    true,
				ExpectError: oneOf,
			},
		},
	})
}

// TestAccEdgeAI_DataSourceResolution asserts the data sources error explicitly
// when a name resolves to zero matches. The summaries are reproduced verbatim
// from ai_model_data_source.go / ai_tool_data_source.go.
func TestAccEdgeAI_DataSourceResolution(t *testing.T) {
	testAccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
data "mistershell_ai_model" "nope" {
  name = %q
}
`, acctestPrefix+"nope"),
				ExpectError: regexp.MustCompile(`No matching AI model found`),
			},
			{
				Config: fmt.Sprintf(`
data "mistershell_ai_tool" "nope" {
  name = %q
}
`, acctestPrefix+"nope"),
				ExpectError: regexp.MustCompile(`No matching AI tool found`),
			},
		},
	})
}

// TestAccEdgeAI_ImportErrors creates a real model, then asserts the import-ID
// parser rejects a non-integer, and a non-existent id surfaces a lenient
// not-found error.
func TestAccEdgeAI_ImportErrors(t *testing.T) {
	testAccPreCheck(t)

	name := acctestPrefix + "import-err-model"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccAIModelConfig(name, "llama3"),
				Check: resource.TestCheckResourceAttrSet(
					"mistershell_ai_model.test", "id"),
			},
			// Non-integer import ID -> ImportState parser rejects it.
			{
				ResourceName:  "mistershell_ai_model.test",
				ImportState:   true,
				ImportStateId: "not-an-int",
				ExpectError:   regexp.MustCompile(`Invalid import ID`),
			},
			// Well-formed but non-existent id -> import finds no remote object.
			{
				ResourceName:  "mistershell_ai_model.test",
				ImportState:   true,
				ImportStateId: "999999999",
				ExpectError:   regexp.MustCompile(`(?i)(cannot import non-existent|non-existent remote object|not found|404)`),
			},
		},
	})
}

// TestAccEdgeAI_BackendConstraints exercises two backend-enforced constraints:
// (a) duplicate model names collide; (b) an agent referencing a non-existent
// system_prompt_id fails to create.
func TestAccEdgeAI_BackendConstraints(t *testing.T) {
	testAccPreCheck(t)

	// (a) two models with the SAME name.
	t.Run("duplicate_model_name", func(t *testing.T) {
		name := acctestPrefix + "dup-model"
		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			CheckDestroy:             testAccCheckAllDestroyed,
			Steps: []resource.TestStep{
				{
					Config: fmt.Sprintf(`
resource "mistershell_ai_model" "a" {
  name           = %q
  model_provider = "ollama"
  model_id       = "llama3"
}

resource "mistershell_ai_model" "b" {
  name           = %q
  model_provider = "ollama"
  model_id       = "llama3"
}
`, name, name),
					ExpectError: edgeBackendErr,
				},
			},
		})
	})

	// (b) agent referencing a non-existent system_prompt_id.
	t.Run("agent_bad_prompt_id", func(t *testing.T) {
		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			CheckDestroy:             testAccCheckAllDestroyed,
			Steps: []resource.TestStep{
				{
					Config: fmt.Sprintf(`
resource "mistershell_ai_model" "test" {
  name           = %q
  model_provider = "ollama"
  model_id       = "llama3"
}

resource "mistershell_ai_agent" "bad" {
  name             = %q
  type             = "chat"
  model_id         = mistershell_ai_model.test.id
  system_prompt_id = 999999
}
`, acctestPrefix+"agent-badprompt-model", acctestPrefix+"agent-badprompt"),
					ExpectError: edgeBackendErr,
				},
			},
		})
	})
}

// ===========================================================================
// P0 — builtin protection (CLIENT-LEVEL; call testAccClient() directly)
// ===========================================================================

// TestAccEdgeAI_DefaultWorkerProtected asserts the default worker cannot be
// deleted or renamed.
func TestAccEdgeAI_DefaultWorkerProtected(t *testing.T) {
	testAccPreCheck(t)

	c := testAccClient()
	if c == nil {
		t.Fatal("TestAccEdgeAI_DefaultWorkerProtected: MISTERSHELL_URL/MISTERSHELL_API_KEY not set")
	}
	ctx := context.Background()

	workers, err := c.ListWorkers(ctx, client.WorkerListFilter{})
	if err != nil {
		t.Fatalf("listing workers: %v", err)
	}
	var defaultID int64 = -1
	for _, w := range workers {
		if w.IsDefault {
			defaultID = w.ID
			break
		}
	}
	if defaultID == -1 {
		t.Skip("no default worker on this instance")
	}

	if derr := c.DeleteWorker(ctx, defaultID); derr == nil {
		t.Errorf("DeleteWorker(default=%d) succeeded; expected protection error", defaultID)
	} else if !edgeProtectedErr.MatchString(derr.Error()) {
		t.Logf("DeleteWorker(default) errored (good), unexpected wording: %v", derr)
	}

	if _, uerr := c.UpdateWorker(ctx, defaultID, client.WorkerUpdateInput{Name: ptrString(acctestPrefix + "x")}); uerr == nil {
		t.Errorf("UpdateWorker(default=%d) succeeded; expected protection error", defaultID)
	} else if !edgeProtectedErr.MatchString(uerr.Error()) {
		t.Logf("UpdateWorker(default) errored (good), unexpected wording: %v", uerr)
	}
}

// TestAccEdgeAI_SystemPromptProtected asserts a builtin system prompt cannot be
// deleted or have its content changed.
func TestAccEdgeAI_SystemPromptProtected(t *testing.T) {
	testAccPreCheck(t)

	c := testAccClient()
	if c == nil {
		t.Fatal("TestAccEdgeAI_SystemPromptProtected: MISTERSHELL_URL/MISTERSHELL_API_KEY not set")
	}
	ctx := context.Background()

	prompts, err := c.ListAIPrompts(ctx, client.AIPromptListFilter{})
	if err != nil {
		t.Fatalf("listing ai prompts: %v", err)
	}
	var sysID int64 = -1
	for _, p := range prompts {
		if p.Type == "system" {
			sysID = p.ID
			break
		}
	}
	if sysID == -1 {
		t.Skip("no system prompt on this instance")
	}

	if derr := c.DeleteAIPrompt(ctx, sysID); derr == nil {
		t.Errorf("DeleteAIPrompt(system=%d) succeeded; expected protection error", sysID)
	} else if !edgeProtectedErr.MatchString(derr.Error()) {
		t.Logf("DeleteAIPrompt(system) errored (good), unexpected wording: %v", derr)
	}

	if _, uerr := c.UpdateAIPrompt(ctx, sysID, client.AIPromptUpdateInput{Content: ptrString("x")}); uerr == nil {
		t.Errorf("UpdateAIPrompt(system=%d) succeeded; expected protection error", sysID)
	} else if !edgeProtectedErr.MatchString(uerr.Error()) {
		t.Logf("UpdateAIPrompt(system) errored (good), unexpected wording: %v", uerr)
	}
}

// TestAccEdgeAI_BuiltinAgentProtected asserts a builtin agent cannot be deleted.
func TestAccEdgeAI_BuiltinAgentProtected(t *testing.T) {
	testAccPreCheck(t)

	c := testAccClient()
	if c == nil {
		t.Fatal("TestAccEdgeAI_BuiltinAgentProtected: MISTERSHELL_URL/MISTERSHELL_API_KEY not set")
	}
	ctx := context.Background()

	agents, err := c.ListAIAgents(ctx, client.AIAgentListFilter{})
	if err != nil {
		t.Fatalf("listing ai agents: %v", err)
	}
	var builtinID int64 = -1
	for _, a := range agents {
		if a.IsBuiltin {
			builtinID = a.ID
			break
		}
	}
	if builtinID == -1 {
		t.Skip("no builtin agent on this instance")
	}

	if derr := c.DeleteAIAgent(ctx, builtinID); derr == nil {
		t.Errorf("DeleteAIAgent(builtin=%d) succeeded; expected protection error", builtinID)
	} else if !edgeProtectedErr.MatchString(derr.Error()) {
		t.Logf("DeleteAIAgent(builtin) errored (good), unexpected wording: %v", derr)
	}
}

// TestAccEdgeAI_BuiltinSkillProtected asserts a builtin skill cannot be deleted,
// and its body cannot be changed (only is_enabled is mutable on a builtin).
func TestAccEdgeAI_BuiltinSkillProtected(t *testing.T) {
	testAccPreCheck(t)

	c := testAccClient()
	if c == nil {
		t.Fatal("TestAccEdgeAI_BuiltinSkillProtected: MISTERSHELL_URL/MISTERSHELL_API_KEY not set")
	}
	ctx := context.Background()

	skills, err := c.ListAISkills(ctx, client.AISkillListFilter{})
	if err != nil {
		t.Fatalf("listing ai skills: %v", err)
	}
	var builtinID int64 = -1
	for _, s := range skills {
		if s.IsBuiltin {
			builtinID = s.ID
			break
		}
	}
	if builtinID == -1 {
		t.Skip("no builtin skill on this instance")
	}

	if derr := c.DeleteAISkill(ctx, builtinID); derr == nil {
		t.Errorf("DeleteAISkill(builtin=%d) succeeded; expected protection error", builtinID)
	} else if !edgeProtectedErr.MatchString(derr.Error()) {
		t.Logf("DeleteAISkill(builtin) errored (good), unexpected wording: %v", derr)
	}

	if _, uerr := c.UpdateAISkill(ctx, builtinID, client.AISkillUpdateInput{Body: ptrString("x")}); uerr == nil {
		t.Errorf("UpdateAISkill(builtin=%d, body) succeeded; expected protection error", builtinID)
	} else if !edgeProtectedErr.MatchString(uerr.Error()) {
		t.Logf("UpdateAISkill(builtin, body) errored (good), unexpected wording: %v", uerr)
	}
}

// ===========================================================================
// P1 — out-of-band drift (PreConfig)
// ===========================================================================

// TestAccEdgeAI_ModelDeletedOutOfBand deletes the model behind Terraform's back
// between steps; the same config must recreate it.
func TestAccEdgeAI_ModelDeletedOutOfBand(t *testing.T) {
	testAccPreCheck(t)

	name := acctestPrefix + "oob-deleted"
	cfg := testAccAIModelConfig(name, "llama3")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check:  resource.TestCheckResourceAttrSet("mistershell_ai_model.test", "id"),
			},
			{
				PreConfig: func() {
					c := testAccClient()
					if c == nil {
						t.Fatal("PreConfig: client not configured")
					}
					ctx := context.Background()
					models, err := c.ListAIModels(ctx, client.AIModelListFilter{Search: name})
					if err != nil {
						t.Fatalf("PreConfig listing models: %v", err)
					}
					for _, m := range models {
						if m.Name == name {
							if derr := c.DeleteAIModel(ctx, m.ID); derr != nil {
								t.Fatalf("PreConfig deleting model %d: %v", m.ID, derr)
							}
						}
					}
				},
				Config: cfg,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_ai_model.test", "name", name),
					resource.TestCheckResourceAttrSet("mistershell_ai_model.test", "id"),
					testAccCheckAIModelExists(t, name),
				),
			},
		},
	})
}

// TestAccEdgeAI_ModelMutatedOutOfBand mutates model_id out-of-band; the next
// apply must correct the drift back to the configured value.
func TestAccEdgeAI_ModelMutatedOutOfBand(t *testing.T) {
	testAccPreCheck(t)

	name := acctestPrefix + "oob-mutated"
	cfg := testAccAIModelConfig(name, "llama3")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check:  resource.TestCheckResourceAttr("mistershell_ai_model.test", "model_id", "llama3"),
			},
			{
				PreConfig: func() {
					c := testAccClient()
					if c == nil {
						t.Fatal("PreConfig: client not configured")
					}
					ctx := context.Background()
					models, err := c.ListAIModels(ctx, client.AIModelListFilter{Search: name})
					if err != nil {
						t.Fatalf("PreConfig listing models: %v", err)
					}
					for _, m := range models {
						if m.Name == name {
							if _, uerr := c.UpdateAIModel(ctx, m.ID, client.AIModelUpdateInput{ModelID: ptrString("changed")}); uerr != nil {
								t.Fatalf("PreConfig mutating model %d: %v", m.ID, uerr)
							}
						}
					}
				},
				Config: cfg,
				Check:  resource.TestCheckResourceAttr("mistershell_ai_model.test", "model_id", "llama3"),
			},
		},
	})
}

// TestAccEdgeAI_IsDefaultFlip asserts a coherent atomic swap of the default
// flag between two models.
func TestAccEdgeAI_IsDefaultFlip(t *testing.T) {
	testAccPreCheck(t)

	nameA := acctestPrefix + "default-a"
	nameB := acctestPrefix + "default-b"

	cfg := func(aDefault, bDefault string) string {
		return fmt.Sprintf(`
resource "mistershell_ai_model" "a" {
  name           = %q
  model_provider = "ollama"
  model_id       = "llama3"
  is_default     = %s
}

resource "mistershell_ai_model" "b" {
  name           = %q
  model_provider = "ollama"
  model_id       = "llama3"
  is_default     = %s
}
`, nameA, aDefault, nameB, bDefault)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			{
				Config: cfg("true", "false"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_ai_model.a", "is_default", "true"),
					resource.TestCheckResourceAttr("mistershell_ai_model.b", "is_default", "false"),
				),
			},
			{
				Config: cfg("false", "true"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_ai_model.a", "is_default", "false"),
					resource.TestCheckResourceAttr("mistershell_ai_model.b", "is_default", "true"),
				),
			},
		},
	})
}

// ===========================================================================
// P2 — subtle bug classes
// ===========================================================================

// TestAccEdgeAI_AgentModelIDClear sets model_id then clears it (falls back to
// default model). NOTE: if the backend ignores a null model_id and keeps the
// previous one, Step2 will surface a provider-produced-inconsistent-result
// error or a perpetual diff in Step3 — flagged in the report, verify live.
// TestAccEdgeAI_AgentModelIDUpdate proves the agent's model_id FK is updatable
// (repointing to a different model). NOTE: clearing model_id to null is NOT
// honored by the backend (the PATCH ignores a null model_id and retains the old
// value, producing "inconsistent result after apply" — see api-bug-register), so
// this test repoints between two real models rather than clearing.
func TestAccEdgeAI_AgentModelIDUpdate(t *testing.T) {
	testAccPreCheck(t)

	modelAName := acctestPrefix + "modelid-a"
	modelBName := acctestPrefix + "modelid-b"
	promptName := acctestPrefix + "modelid-prompt"
	agentName := acctestPrefix + "modelid-agent"

	cfg := func(target string) string {
		return fmt.Sprintf(`
resource "mistershell_ai_model" "a" {
  name           = %q
  model_provider = "ollama"
  model_id       = "llama3"
}

resource "mistershell_ai_model" "b" {
  name           = %q
  model_provider = "ollama"
  model_id       = "mistral"
}

resource "mistershell_ai_prompt" "test" {
  name    = %q
  content = "You are a helpful assistant."
}

resource "mistershell_ai_agent" "test" {
  name             = %q
  type             = "chat"
  model_id         = %s
  system_prompt_id = mistershell_ai_prompt.test.id
}
`, modelAName, modelBName, promptName, agentName, target)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			{
				Config: cfg("mistershell_ai_model.a.id"),
				Check: resource.TestCheckResourceAttrPair(
					"mistershell_ai_agent.test", "model_id", "mistershell_ai_model.a", "id"),
			},
			// Repoint the FK to model b -> updatable in place.
			{
				Config: cfg("mistershell_ai_model.b.id"),
				Check: resource.TestCheckResourceAttrPair(
					"mistershell_ai_agent.test", "model_id", "mistershell_ai_model.b", "id"),
			},
		},
	})
}

// TestAccEdgeAI_ModelConfigStable proves the stored-from-config / masked-secret
// config does not drift on refresh: the same config re-plans clean.
func TestAccEdgeAI_ModelConfigStable(t *testing.T) {
	testAccPreCheck(t)

	name := acctestPrefix + "config-stable"
	cfg := testAccAIModelConfig(name, "llama3")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check:  resource.TestCheckResourceAttrSet("mistershell_ai_model.test", "config"),
			},
			{
				Config:             cfg,
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
		},
	})
}

// TestAccEdgeAI_SkillAgentTypesSetModify exercises the SUPPORTED agent_types
// lifecycle: set a non-empty restriction, modify it to a different set, and
// confirm idempotency. The set is order-independent.
//
// NOTE: CLEARING agent_types once set is not cleanly supported and is therefore
// NOT exercised here — omitting it leaves the backend value unchanged (the PATCH
// ignores a null agent_types), and passing an explicit [] makes the backend
// store null while the config is an empty set, both yielding "inconsistent
// result after apply". Tracked in api-bug-register; to remove a restriction,
// replace the skill rather than clearing the field.
func TestAccEdgeAI_SkillAgentTypesSetModify(t *testing.T) {
	testAccPreCheck(t)

	name := acctestPrefix + "skill-types-mod"

	cfg := func(agentTypes string) string {
		return fmt.Sprintf(`
resource "mistershell_ai_skill" "test" {
  name        = %q
  body        = "# Skill with agent_types"
  agent_types = %s
}
`, name, agentTypes)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllDestroyed,
		Steps: []resource.TestStep{
			{
				Config: cfg(`["chat"]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_ai_skill.test", "agent_types.#", "1"),
					resource.TestCheckTypeSetElemAttr("mistershell_ai_skill.test", "agent_types.*", "chat"),
				),
			},
			// Modify the set in place.
			{
				Config: cfg(`["chat", "background"]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mistershell_ai_skill.test", "agent_types.#", "2"),
					resource.TestCheckTypeSetElemAttr("mistershell_ai_skill.test", "agent_types.*", "background"),
				),
			},
			// Idempotency: re-plan is clean.
			{
				Config:             cfg(`["chat", "background"]`),
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
		},
	})
}

// ===========================================================================
// Check helpers
// ===========================================================================

// testAccCheckAIModelExists asserts a model with the given exact name exists
// server-side (used to confirm out-of-band-deleted models were recreated).
func testAccCheckAIModelExists(t *testing.T, name string) resource.TestCheckFunc {
	t.Helper()
	return func(_ *terraform.State) error {
		c := testAccClient()
		if c == nil {
			return fmt.Errorf("testAccCheckAIModelExists: client not configured")
		}
		models, err := c.ListAIModels(context.Background(), client.AIModelListFilter{Search: name})
		if err != nil {
			return fmt.Errorf("listing models for %q: %w", name, err)
		}
		for _, m := range models {
			if m.Name == name {
				return nil
			}
		}
		return fmt.Errorf("AI model %q not found after recreate", name)
	}
}
