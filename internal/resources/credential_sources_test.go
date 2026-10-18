package resources

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"net/http"
	"net/http/httptest"
	"terraform-provider-mistershell/internal/client"
	"testing"
)

func TestCredentialSourcesSchema(t *testing.T) {
	var resp resource.SchemaResponse
	NewCredentialResource().Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if diags := resp.Schema.ValidateImplementation(context.Background()); diags.HasError() {
		t.Fatal(diags)
	}
	literals := resp.Schema.Attributes["credential_data"].(schema.DynamicAttribute)
	if !literals.Optional || literals.Required {
		t.Error("all-mapped credentials must allow omitted literal data")
	}
	for _, name := range []string{"source", "user_source"} {
		a, ok := resp.Schema.Attributes[name].(schema.StringAttribute)
		if !ok || !a.Optional || !a.Computed || a.Default != nil {
			t.Errorf("%s must be optional+computed without a schema default", name)
		}
	}
	for _, name := range []string{"external_vault_ref", "user_external_vault_ref"} {
		a, ok := resp.Schema.Attributes[name].(schema.SingleNestedAttribute)
		if !ok || !a.Optional {
			t.Errorf("%s must be a typed optional reference", name)
		}
	}
}

func credentialTestModel(t *testing.T) CredentialResourceModel {
	t.Helper()
	return CredentialResourceModel{ID: types.Int64Value(7), Name: types.StringValue("test"), CredentialType: types.StringValue("username_password"), CredentialData: credentialTestDynamic(t, `{"username":"admin","password":"secret"}`), Source: types.StringValue("local"), UserSource: types.StringValue("local"), RequiresUserMapping: types.BoolValue(false), ExternalVaultRef: credentialTestRefNull(), UserExternalVaultRef: credentialTestRefNull()}
}
func credentialTestDynamic(t *testing.T, raw string) types.Dynamic {
	t.Helper()
	v, e := rawJSONToDynamic(json.RawMessage(raw))
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func credentialTestRefNull() types.Object {
	return types.ObjectNull(map[string]attr.Type{"base": types.DynamicType, "fields": types.MapType{ElemType: types.StringType}})
}
func credentialTestRef(t *testing.T) types.Object {
	t.Helper()
	return types.ObjectValueMust(map[string]attr.Type{"base": types.DynamicType, "fields": types.MapType{ElemType: types.StringType}}, map[string]attr.Value{"base": credentialTestDynamic(t, `{"path":"servers/{{ resource.name }}","version":2}`), "fields": types.MapValueMust(types.StringType, map[string]attr.Value{"username": types.StringValue("username"), "password": types.StringValue("password")})})
}

func TestCredentialUpdateSourcePayloads(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*CredentialResourceModel)
		check  func(*testing.T, map[string]json.RawMessage)
	}{
		{"metadata omits literals", func(m *CredentialResourceModel) { m.Name = types.StringValue("renamed") }, func(t *testing.T, b map[string]json.RawMessage) {
			if _, ok := b["credential_data"]; ok {
				t.Error("unchanged literals must be omitted")
			}
			if _, ok := b["description"]; ok {
				t.Error("unchanged description must be omitted")
			}
		}},
		{"platform all mapped", func(m *CredentialResourceModel) {
			m.Source = types.StringValue("external")
			m.ExternalVaultID = types.Int64Value(3)
			m.ExternalVaultRef = credentialTestRef(t)
			m.CredentialData = types.DynamicNull()
		}, func(t *testing.T, b map[string]json.RawMessage) {
			if string(b["credential_data"]) != "{}" || string(b["source"]) != `"external"` || string(b["external_vault_id"]) != "3" {
				t.Error("external transition must include source, ID and complete empty literals")
			}
			if !bytes.Contains(b["external_vault_ref"], []byte(`"version":2`)) || !bytes.Contains(b["external_vault_ref"], []byte(`{{ resource.name }}`)) {
				t.Error("reference integer and Jinja must be retained")
			}
		}},
		{"user independent", func(m *CredentialResourceModel) {
			m.UserSource = types.StringValue("external")
			m.UserExternalVaultID = types.Int64Value(3)
			m.UserExternalVaultRef = credentialTestRef(t)
		}, func(t *testing.T, b map[string]json.RawMessage) {
			if string(b["user_source"]) != `"external"` || string(b["user_credential_data"]) != "{}" {
				t.Error("user transition missing complete payload")
			}
			if _, ok := b["credential_data"]; ok {
				t.Error("user change must not resend platform literals")
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			state := credentialTestModel(t)
			plan := state
			tc.change(&plan)
			called := false
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				var body map[string]json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				tc.check(t, body)
				reply := map[string]any{"id": 7, "name": plan.Name.ValueString(), "credential_type": "username_password", "source": plan.Source.ValueString(), "user_source": plan.UserSource.ValueString(), "requires_user_mapping": false, "credential_data": map[string]any{}, "created_at": "today", "updated_at": "today"}
				for _, k := range []string{"external_vault_id", "external_vault_ref", "user_external_vault_id", "user_external_vault_ref"} {
					if v, ok := body[k]; ok {
						reply[k] = v
					}
				}
				json.NewEncoder(w).Encode(map[string]any{"success": true, "data": reply})
			}))
			defer srv.Close()
			r := &CredentialResource{client: client.NewClient(srv.URL, "test", srv.Client())}
			var schemaResp resource.SchemaResponse
			r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
			p := tfsdk.Plan{Schema: schemaResp.Schema}
			s := tfsdk.State{Schema: schemaResp.Schema}
			if d := p.Set(ctx, &plan); d.HasError() {
				t.Fatal(d)
			}
			if d := s.Set(ctx, &state); d.HasError() {
				t.Fatal(d)
			}
			resp := resource.UpdateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
			r.Update(ctx, resource.UpdateRequest{Plan: p, State: s}, &resp)
			if !called {
				t.Fatal("PATCH not called")
			}
			if resp.Diagnostics.HasError() {
				t.Fatal(resp.Diagnostics)
			}
			var got CredentialResourceModel
			if d := resp.State.Get(ctx, &got); d.HasError() {
				t.Fatal(d)
			}
			if !got.CredentialData.Equal(plan.CredentialData) || !got.UserCredentialData.Equal(plan.UserCredentialData) {
				t.Error("planned literal null/empty and secrets must be preserved")
			}
		})
	}
}

