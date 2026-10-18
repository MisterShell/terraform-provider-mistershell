package apimanaged

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"terraform-provider-mistershell/internal/client"
)

const auditCertificateBlock = "-----BEGIN CERTIFICATE-----\nQUJD\n-----END CERTIFICATE-----\n"

func auditCAResource(t *testing.T) (*ManagedResource, tfsdk.Plan) {
	t.Helper()
	ctx := context.Background()
	var d Definition
	for _, v := range Definitions() {
		if v.Name == "ca_certificate" {
			d = v
		}
	}
	r := &ManagedResource{definition: d}
	var resp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &resp)
	values := map[string]tftypes.Value{}
	for name, a := range resp.Schema.Attributes {
		values[name] = tftypes.NewValue(a.GetType().TerraformType(ctx), nil)
	}
	return r, tfsdk.Plan{Schema: resp.Schema, Raw: tftypes.NewValue(resp.Schema.Type().TerraformType(ctx), values)}
}
func TestCACertificateUnknownPlanBundleRejectedBeforeAPICreate(t *testing.T) {
	ctx := context.Background()
	r, plan := auditCAResource(t)
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		calls++
		fmt.Fprint(w, `{"success":true,"data":{"created":[{"id":1},{"id":2}]}}`)
	}))
	defer srv.Close()
	r.client = client.NewClient(srv.URL, "test", srv.Client())
	if d := plan.SetAttribute(ctx, path.Root("pem"), types.StringUnknown()); d.HasError() {
		t.Fatal(d)
	}
	validation := resource.ValidateConfigResponse{}
	r.ValidateConfig(ctx, resource.ValidateConfigRequest{Config: tfsdk.Config{Schema: plan.Schema, Raw: plan.Raw}}, &validation)
	if validation.Diagnostics.HasError() {
		t.Fatal(validation.Diagnostics)
	}
	if d := plan.SetAttribute(ctx, path.Root("pem"), types.StringValue(auditCertificateBlock+auditCertificateBlock)); d.HasError() {
		t.Fatal(d)
	}
	resp := resource.CreateResponse{State: tfsdk.State{Schema: plan.Schema}}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, &resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("apply accepted certificate bundle")
	}
	if calls != 0 {
		t.Fatalf("resolved bundle reached CA API %d times", calls)
	}
}
func TestCACertificateUnexpectedMultipleCreationRollsBackEveryID(t *testing.T) {
	ctx := context.Background()
	r, plan := auditCAResource(t)
	deleted := map[string]bool{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method == "DELETE" {
			deleted[req.URL.Path] = true
			fmt.Fprint(w, `{"success":true,"data":null}`)
			return
		}
		fmt.Fprint(w, `{"success":true,"data":{"created":[{"id":11},{"id":12}]}}`)
	}))
	defer srv.Close()
	r.client = client.NewClient(srv.URL, "test", srv.Client())
	plan.SetAttribute(ctx, path.Root("pem"), types.StringValue(auditCertificateBlock))
	resp := resource.CreateResponse{State: tfsdk.State{Schema: plan.Schema}}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, &resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("unexpected multi-create succeeded")
	}
	if !deleted["/api/v1/ca-certificates/11"] || !deleted["/api/v1/ca-certificates/12"] {
		t.Fatalf("unmanaged trusted certificates remain: deleted %v", deleted)
	}
	if strings.Contains(resp.Diagnostics.Errors()[0].Detail(), "already exists") {
		t.Fatal("multiple creation incorrectly reported as duplicate")
	}
}

func TestCACertificateApplyUpdateRejectsBundleBeforeAPI(t *testing.T) {
	ctx := context.Background()
	r, plan := auditCAResource(t)
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		calls++
		fmt.Fprint(w, `{"success":true,"data":{"id":7}}`)
	}))
	defer srv.Close()
	r.client = client.NewClient(srv.URL, "test", srv.Client())
	plan.SetAttribute(ctx, path.Root("pem"), types.StringValue(auditCertificateBlock))
	plan.SetAttribute(ctx, path.Root("id"), types.StringValue("7"))
	state := tfsdk.State{Schema: plan.Schema, Raw: plan.Raw}
	plan.SetAttribute(ctx, path.Root("pem"), types.StringValue(auditCertificateBlock+auditCertificateBlock))
	resp := resource.UpdateResponse{State: state}
	r.Update(ctx, resource.UpdateRequest{State: state, Plan: plan}, &resp)
	if !resp.Diagnostics.HasError() || calls != 0 {
		t.Fatalf("bundle update was not rejected before API: calls=%d diagnostics=%v", calls, resp.Diagnostics)
	}
}
