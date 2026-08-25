package provider

import (
	"context"
	"strings"
	"testing"

	frameworkprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	providerschema "github.com/hashicorp/terraform-plugin-framework/provider/schema"
)

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
