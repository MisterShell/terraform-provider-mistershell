package datasources

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestMisterShell2DataSourceSchemas(t *testing.T) {
	tests := []struct {
		name       string
		dataSource datasource.DataSource
		attributes []string
	}{
		{"resource", NewNetworkResourceDataSource(), []string{"engine_id"}},
		{"role", NewRoleDataSource(), []string{"location_scope_mode"}},
		{"auth provider", NewAuthProviderDataSource(), []string{"mnemonic", "oidc_redirect_uri"}},
		{"worker", NewWorkerDataSource(), []string{"version", "rdk_contract_major", "rdk_catalog_identity", "rdk_compatible"}},
		{"AI tool", NewAIToolDataSource(), []string{"restricted_to_agent_types"}},
		{"setting", NewSettingDataSource(), []string{"schema", "is_encrypted"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var response datasource.SchemaResponse
			test.dataSource.Schema(context.Background(), datasource.SchemaRequest{}, &response)
			if response.Diagnostics.HasError() {
				t.Fatalf("schema diagnostics: %v", response.Diagnostics)
			}
			for _, name := range test.attributes {
				attribute, ok := response.Schema.Attributes[name]
				if !ok {
					t.Errorf("missing MisterShell 2.0 attribute %q", name)
					continue
				}
				if !attribute.IsComputed() {
					t.Errorf("attribute %q must be computed", name)
				}
			}
		})
	}
}

func TestLogDestinationPresetsEmptyResponseIsEmptySequence(t *testing.T) {
	t.Parallel()

	got, err := logDestinationPresetsToDynamic(nil)
	if err != nil {
		t.Fatalf("logDestinationPresetsToDynamic() error: %v", err)
	}
	tuple, ok := got.UnderlyingValue().(types.Tuple)
	if !ok {
		t.Fatalf("presets underlying type = %T, want types.Tuple", got.UnderlyingValue())
	}
	if len(tuple.Elements()) != 0 {
		t.Fatalf("presets element count = %d, want 0", len(tuple.Elements()))
	}
}

func TestJSONPayloadDataSourceAttributesUseNativeTerraformValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		dataSource datasource.DataSource
		attribute  string
		sensitive  bool
	}{
		{name: "location extra_data", dataSource: NewLocationDataSource(), attribute: "extra_data"},
		{name: "credential credential_data", dataSource: NewCredentialDataSource(), attribute: "credential_data", sensitive: true},
		{name: "credential extra_data", dataSource: NewCredentialDataSource(), attribute: "extra_data"},
		{name: "resource connector_data", dataSource: NewNetworkResourceDataSource(), attribute: "connector_data"},
		{name: "resource extra_data", dataSource: NewNetworkResourceDataSource(), attribute: "extra_data"},
		{name: "resource last_connectivity_result", dataSource: NewNetworkResourceDataSource(), attribute: "last_connectivity_result"},
		{name: "setting value", dataSource: NewSettingDataSource(), attribute: "value", sensitive: true},
		{name: "setting default", dataSource: NewSettingDataSource(), attribute: "default"},
		{name: "setting schema", dataSource: NewSettingDataSource(), attribute: "schema"},
		{name: "worker config", dataSource: NewWorkerDataSource(), attribute: "config"},
		{name: "worker config_schema", dataSource: NewWorkerDataSource(), attribute: "config_schema"},
		{name: "AI model config", dataSource: NewAIModelDataSource(), attribute: "config", sensitive: true},
		{name: "AI prompt variable_schema", dataSource: NewAIPromptDataSource(), attribute: "variable_schema"},
		{name: "AI agent config", dataSource: NewAIAgentDataSource(), attribute: "config"},
		{name: "AI tool input_schema", dataSource: NewAIToolDataSource(), attribute: "input_schema"},
		{name: "auth provider config", dataSource: NewAuthProviderDataSource(), attribute: "config", sensitive: true},
		{name: "log destination config", dataSource: NewLogDestinationDataSource(), attribute: "config", sensitive: true},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var response datasource.SchemaResponse
			test.dataSource.Schema(context.Background(), datasource.SchemaRequest{}, &response)
			attribute, ok := response.Schema.Attributes[test.attribute].(datasourceschema.DynamicAttribute)
			if !ok {
				t.Fatalf("%s schema type = %T, want schema.DynamicAttribute", test.attribute, response.Schema.Attributes[test.attribute])
			}
			if !attribute.Computed || attribute.Optional || attribute.Required || attribute.Sensitive != test.sensitive {
				t.Fatalf("%s schema flags = required:%t optional:%t computed:%t sensitive:%t", test.attribute, attribute.Required, attribute.Optional, attribute.Computed, attribute.Sensitive)
			}
		})
	}
}

func TestLogDestinationPresetsUseNativeTerraformValue(t *testing.T) {
	t.Parallel()

	var response datasource.SchemaResponse
	NewLogDestinationPresetsDataSource().Schema(context.Background(), datasource.SchemaRequest{}, &response)
	presets, ok := response.Schema.Attributes["presets"].(datasourceschema.DynamicAttribute)
	if !ok || !presets.Computed {
		t.Fatalf("presets schema = %#v", response.Schema.Attributes["presets"])
	}
}