func TestCredentialRefreshSourcesAndImport(t *testing.T) {
	var cred client.CredentialResponse
	if err := json.Unmarshal([]byte(`{"id":7,"name":"test","credential_type":"username_password","source":"external","user_source":"external","external_vault_id":3,"user_external_vault_id":4,"external_vault_name":"platform","external_vault_type":"hashicorp","user_external_vault_name":"people","user_external_vault_type":"azure_key_vault","external_vault_ref":{"base":{"version":2,"path":"servers/{{ resource.name }}"},"fields":{"username":"username","password":"password"}},"user_external_vault_ref":{"base":{},"fields":{"username":"{{ user.email }}","password":"password"}},"credential_data":{},"user_credential_data":{"password":"****"}}`), &cred); err != nil {
		t.Fatal(err)
	}
	for _, imported := range []bool{false, true} {
		m := credentialTestModel(t)
		if imported {
			m = CredentialResourceModel{}
		}
		original := m.CredentialData
		if err := mapCredentialResponseToModel(&cred, &m); err != nil {
			t.Fatal(err)
		}
		if m.Source.ValueString() != "external" || m.UserSource.ValueString() != "external" || m.ExternalVaultID.ValueInt64() != 3 || m.UserExternalVaultID.ValueInt64() != 4 || m.UserExternalVaultType.ValueString() != "azure_key_vault" {
			t.Error("refresh/import must retain both source selections and metadata")
		}
		if m.ExternalVaultRef.IsNull() || m.UserExternalVaultRef.IsNull() {
			t.Error("refresh must populate both references")
		}
		if !m.CredentialData.Equal(original) {
			t.Error("refresh must preserve configured literals")
		}
	}
}

