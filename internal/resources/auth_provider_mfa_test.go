package resources

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestAuthProviderValidateConfigMfaMethod(t *testing.T) {
	ctx := context.Background()
	var schema resource.SchemaResponse
	(&AuthProviderResource{}).Schema(ctx, resource.SchemaRequest{}, &schema)
	ldapConfig := func(method attr.Value) types.Dynamic {
		attrs := map[string]attr.Value{"server_url": types.StringValue("ldaps://ldap.example.com")}
		attrTypes := map[string]attr.Type{"server_url": types.StringType}
		if method != nil {
			attrs["mfa_method"], attrTypes["mfa_method"] = method, method.Type(ctx)
		}
		return types.DynamicValue(types.ObjectValueMust(attrTypes, attrs))
	}
	for _, tc := range []struct {
		name    string
		config  types.Dynamic
		wantErr string
	}{
		{"omitted", ldapConfig(nil), ""},
		{"none", ldapConfig(types.StringValue("none")), ""},
		{"totp", ldapConfig(types.StringValue("totp")), ""},
		{"unknown", ldapConfig(types.StringUnknown()), ""},
		{"email", ldapConfig(types.StringValue("email")), "Email MFA cannot be managed with Terraform"},
		{"other", ldapConfig(types.StringValue("sms")), "config.mfa_method must be one of: none, totp"},
		{"map config", types.DynamicValue(types.MapValueMust(types.StringType, map[string]attr.Value{"mfa_method": types.StringValue("email")})), "Email MFA"},
		{"unknown config", types.DynamicUnknown(), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := tfsdk.State{Schema: schema.Schema}
			if diags := state.Set(ctx, &AuthProviderResourceModel{
				ID: types.Int64Null(), Name: types.StringValue("corp"), Mnemonic: types.StringValue("corp"),
				ProviderType: types.StringValue("LDAP"), IsEnabled: types.BoolNull(), DisplayOrder: types.Int64Null(),
				Config: tc.config, GroupMappingsCount: types.Int64Null(), SPEntityID: types.StringNull(),
				SPACSURL: types.StringNull(), OIDCRedirectURI: types.StringNull(),
				CreatedAt: types.StringNull(), UpdatedAt: types.StringNull(),
			}); diags.HasError() {
				t.Fatal(diags)
			}
			var resp resource.ValidateConfigResponse
			(&AuthProviderResource{}).ValidateConfig(ctx, resource.ValidateConfigRequest{
				Config: tfsdk.Config{Schema: schema.Schema, Raw: state.Raw},
			}, &resp)
			got := ""
			for _, d := range resp.Diagnostics.Errors() {
				got += d.Detail() + ";"
			}
			if (tc.wantErr == "") != (got == "") || !strings.Contains(got, tc.wantErr) {
				t.Fatalf("diagnostics = %q, want %q", got, tc.wantErr)
			}
		})
	}
}
