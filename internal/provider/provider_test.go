package provider_test

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"

	"terraform-provider-mistershell/internal/provider"
)

var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"mistershell": providerserver.NewProtocol6WithError(provider.New("test")()),
}

func testAccPreCheck(t *testing.T) {
	t.Helper()
	if os.Getenv("MISTERSHELL_URL") == "" {
		t.Fatal("MISTERSHELL_URL must be set for acceptance tests")
	}
	if os.Getenv("MISTERSHELL_API_KEY") == "" {
		t.Fatal("MISTERSHELL_API_KEY must be set for acceptance tests")
	}
}