func TestCredentialCreateSources(t *testing.T) {
	for _, platform := range []string{"local", "external"} {
		for _, user := range []string{"local", "external"} {
			t.Run(platform+"/"+user, func(t *testing.T) {
				m := credentialTestModel(t)
				m.Source = types.StringValue(platform)
				m.UserSource = types.StringValue(user)
				if platform == "external" {
					m.CredentialData = types.DynamicNull()
					m.ExternalVaultID = types.Int64Value(3)
					m.ExternalVaultRef = credentialTestRef(t)
				}
				if user == "external" {
					m.UserExternalVaultID = types.Int64Value(4)
					m.UserExternalVaultRef = credentialTestRef(t)
				}
				input, err := credentialCreateInput(&m)
				if err != nil {
					t.Fatal(err)
				}
				raw, err := json.Marshal(input)
				if err != nil {
					t.Fatal(err)
				}
				var body map[string]json.RawMessage
				json.Unmarshal(raw, &body)
				for _, a := range []struct{ prefix, source string }{{"", platform}, {"user_", user}} {
					if a.source == "local" {
						if _, ok := body[a.prefix+"source"]; ok {
							t.Error("default local sources must be omitted for legacy backends")
						}
					} else {
						if string(body[a.prefix+"source"]) != `"external"` || string(body[a.prefix+"credential_data"]) != "{}" || len(body[a.prefix+"external_vault_ref"]) == 0 {
							t.Error("external creation must include reference and empty complete literal payload")
						}
					}
				}
			})
		}
	}
}

func TestCredentialSourceValidation(t *testing.T) {
	for _, tc := range []struct {
		name      string
		change    func(*CredentialResourceModel)
		wantError bool
	}{
		{"missing local literals", func(m *CredentialResourceModel) { m.CredentialData = types.DynamicNull() }, true},
		{"local user literals", func(m *CredentialResourceModel) { m.UserCredentialData = credentialTestDynamic(t, `{}`) }, true},
		{"invalid source", func(m *CredentialResourceModel) { m.Source = types.StringValue("typo") }, true},
		{"external missing ref", func(m *CredentialResourceModel) {
			m.Source = types.StringValue("external")
			m.ExternalVaultID = types.Int64Value(3)
		}, true},
		{"unknown reference", func(m *CredentialResourceModel) {
			m.Source = types.StringValue("external")
			m.ExternalVaultID = types.Int64Unknown()
			m.ExternalVaultRef = types.ObjectUnknown(credentialTestRefNull().AttributeTypes(context.Background()))
			m.CredentialData = types.DynamicUnknown()
		}, false},
		{"unknown source", func(m *CredentialResourceModel) {
			m.Source = types.StringUnknown()
			m.CredentialData = types.DynamicNull()
		}, false},
		{"literal overlap", func(m *CredentialResourceModel) {
			m.Source = types.StringValue("external")
			m.ExternalVaultID = types.Int64Value(3)
			m.ExternalVaultRef = credentialTestRef(t)
		}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := credentialTestModel(t)
			tc.change(&m)
			d := validateCredentialSources(m)
			if d.HasError() != tc.wantError {
				t.Fatalf("validation error=%v, want=%v: %v", d.HasError(), tc.wantError, d)
			}
		})
	}
}

func TestCredentialReferenceBaseRejectsNull(t *testing.T) {
	m := credentialTestModel(t)
	m.Source = types.StringValue("external")
	m.ExternalVaultID = types.Int64Value(3)
	m.CredentialData = types.DynamicNull()
	ref := credentialTestRef(t)
	attrs := ref.Attributes()
	attrs["base"] = credentialTestDynamic(t, `{"path":null}`)
	m.ExternalVaultRef = types.ObjectValueMust(ref.AttributeTypes(context.Background()), attrs)
	if !validateCredentialSources(m).HasError() {
		t.Fatal("a null base parameter is not a string or integer")
	}
}

