package resources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-mistershell/internal/client"
)

func TestNetworkResourceSchemaUsesOnboardingInputs(t *testing.T) {
	r := &NetworkResourceResource{}
	var resp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &resp)

	engine := requireStringAttribute(t, resp.Schema.Attributes["engine_id"])
	if !engine.Optional || !engine.Computed || engine.Required || len(engine.PlanModifiers) == 0 {
		t.Fatalf("engine_id schema = %#v", engine)
	}

	resourceType := requireStringAttribute(t, resp.Schema.Attributes["resource_type"])
	if !resourceType.Optional || !resourceType.Computed || resourceType.Required || len(resourceType.PlanModifiers) == 0 {
		t.Fatalf("resource_type schema = %#v", resourceType)
	}

	externalID := requireStringAttribute(t, resp.Schema.Attributes["external_id"])
	if !externalID.Computed || externalID.Optional || externalID.Required {
		t.Fatalf("external_id schema = %#v", externalID)
	}

	credential, ok := resp.Schema.Attributes["credential_id"].(resourceschema.Int64Attribute)
	if !ok || !credential.Required {
		t.Fatalf("credential_id schema = %#v", resp.Schema.Attributes["credential_id"])
	}
	connector, ok := resp.Schema.Attributes["connector_data"].(resourceschema.DynamicAttribute)
	if !ok || !connector.Required {
		t.Fatalf("connector_data schema = %#v", connector)
	}

	validators := r.ConfigValidators(context.Background())
	if len(validators) != 1 {
		t.Fatalf("config validators = %d, want 1", len(validators))
	}
}

func TestBuildNetworkResourceUpdateSendsOnlyChangedFields(t *testing.T) {
	state := NetworkResourceResourceModel{
		Name: types.StringValue("old"), LocationID: types.Int64Value(3),
		CredentialID: types.Int64Value(7), ConnectorData: testConnectorData("old"),
		IsEnabled: types.BoolValue(true),
	}
	plan := state
	plan.Name = types.StringValue("new")

	got, err := buildNetworkResourceUpdate(&plan, &state)
	if err != nil {
		t.Fatalf("buildNetworkResourceUpdate() error: %v", err)
	}
	if got.Name == nil || *got.Name != "new" || got.LocationID != nil || got.CredentialID != nil || got.ConnectorData != nil || got.IsEnabled != nil {
		t.Fatalf("name-only update = %#v", got)
	}

	plan.ConnectorData = testConnectorData("new")
	got, err = buildNetworkResourceUpdate(&plan, &state)
	if err != nil {
		t.Fatalf("buildNetworkResourceUpdate() error: %v", err)
	}
	if string(got.ConnectorData) != `{"host":"new"}` {
		t.Fatalf("connector update = %s", got.ConnectorData)
	}
}

func testConnectorData(host string) types.Dynamic {
	return types.DynamicValue(types.ObjectValueMust(
		map[string]attr.Type{"host": types.StringType},
		map[string]attr.Value{"host": types.StringValue(host)},
	))
}

func TestResolveOnboardingMode(t *testing.T) {
	t.Run("engine only selects automatic discovery", func(t *testing.T) {
		r := &NetworkResourceResource{}
		engine, expected, err := r.resolveOnboardingMode(context.Background(), &NetworkResourceResourceModel{
			EngineID: types.StringValue("ssh"), ResourceType: types.StringNull(),
		})
		if err != nil || engine != "ssh" || expected != nil {
			t.Fatalf("engine=%q expected=%v err=%v", engine, expected, err)
		}
	})

	t.Run("type only infers engine from runtime catalog", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"success":true,"message":"ok","data":[{"type":"cisco_ios","engine_id":"ssh","display_name":"Cisco IOS","description":"IOS","connector_schema":{},"credential_types":["username_password"]}]}`))
		}))
		defer srv.Close()
		r := &NetworkResourceResource{client: client.NewClient(srv.URL, "test", srv.Client())}
		engine, expected, err := r.resolveOnboardingMode(context.Background(), &NetworkResourceResourceModel{
			EngineID: types.StringNull(), ResourceType: types.StringValue("cisco_ios"),
		})
		if err != nil || engine != "ssh" || expected == nil || *expected != "cisco_ios" {
			t.Fatalf("engine=%q expected=%v err=%v", engine, expected, err)
		}
	})

	t.Run("inconsistent engine and type fail before discovery", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"success":true,"message":"ok","data":[{"type":"cisco_ios","engine_id":"ssh","display_name":"Cisco IOS","description":"IOS","connector_schema":{},"credential_types":["username_password"]}]}`))
		}))
		defer srv.Close()
		r := &NetworkResourceResource{client: client.NewClient(srv.URL, "test", srv.Client())}
		_, _, err := r.resolveOnboardingMode(context.Background(), &NetworkResourceResourceModel{
			EngineID: types.StringValue("db"), ResourceType: types.StringValue("cisco_ios"),
		})
		if err == nil || !strings.Contains(err.Error(), "uses engine") {
			t.Fatalf("error = %v", err)
		}
	})
}

func requireStringAttribute(t *testing.T, attribute resourceschema.Attribute) resourceschema.StringAttribute {
	t.Helper()
	got, ok := attribute.(resourceschema.StringAttribute)
	if !ok {
		t.Fatalf("attribute type = %T, want schema.StringAttribute", attribute)
	}
	return got
}
