package provider

import (
	"context"
	"crypto/x509"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	frameworkprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	providerschema "github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-mistershell/internal/client"
)

func TestProviderTLSConfiguration(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/settings/app_base_url" || r.Header.Get("Authorization") != "Bearer apikey_test" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = io.WriteString(w, `{"success":true,"data":{"key":"app_base_url","value":"https://example.com"}}`)
	}))
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	t.Cleanup(server.Close)

	for _, tc := range []struct {
		name       string
		env        string
		configured types.Bool
		wantOK     bool
	}{
		{"secure default", "", types.BoolNull(), false},
		{"environment true", "true", types.BoolNull(), true},
		{"environment one", "1", types.BoolNull(), true},
		{"environment mixed case", "TrUe", types.BoolNull(), true},
		{"environment false", "false", types.BoolNull(), false},
		{"invalid environment stays secure", "invalid", types.BoolNull(), false},
		{"explicit false overrides environment", "true", types.BoolValue(false), false},
		{"explicit true overrides environment", "false", types.BoolValue(true), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("MISTERSHELL_INSECURE", tc.env)
			p := New("test")()
			ctx := context.Background()
			var schema frameworkprovider.SchemaResponse
			p.Schema(ctx, frameworkprovider.SchemaRequest{}, &schema)
			state := tfsdk.State{Schema: schema.Schema}
			diags := state.Set(ctx, MisterShellProviderModel{
				URL: types.StringValue(server.URL), APIKey: types.StringValue("apikey_test"), Insecure: tc.configured,
			})
			if diags.HasError() {
				t.Fatal(diags)
			}
			var configured frameworkprovider.ConfigureResponse
			p.Configure(ctx, frameworkprovider.ConfigureRequest{Config: tfsdk.Config{Schema: schema.Schema, Raw: state.Raw}}, &configured)
			if configured.Diagnostics.HasError() {
				t.Fatal(configured.Diagnostics)
			}
			c := configured.ResourceData.(*client.Client)
			_, err := c.GetSetting(ctx, "app_base_url")
			if tc.wantOK {
				if err != nil {
					t.Fatalf("self-signed API request failed: %v", err)
				}
			} else {
				var unknownCA x509.UnknownAuthorityError
				if !errors.As(err, &unknownCA) {
					t.Fatalf("expected untrusted-certificate rejection, got %v", err)
				}
			}
		})
	}
}

func TestProviderAPIKeyDescriptionUsesMisterShell2Prefix(t *testing.T) {
	p := New("test")()
	var resp frameworkprovider.SchemaResponse
	p.Schema(context.Background(), frameworkprovider.SchemaRequest{}, &resp)

	attribute, ok := resp.Schema.Attributes["api_key"].(providerschema.StringAttribute)
	if !ok {
		t.Fatalf("api_key schema type = %T", resp.Schema.Attributes["api_key"])
	}
	if !strings.Contains(attribute.Description, "apikey_") {
		t.Fatalf("api_key description = %q, want apikey_ prefix", attribute.Description)
	}
	if strings.Contains(attribute.Description, "yami_") {
		t.Fatalf("api_key description still contains legacy prefix: %q", attribute.Description)
	}
}
