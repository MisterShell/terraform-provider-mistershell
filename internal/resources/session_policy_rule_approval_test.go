package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-mistershell/internal/client"
)

func testRuleSchema(t *testing.T) resource.SchemaResponse {
	t.Helper()
	var resp resource.SchemaResponse
	(&SessionPolicyRuleResource{}).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	return resp
}

func testRuleApproval(roleIDs ...int64) types.Object {
	return ruleApprovalToObject(&client.RuleApproval{
		ApproverRoleIDs: roleIDs, ExpirationSeconds: 86400, ValiditySeconds: 3600,
	})
}

// testRuleModel is a fully-known accept rule; callers override fields.
func testRuleModel() SessionPolicyRuleResourceModel {
	emptyStrings := types.SetValueMust(types.StringType, []attr.Value{})
	emptyIDs := types.SetValueMust(types.Int64Type, []attr.Value{})
	return SessionPolicyRuleResourceModel{
		ID: types.Int64Value(8), Position: types.Int64Value(10), Name: types.StringValue("gate"),
		Comment: types.StringNull(), ResourceTypes: emptyStrings, SessionTypes: emptyStrings,
		LocationIDs: emptyIDs, TagIDs: emptyIDs, RoleIDs: emptyIDs, CommandAclIDs: emptyIDs,
		Action: types.StringValue("accept"), Notify: types.BoolValue(false), Log: types.BoolValue(false),
		Enabled: types.BoolValue(true), Approval: types.ObjectNull(ruleApprovalAttrTypes),
		HitCount: types.Int64Value(0), LastHitAt: types.StringNull(),
		CreatedAt: types.StringValue("2026-09-24T10:00:00Z"), UpdatedAt: types.StringValue("2026-09-24T10:00:00Z"),
	}
}

func TestSessionPolicyRuleValidateConfigApproval(t *testing.T) {
	ctx := context.Background()
	schema := testRuleSchema(t)
	for _, tc := range []struct {
		name    string
		edit    func(*SessionPolicyRuleResourceModel)
		wantErr string
	}{
		{"approve with approval", func(m *SessionPolicyRuleResourceModel) {
			m.Action, m.Approval = types.StringValue("approve"), testRuleApproval(3)
		}, ""},
		{"accept without approval", func(*SessionPolicyRuleResourceModel) {}, ""},
		{"approve without approval", func(m *SessionPolicyRuleResourceModel) {
			m.Action = types.StringValue("approve")
		}, "Missing approval configuration"},
		{"deny with approval", func(m *SessionPolicyRuleResourceModel) {
			m.Action, m.Approval = types.StringValue("deny"), testRuleApproval(3)
		}, "Unexpected approval configuration"},
		{"default action with approval", func(m *SessionPolicyRuleResourceModel) {
			m.Action, m.Approval = types.StringNull(), testRuleApproval(3)
		}, "Unexpected approval configuration"},
		{"approve with command ACLs", func(m *SessionPolicyRuleResourceModel) {
			m.Action, m.Approval = types.StringValue("approve"), testRuleApproval(3)
			m.CommandAclIDs = types.SetValueMust(types.Int64Type, []attr.Value{types.Int64Value(4)})
		}, "Command ACLs not allowed on approve rules"},
		{"unknown action is deferred", func(m *SessionPolicyRuleResourceModel) {
			m.Action, m.Approval = types.StringUnknown(), testRuleApproval(3)
		}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model := testRuleModel()
			tc.edit(&model)
			state := tfsdk.State{Schema: schema.Schema}
			if diags := state.Set(ctx, &model); diags.HasError() {
				t.Fatal(diags)
			}
			var resp resource.ValidateConfigResponse
			(&SessionPolicyRuleResource{}).ValidateConfig(ctx, resource.ValidateConfigRequest{
				Config: tfsdk.Config{Schema: schema.Schema, Raw: state.Raw},
			}, &resp)
			got := ""
			for _, d := range resp.Diagnostics.Errors() {
				got += d.Summary() + ";"
			}
			if (tc.wantErr == "") != (got == "") || !strings.Contains(got, tc.wantErr) {
				t.Fatalf("diagnostics = %q, want %q", got, tc.wantErr)
			}
		})
	}
}

