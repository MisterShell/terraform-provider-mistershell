package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"terraform-provider-mistershell/internal/client"
)

func TestAuditTagUnknownMembershipDoesNotClear(t *testing.T) {
	ctx := context.Background()
	writes := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PUT" {
			writes++
		}
		if r.URL.Path == "/api/v1/tags/9/resources" {
			fmt.Fprint(w, `{"success":true,"data":[7]}`)
			return
		}
		if r.URL.Path == "/api/v1/tags/9/resource-ids" {
			fmt.Fprint(w, `{"success":true,"data":[7]}`)
			return
		}
		fmt.Fprint(w, `{"success":true,"data":{"id":9,"name":"new","color":"grey"}}`)
	}))
	defer srv.Close()
	r := &TagResource{client: client.NewClient(srv.URL, "test", srv.Client())}
	var s resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &s)
	m := TagResourceModel{ID: types.Int64Value(9), Name: types.StringValue("old"), Color: types.StringValue("grey"), ResourceIDs: int64SliceToSet([]int64{7})}
	state := tfsdk.State{Schema: s.Schema}
	if d := state.Set(ctx, &m); d.HasError() {
		t.Fatal(d)
	}
	m.Name = types.StringValue("new")
	m.ResourceIDs = types.SetUnknown(types.Int64Type)
	plan := tfsdk.Plan{Schema: s.Schema}
	if d := plan.Set(ctx, &m); d.HasError() {
		t.Fatal(d)
	}
	resp := resource.UpdateResponse{State: state}
	r.Update(ctx, resource.UpdateRequest{State: state, Plan: plan}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if d := resp.State.Get(ctx, &m); d.HasError() {
		t.Fatal(d)
	}
	if !m.ResourceIDs.Equal(int64SliceToSet([]int64{7})) {
		t.Fatal("readback lost unmanaged assignment")
	}
	if writes != 0 {
		t.Fatalf("metadata update made %d membership writes", writes)
	}
}

func TestAuditLocationOrgCoordinatesRejected(t *testing.T) {
	ctx := context.Background()
	r := &LocationResource{}
	v, ok := any(r).(resource.ResourceWithValidateConfig)
	if !ok {
		t.Fatal("location does not validate coordinate combinations")
	}
	var s resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &s)
	config := tfsdk.Config{Schema: s.Schema}
	state := tfsdk.State{Schema: s.Schema}
	m := LocationResourceModel{Name: types.StringValue("org"), Kind: types.StringValue("org"), ParentID: types.Int64Value(1), Latitude: types.Float64Value(1)}
	if d := state.Set(ctx, &m); d.HasError() {
		t.Fatal(d)
	}
	config.Raw = state.Raw
	var resp resource.ValidateConfigResponse
	v.ValidateConfig(ctx, resource.ValidateConfigRequest{Config: config}, &resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("organizational coordinates accepted")
	}
}

func TestAuditWorkerRefreshAndNullClear(t *testing.T) {
	ctx := context.Background()
	var patch map[string]json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PATCH" {
			json.NewDecoder(r.Body).Decode(&patch)
		}
		fmt.Fprint(w, `{"success":true,"data":{"id":9,"name":"worker","location_id":1,"is_enabled":true,"config":{"changed":true},"config_schema":{"type":"object"}}}`)
	}))
	defer srv.Close()
	r := &WorkerResource{client: client.NewClient(srv.URL, "test", srv.Client())}
	var s resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &s)
	m := WorkerResourceModel{ID: types.Int64Value(9)}
	state := tfsdk.State{Schema: s.Schema}
	if d := state.Set(ctx, &m); d.HasError() {
		t.Fatal(d)
	}
	read := resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, &read)
	if read.Diagnostics.HasError() {
		t.Fatal(read.Diagnostics)
	}
	read.State.Get(ctx, &m)
	if m.Config.IsNull() || m.ConfigSchema.IsNull() {
		t.Error("import discarded readable worker configuration")
	}
	// Known prior configuration must be explicitly cleared with JSON null.
	m.Config, _ = rawJSONToDynamic(json.RawMessage(`{"changed":true}`))
	m.ConfigSchema, _ = rawJSONToDynamic(json.RawMessage(`{"type":"object"}`))
	state.Set(ctx, &m)
	m.Config = types.DynamicNull()
	m.ConfigSchema = types.DynamicNull()
	plan := tfsdk.Plan{Schema: s.Schema}
	plan.Set(ctx, &m)
	update := resource.UpdateResponse{State: state}
	r.Update(ctx, resource.UpdateRequest{State: state, Plan: plan}, &update)
	if string(patch["config"]) != "null" || string(patch["config_schema"]) != "null" {
		t.Errorf("clearing config did not send null: %v", patch)
	}
	if _, ok := patch["name"]; ok {
		t.Error("unchanged identity included in PATCH")
	}
	if _, ok := patch["description"]; ok {
		t.Error("unchanged description included in PATCH")
	}
}

