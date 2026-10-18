package provider_test

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccAuditCAUnknownCertificateBundleRejectedWithoutMutation(t *testing.T) {
	testAccPreCheck(t)
	ctx := context.Background()
	c := testAccClient()
	before, err := c.ConfigurationList(ctx, "/api/v1/ca-certificates", nil)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, raw := range before {
		var row map[string]json.RawMessage
		json.Unmarshal(raw, &row)
		ids[string(row["id"])] = true
	}
	// Cleanup protects the disposable store if this regression ever recurs; the
	// assertion below still fails if even one unmanaged trust entry was created.
	t.Cleanup(func() {
		rows, err := c.ConfigurationList(ctx, "/api/v1/ca-certificates", nil)
		if err != nil {
			t.Error(err)
			return
		}
		for _, raw := range rows {
			var row map[string]json.RawMessage
			json.Unmarshal(raw, &row)
			id := string(row["id"])
			if !ids[id] {
				if _, err := c.ConfigurationRequest(ctx, "DELETE", "/api/v1/ca-certificates/"+id, nil); err != nil {
					t.Error(err)
				}
			}
		}
	})
	bundle := auditCACertificate(t) + auditCACertificate(t)
	config := fmt.Sprintf(`resource "terraform_data" "certificate_bundle" {
 input=%q
}
resource "mistershell_ca_certificate" "audit_unknown" {
 name=%q
 pem=terraform_data.certificate_bundle.output
}`, bundle, acctestPrefix+"unknown-ca-bundle")
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, Steps: []resource.TestStep{{Config: config, ExpectError: regexp.MustCompile("Single certificate required")}}})
	after, err := c.ConfigurationList(ctx, "/api/v1/ca-certificates", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != len(after) {
		t.Fatalf("rejected bundle mutated CA trust store: before=%d after=%d", len(before), len(after))
	}
	for _, raw := range after {
		var row map[string]json.RawMessage
		json.Unmarshal(raw, &row)
		if !ids[string(row["id"])] {
			t.Fatal("rejected bundle created an unmanaged trust entry")
		}
	}
}
