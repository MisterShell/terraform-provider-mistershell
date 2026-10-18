package resources

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-mistershell/internal/client"
)

func TestMisterShell2ManagedResourceSchemas(t *testing.T) {
	t.Run("role exposes location scope mode", func(t *testing.T) {
		var resp resource.SchemaResponse
		(&RoleResource{}).Schema(context.Background(), resource.SchemaRequest{}, &resp)
		attribute, ok := resp.Schema.Attributes["location_scope_mode"].(resourceschema.StringAttribute)
		if !ok || !attribute.Optional || !attribute.Computed || len(attribute.Validators) == 0 {
			t.Fatalf("location_scope_mode schema = %#v", resp.Schema.Attributes["location_scope_mode"])
		}
	})

	t.Run("auth provider requires immutable mnemonic and exposes OIDC redirect", func(t *testing.T) {
		var resp resource.SchemaResponse
		(&AuthProviderResource{}).Schema(context.Background(), resource.SchemaRequest{}, &resp)
		mnemonic, ok := resp.Schema.Attributes["mnemonic"].(resourceschema.StringAttribute)
		if !ok || !mnemonic.Required || len(mnemonic.PlanModifiers) == 0 || len(mnemonic.Validators) == 0 {
			t.Fatalf("mnemonic schema = %#v", resp.Schema.Attributes["mnemonic"])
		}
		redirect, ok := resp.Schema.Attributes["oidc_redirect_uri"].(resourceschema.StringAttribute)
		if !ok || !redirect.Computed || redirect.Optional || redirect.Required {
			t.Fatalf("oidc_redirect_uri schema = %#v", resp.Schema.Attributes["oidc_redirect_uri"])
		}
	})

	t.Run("session policy rule accepts an optional approval block", func(t *testing.T) {
		var resp resource.SchemaResponse
		(&SessionPolicyRuleResource{}).Schema(context.Background(), resource.SchemaRequest{}, &resp)
		approval, ok := resp.Schema.Attributes["approval"].(resourceschema.SingleNestedAttribute)
		if !ok || !approval.Optional || approval.Computed {
			t.Fatalf("approval schema = %#v", resp.Schema.Attributes["approval"])
		}
		if roles, ok := approval.Attributes["approver_role_ids"].(resourceschema.SetAttribute); !ok || !roles.Required {
			t.Fatalf("approver_role_ids schema = %#v", approval.Attributes["approver_role_ids"])
		}
	})

	t.Run("user exposes read-only auth provider id", func(t *testing.T) {
		var resp resource.SchemaResponse
		(&UserResource{}).Schema(context.Background(), resource.SchemaRequest{}, &resp)
		attribute, ok := resp.Schema.Attributes["auth_provider_id"].(resourceschema.Int64Attribute)
		if !ok || !attribute.Computed || attribute.Optional || attribute.Required {
			t.Fatalf("auth_provider_id schema = %#v", resp.Schema.Attributes["auth_provider_id"])
		}
	})
}