func TestCredentialTransitionAndLiteralRemoval(t *testing.T) {
	for _, user := range []bool{false, true} {
		t.Run(fmt.Sprint("user=", user), func(t *testing.T) {
			state := credentialTestModel(t)
			if user {
				state.UserSource = types.StringValue("external")
				state.UserExternalVaultID = types.Int64Value(3)
				state.UserExternalVaultRef = credentialTestRef(t)
				state.UserCredentialData = credentialTestDynamic(t, `{"username":"literal"}`)
			} else {
				state.Source = types.StringValue("external")
				state.ExternalVaultID = types.Int64Value(3)
				state.ExternalVaultRef = credentialTestRef(t)
				state.CredentialData = credentialTestDynamic(t, `{"username":"literal"}`)
			}
			plan := state
			prefix := ""
			if user {
				prefix = "user_"
				plan.UserCredentialData = types.DynamicNull()
			} else {
				plan.CredentialData = types.DynamicNull()
			}
			input, err := credentialUpdateInput(&plan, state)
			if err != nil {
				t.Fatal(err)
			}
			raw, _ := json.Marshal(input)
			var body map[string]json.RawMessage
			json.Unmarshal(raw, &body)
			if string(body[prefix+"credential_data"]) != "{}" {
				t.Fatal("removing final literal must replace with an empty object")
			}
			plan = state
			if user {
				plan.UserSource = types.StringValue("local")
				plan.UserExternalVaultID = types.Int64Null()
				plan.UserExternalVaultRef = credentialTestRefNull()
				plan.UserCredentialData = types.DynamicNull()
			} else {
				plan.Source = types.StringValue("local")
				plan.ExternalVaultID = types.Int64Null()
				plan.ExternalVaultRef = credentialTestRefNull()
				plan.CredentialData = credentialTestDynamic(t, `{"username":"admin","password":"new"}`)
			}
			input, err = credentialUpdateInput(&plan, state)
			if err != nil {
				t.Fatal(err)
			}
			raw, _ = json.Marshal(input)
			json.Unmarshal(raw, &body)
			if string(body[prefix+"source"]) != `"local"` || string(body[prefix+"external_vault_id"]) != "null" || string(body[prefix+"external_vault_ref"]) != "null" {
				t.Fatal("returning local must explicitly clear both reference fields")
			}
			if user && string(body["user_credential_data"]) != "null" {
				t.Fatal("local user source must clear its literal payload")
			}
		})
	}
}

func TestCredentialTransitionRejectsMasksAndPreservesImportedSource(t *testing.T) {
	state := credentialTestModel(t)
	state.Source = types.StringValue("external")
	state.ExternalVaultID = types.Int64Value(3)
	state.ExternalVaultRef = credentialTestRef(t)
	state.CredentialData = types.DynamicNull()
	plan := state
	plan.Source = types.StringUnknown()
	plan.Name = types.StringValue("renamed")
	input, err := credentialUpdateInput(&plan, state)
	if err != nil {
		t.Fatal(err)
	}
	if input.Source != "" || plan.Source.ValueString() != "external" {
		t.Fatal("an unknown computed source must inherit imported state")
	}
	plan = state
	plan.Source = types.StringValue("local")
	plan.ExternalVaultID = types.Int64Null()
	plan.ExternalVaultRef = credentialTestRefNull()
	plan.CredentialData = credentialTestDynamic(t, `{"username":"admin","password":"****"}`)
	if _, err := credentialUpdateInput(&plan, state); err == nil {
		t.Fatal("source transition must reject masks")
	}
	state = credentialTestModel(t)
	plan = state
	plan.CredentialType = types.StringValue("other")
	plan.CredentialData = credentialTestDynamic(t, `{"password":"****"}`)
	if _, err := credentialUpdateInput(&plan, state); err == nil {
		t.Fatal("type transition must reject masks")
	}
}
