package datasources

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
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