func TestJSONPayloadResourceAttributesUseNativeTerraformValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		resource  resource.Resource
		attribute string
		required  bool
		optional  bool
		computed  bool
		sensitive bool
	}{
		{name: "location extra_data", resource: &LocationResource{}, attribute: "extra_data", optional: true},
		{name: "credential credential_data", resource: &CredentialResource{}, attribute: "credential_data", optional: true, sensitive: true},
		{name: "credential extra_data", resource: &CredentialResource{}, attribute: "extra_data", optional: true},
		{name: "resource connector_data", resource: &NetworkResourceResource{}, attribute: "connector_data", required: true},
		{name: "resource extra_data", resource: &NetworkResourceResource{}, attribute: "extra_data", optional: true, computed: true},
		{name: "resource last_connectivity_result", resource: &NetworkResourceResource{}, attribute: "last_connectivity_result", computed: true},
		{name: "setting value", resource: &SettingResource{}, attribute: "value", required: true, sensitive: true},
		{name: "setting default", resource: &SettingResource{}, attribute: "default", computed: true},
		{name: "setting schema", resource: &SettingResource{}, attribute: "schema", computed: true},
		{name: "worker config", resource: &WorkerResource{}, attribute: "config", optional: true},
		{name: "worker config_schema", resource: &WorkerResource{}, attribute: "config_schema", optional: true},
		{name: "AI model config", resource: &AIModelResource{}, attribute: "config", optional: true, sensitive: true},
		{name: "AI prompt variable_schema", resource: &AIPromptResource{}, attribute: "variable_schema", optional: true},
		{name: "AI agent config", resource: &AIAgentResource{}, attribute: "config", optional: true},
		{name: "auth provider config", resource: &AuthProviderResource{}, attribute: "config", required: true, sensitive: true},
		{name: "log destination config", resource: &LogDestinationResource{}, attribute: "config", required: true, sensitive: true},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var resp resource.SchemaResponse
			test.resource.Schema(context.Background(), resource.SchemaRequest{}, &resp)
			attribute, ok := resp.Schema.Attributes[test.attribute].(resourceschema.DynamicAttribute)
			if !ok {
				t.Fatalf("%s schema type = %T, want schema.DynamicAttribute", test.attribute, resp.Schema.Attributes[test.attribute])
			}
			if attribute.Required != test.required || attribute.Optional != test.optional || attribute.Computed != test.computed || attribute.Sensitive != test.sensitive {
				t.Fatalf("%s schema flags = required:%t optional:%t computed:%t sensitive:%t", test.attribute, attribute.Required, attribute.Optional, attribute.Computed, attribute.Sensitive)
			}
		})
	}
}

func TestBuildWorkerCreateInputPreservesDisabledValue(t *testing.T) {
	got, err := buildWorkerCreateInput(&WorkerResourceModel{
		Name: types.StringValue("worker"), LocationID: types.Int64Value(2),
		Description: types.StringNull(), Config: types.DynamicNull(),
		ConfigSchema: types.DynamicNull(), IsEnabled: types.BoolValue(false),
	})
	if err != nil {
		t.Fatalf("buildWorkerCreateInput() error: %v", err)
	}
	if got.IsEnabled {
		t.Fatal("is_enabled=false was not preserved")
	}
}

func TestWorkerCreatedStateHasNoUnknownRuntimeValues(t *testing.T) {
	state := WorkerResourceModel{
		ActiveTaskCount: types.Int64Unknown(), TotalTaskCount: types.Int64Unknown(), LastHeartbeatAt: types.StringUnknown(),
		Version: types.StringUnknown(), RDKContractMajor: types.Int64Unknown(), RDKCatalogID: types.StringUnknown(),
		RDKCompatible: types.BoolUnknown(), ConnectedAt: types.StringUnknown(), PresenceIP: types.StringUnknown(),
		PresenceIPSource: types.StringUnknown(),
	}
	mapWorkerCreatedResponseToModel(&client.WorkerCreatedResponse{ID: 1, Name: "worker"}, &state)
	unknown := map[string]bool{
		"active_task_count": state.ActiveTaskCount.IsUnknown(), "total_task_count": state.TotalTaskCount.IsUnknown(),
		"last_heartbeat_at": state.LastHeartbeatAt.IsUnknown(), "version": state.Version.IsUnknown(),
		"rdk_contract_major": state.RDKContractMajor.IsUnknown(), "rdk_catalog_identity": state.RDKCatalogID.IsUnknown(),
		"rdk_compatible": state.RDKCompatible.IsUnknown(), "connected_at": state.ConnectedAt.IsUnknown(),
		"presence_ip": state.PresenceIP.IsUnknown(), "presence_ip_source": state.PresenceIPSource.IsUnknown(),
	}
	for name, isUnknown := range unknown {
		if isUnknown {
			t.Errorf("%s remains unknown after create", name)
		}
	}
}

func TestLocationMutationResponseDoesNotInventDetailFields(t *testing.T) {
	var state LocationResourceModel
	if err := mapLocationResponseToModel(&client.LocationResponse{ID: 1, Name: "site"}, &state); err != nil {
		t.Fatalf("mapLocationResponseToModel() error: %v", err)
	}
	if !state.ResourceCount.IsNull() || !state.Path.IsNull() {
		t.Fatalf("detail-only values after mutation: resource_count=%v path=%v", state.ResourceCount, state.Path)
	}
}
