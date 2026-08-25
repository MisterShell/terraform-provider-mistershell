package resources

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
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
}

func TestBuildWorkerCreateInputPreservesDisabledValue(t *testing.T) {
	got := buildWorkerCreateInput(&WorkerResourceModel{
		Name: types.StringValue("worker"), LocationID: types.Int64Value(2),
		Description: types.StringNull(), Config: jsontypes.NewNormalizedNull(),
		ConfigSchema: jsontypes.NewNormalizedNull(), IsEnabled: types.BoolValue(false),
	})
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
	mapLocationResponseToModel(&client.LocationResponse{ID: 1, Name: "site"}, &state)
	if !state.ResourceCount.IsNull() || !state.Path.IsNull() {
		t.Fatalf("detail-only values after mutation: resource_count=%v path=%v", state.ResourceCount, state.Path)
	}
}