// The backend merges a PATCH onto the stored rule, so leaving `approval` out
// when an approve rule becomes accept would keep the stored approval and fail
// validation. Create omits it for accept/deny; update always sends it.
func TestSessionPolicyRuleApprovalWireFormat(t *testing.T) {
	ctx := context.Background()
	var bodies []map[string]json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var body map[string]json.RawMessage
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		bodies = append(bodies, body)
		approval := string(body["approval"])
		if approval == "" {
			approval = "null"
		}
		fmt.Fprintf(w, `{"success":true,"data":{"id":8,"position":10,"name":"gate","comment":null,"resource_types":[],"session_types":[],"location_ids":[],"tag_ids":[],"role_ids":[],"command_acl_ids":[],"action":%s,"notify":false,"log":false,"enabled":true,"approval":%s,"hit_count":0,"last_hit_at":null,"created_at":"2026-09-24T10:00:00Z","updated_at":"2026-09-24T10:00:00Z"}}`, body["action"], approval)
	}))
	defer srv.Close()
	r := &SessionPolicyRuleResource{client: client.NewClient(srv.URL, "test", srv.Client())}
	schema := testRuleSchema(t)

	approveModel := testRuleModel()
	approveModel.Action = types.StringValue("approve")
	approveModel.Approval = ruleApprovalToObject(&client.RuleApproval{
		ApproverRoleIDs: []int64{3, 5}, ExpirationSeconds: 3600, ValiditySeconds: 1800,
		NotifyEmail: true, AllowSelfApproval: true,
	})
	plan := tfsdk.Plan{Schema: schema.Schema}
	if diags := plan.Set(ctx, &approveModel); diags.HasError() {
		t.Fatal(diags)
	}
	created := resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, &created)
	if created.Diagnostics.HasError() {
		t.Fatal(created.Diagnostics)
	}
	var approval client.RuleApproval
	if err := json.Unmarshal(bodies[0]["approval"], &approval); err != nil {
		t.Fatalf("create approval %s: %v", bodies[0]["approval"], err)
	}
	if len(approval.ApproverRoleIDs) != 2 || approval.ExpirationSeconds != 3600 || approval.ValiditySeconds != 1800 || !approval.NotifyEmail || !approval.AllowSelfApproval {
		t.Fatalf("create approval = %+v", approval)
	}
	var state SessionPolicyRuleResourceModel
	if diags := created.State.Get(ctx, &state); diags.HasError() {
		t.Fatal(diags)
	}
	if !state.Approval.Equal(approveModel.Approval) {
		t.Fatalf("created approval state = %v, want %v", state.Approval, approveModel.Approval)
	}

	acceptModel := testRuleModel()
	acceptPlan := tfsdk.Plan{Schema: schema.Schema}
	if diags := acceptPlan.Set(ctx, &acceptModel); diags.HasError() {
		t.Fatal(diags)
	}
	updated := resource.UpdateResponse{State: tfsdk.State{Schema: schema.Schema}}
	r.Update(ctx, resource.UpdateRequest{Plan: acceptPlan, State: created.State}, &updated)
	if updated.Diagnostics.HasError() {
		t.Fatal(updated.Diagnostics)
	}
	if raw, present := bodies[1]["approval"]; !present || string(raw) != "null" {
		t.Fatalf("update approval = %s (present=%t), want explicit null", raw, present)
	}
	if diags := updated.State.Get(ctx, &state); diags.HasError() {
		t.Fatal(diags)
	}
	if !state.Approval.IsNull() {
		t.Fatalf("approval after switching to accept = %v, want null", state.Approval)
	}

	r.Create(ctx, resource.CreateRequest{Plan: acceptPlan}, &resource.CreateResponse{State: tfsdk.State{Schema: schema.Schema}})
	if _, present := bodies[2]["approval"]; present {
		t.Fatalf("accept create sent approval %s; it must be omitted", bodies[2]["approval"])
	}
}