func TestAuditCredentialTypeUpdateDoesNotReplace(t *testing.T) {
	var s resource.SchemaResponse
	(&CredentialResource{}).Schema(context.Background(), resource.SchemaRequest{}, &s)
	a := s.Schema.Attributes["credential_type"].(schema.StringAttribute)
	if len(a.PlanModifiers) != 0 {
		t.Fatal("credential type change requires replacement")
	}
}

func TestAuditInventoryEvidenceAndHealthSchema(t *testing.T) {
	var s resource.SchemaResponse
	(&NetworkResourceResource{}).Schema(context.Background(), resource.SchemaRequest{}, &s)
	for _, k := range []string{"check_task_id", "accepted_trust_fields", "accepted_identity", "health_status", "last_health_at"} {
		if s.Schema.Attributes[k] == nil {
			t.Errorf("missing %s", k)
		}
	}
	a := s.Schema.Attributes["extra_data"].(schema.DynamicAttribute)
	if !a.Optional {
		t.Error("extra_data is not writable")
	}
}

func TestAuditCredentialRefreshReadableFieldsBothAudiences(t *testing.T) {
	ctx := context.Background()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/credentials/types" {
			fmt.Fprint(w, `{"success":true,"data":[{"type":"username_password","schema":{"properties":{"username":{"type":"string"},"password":{"type":"string","x-secret":true}}}}]}`)
			return
		}
		fmt.Fprint(w, `{"success":true,"data":{"id":7,"name":"test","credential_type":"username_password","source":"local","user_source":"external","credential_data":{"username":"remote","password":"****"},"user_credential_data":{"username":"remote-user","password":"****"}}}`)
	}))
	defer srv.Close()
	r := &CredentialResource{client: client.NewClient(srv.URL, "test", srv.Client())}
	var s resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &s)
	m := credentialTestModel(t)
	m.UserCredentialData = credentialTestDynamic(t, `{"username":"old-user","password":"user-secret"}`)
	state := tfsdk.State{Schema: s.Schema}
	if d := state.Set(ctx, &m); d.HasError() {
		t.Fatal(d)
	}
	resp := resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	resp.State.Get(ctx, &m)
	for _, tc := range []struct {
		value types.Dynamic
		want  string
	}{{m.CredentialData, `{"password":"secret","username":"remote"}`}, {m.UserCredentialData, `{"password":"user-secret","username":"remote-user"}`}} {
		raw, _ := dynamicToRawJSON(tc.value)
		if string(raw) != tc.want {
			t.Errorf("refresh got %s want %s", raw, tc.want)
		}
	}
}

func TestAuditNetworkResourceRefreshReadableConnector(t *testing.T) {
	m := NetworkResourceResourceModel{ConnectorData: credentialTestDynamic(t, `{"host":"old","port":22}`)}
	if err := mapNetworkResourceResponseToModel(&client.NetworkResourceResponse{ConnectorData: json.RawMessage(`{"host":"remote","port":22,"default":true}`)}, &m); err != nil {
		t.Fatal(err)
	}
	raw, _ := dynamicToRawJSON(m.ConnectorData)
	if string(raw) != `{"host":"remote","port":22}` {
		t.Fatalf("connector refresh = %s", raw)
	}
	m.ConnectorData = types.DynamicNull()
	if err := mapNetworkResourceResponseToModel(&client.NetworkResourceResponse{ConnectorData: json.RawMessage(`{"host":"imported"}`)}, &m); err != nil {
		t.Fatal(err)
	}
	if m.ConnectorData.IsNull() {
		t.Fatal("import discarded connector")
	}
}

func TestAuditDefaultWorkerConfigAndDestroy(t *testing.T) {
	ctx := context.Background()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		t.Errorf("unexpected default worker deletion: %s", r.Method)
	}))
	defer srv.Close()
	r := &WorkerResource{client: client.NewClient(srv.URL, "test", srv.Client())}
	var s resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &s)
	m := WorkerResourceModel{ID: types.Int64Value(1), Name: types.StringValue("default"), LocationID: types.Int64Value(1), IsDefault: types.BoolValue(true), IsEnabled: types.BoolValue(true)}
	state := tfsdk.State{Schema: s.Schema}
	state.Set(ctx, &m)
	var resp resource.DeleteResponse
	r.Delete(ctx, resource.DeleteRequest{State: state}, &resp)
	if resp.Diagnostics.HasError() || calls != 0 {
		t.Fatalf("default destroy mutated backend: %v", resp.Diagnostics)
	}
	plan := m
	plan.Config = credentialTestDynamic(t, `{"collection":false}`)
	input, err := buildWorkerUpdateInput(&plan, &m)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(input)
	if string(raw) != `{"config":{"collection":false}}` {
		t.Fatalf("default PATCH = %s", raw)
	}
	plan.Name = types.StringValue("rename")
	if _, err := buildWorkerUpdateInput(&plan, &m); err == nil {
		t.Fatal("default identity update allowed")
	}
}

func TestAuditEvidenceOnlyExplicitNewCheck(t *testing.T) {
	state := NetworkResourceResourceModel{AcceptedTrustFields: types.SetNull(types.StringType)}
	plan := state
	input, err := buildNetworkResourceUpdate(&plan, &state)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(input)
	if string(raw) != "{}" {
		t.Fatalf("implicit evidence = %s", raw)
	}
	plan.CheckTaskID = types.Int64Value(42)
	plan.AcceptedTrustFields = stringSliceToSet([]string{"host_key_fingerprint"})
	plan.AcceptedIdentity = types.BoolValue(true)
	input, err = buildNetworkResourceUpdate(&plan, &state)
	if err != nil {
		t.Fatal(err)
	}
	if input.CheckTaskID == nil || *input.CheckTaskID != 42 || len(input.AcceptedTrustFields) != 1 || input.AcceptedIdentity == nil || !*input.AcceptedIdentity {
		t.Fatalf("explicit evidence missing: %#v", input)
	}
	state = plan
	plan.Name = types.StringValue("renamed")
	input, err = buildNetworkResourceUpdate(&plan, &state)
	if err != nil {
		t.Fatal(err)
	}
	if input.CheckTaskID != nil || len(input.AcceptedTrustFields) != 0 || input.AcceptedIdentity != nil {
		t.Fatal("metadata change replayed evidence")
	}
}

func TestAuditEvidenceSubmitsExactDraftEvenUnchangedFields(t *testing.T) {
	state := NetworkResourceResourceModel{Name: types.StringValue("same"), ConnectorData: credentialTestDynamic(t, `{"host":"target","port":22}`), CredentialID: types.Int64Value(3), LocationID: types.Int64Value(4), AcceptedTrustFields: types.SetNull(types.StringType)}
	plan := state
	plan.CheckTaskID = types.Int64Value(99)
	input, err := buildNetworkResourceUpdate(&plan, &state)
	if err != nil {
		t.Fatal(err)
	}
	if input.Name == nil || input.ConnectorData == nil || input.CredentialID == nil || input.LocationID == nil {
		t.Fatalf("evidence omitted exact draft fields: %#v", input)
	}
}

func TestAuditEvidenceEnrichmentPreservesManagedMetadata(t *testing.T) {
	ctx := context.Background()
	patches := 0
	extra := json.RawMessage(`{"databases":["old"]}`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/api/v1/resources/9/tags" {
			fmt.Fprint(w, `{"success":true,"data":[]}`)
			return
		}
		if req.Method == "PATCH" {
			patches++
			var input map[string]json.RawMessage
			if err := json.NewDecoder(req.Body).Decode(&input); err != nil {
				t.Fatal(err)
			}
			if _, ok := input["check_task_id"]; ok {
				extra = json.RawMessage(`{"databases":["observed","second"]}`)
			} else if v, ok := input["extra_data"]; ok {
				extra = v
			}
		}
		fmt.Fprintf(w, `{"success":true,"data":{"id":9,"name":"db","resource_type":"database_postgres","location_id":1,"credential_id":2,"connector_data":{"host":"db"},"extra_data":%s,"external_id":"new-identity"}}`, extra)
	}))
	defer srv.Close()
	r := &NetworkResourceResource{client: client.NewClient(srv.URL, "test", srv.Client())}
	var schemaResponse resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResponse)
	model := NetworkResourceResourceModel{ID: types.Int64Value(9), Name: types.StringValue("db"), LocationID: types.Int64Value(1), CredentialID: types.Int64Value(2), ConnectorData: credentialTestDynamic(t, `{"host":"db"}`), ExtraData: credentialTestDynamic(t, `{"databases":["configured"]}`), AcceptedTrustFields: types.SetNull(types.StringType), TagIDs: types.SetNull(types.Int64Type), Tags: types.ListNull(tagObjectType)}
	state := tfsdk.State{Schema: schemaResponse.Schema}
	if d := state.Set(ctx, &model); d.HasError() {
		t.Fatal(d)
	}
	model.CheckTaskID = types.Int64Value(42)
	model.AcceptedIdentity = types.BoolValue(true)
	plan := tfsdk.Plan{Schema: schemaResponse.Schema}
	if d := plan.Set(ctx, &model); d.HasError() {
		t.Fatal(d)
	}
	resp := resource.UpdateResponse{State: state}
	r.Update(ctx, resource.UpdateRequest{State: state, Plan: plan}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if patches != 2 {
		t.Fatalf("evidence enrichment left managed metadata overwritten: %d PATCH requests", patches)
	}
	if string(extra) != `{"databases":["configured"]}` {
		t.Fatalf("server metadata differs from configuration: %s", extra)
	}
	var got NetworkResourceResourceModel
	if d := resp.State.Get(ctx, &got); d.HasError() {
		t.Fatal(d)
	}
	if !got.ExtraData.Equal(model.ExtraData) {
		t.Fatal("planned metadata type/value was not preserved")
	}
}
