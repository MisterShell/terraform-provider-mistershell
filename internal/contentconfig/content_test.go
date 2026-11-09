package contentconfig

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"terraform-provider-mistershell/internal/client"
	"testing"
	"time"
)

var ctx = context.Background()

func stateFor(t *testing.T, r *Resource, values map[string]attr.Value) tfsdk.State {
	t.Helper()
	var schema resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schema)
	v := map[string]tftypes.Value{}
	for k, a := range schema.Schema.Attributes {
		v[k] = tftypes.NewValue(a.GetType().TerraformType(ctx), nil)
	}
	for k, a := range values {
		var e error
		v[k], e = a.ToTerraformValue(ctx)
		if e != nil {
			t.Fatal(e)
		}
	}
	return tfsdk.State{Schema: schema.Schema, Raw: tftypes.NewValue(schema.Schema.Type().TerraformType(ctx), v)}
}
func planFor(s tfsdk.State) tfsdk.Plan { return tfsdk.Plan{Schema: s.Schema, Raw: s.Raw} }
func api(t *testing.T, handler http.HandlerFunc) *client.Client {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fixture" {
			t.Error("missing shared authentication")
		}
		w.Header().Set("Content-Type", "application/json")
		handler(w, r)
	}))
	t.Cleanup(s.Close)
	return client.NewClient(s.URL, "fixture", s.Client())
}
func respond(w http.ResponseWriter, v any) {
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": v})
}
func fail(w http.ResponseWriter, code int, message string) {
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "message": message})
}
func licenseState(t *testing.T, r *Resource) tfsdk.State {
	return stateFor(t, r, map[string]attr.Value{"id": types.StringValue("lic-1"), "license_key": types.StringValue("sensitive-license-key"), "confirm_over_capacity": types.BoolValue(false), "confirm_removal_over_capacity": types.BoolValue(false)})
}
func noteState(t *testing.T, r *Resource) tfsdk.State {
	return stateFor(t, r, map[string]attr.Value{"id": types.StringValue("location:2"), "scope_type": types.StringValue("location"), "scope_id": types.Int64Value(2), "content": types.StringValue("managed note"), "version": types.Int64Value(1)})
}
func fileState(t *testing.T, r *Resource) tfsdk.State {
	return stateFor(t, r, map[string]attr.Value{"id": types.StringValue("7"), "location_id": types.Int64Value(2), "name": types.StringValue("guide.txt"), "parent_path": types.StringValue("")})
}
func TestSchemaContracts(t *testing.T) {
	for _, constructor := range Resources() {
		r := constructor()
		var p resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &p)
		if d := p.Schema.ValidateImplementation(ctx); d.HasError() {
			t.Fatal(d)
		}
	}
	for _, constructor := range DataSources() {
		d := constructor()
		var p datasource.SchemaResponse
		d.Schema(ctx, datasource.SchemaRequest{}, &p)
		if e := p.Schema.ValidateImplementation(ctx); e.HasError() {
			t.Fatal(e)
		}
	}
}
func TestLicenseCreateRequiresExplicitOwnershipAndConfirmation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		preview any
		want    string
	}{{"adoption", map[string]any{"already_installed": true}, "import"}, {"capacity", map[string]any{"requires_confirmation": true}, "confirm_over_capacity"}} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			r := &Resource{kind: "license", client: api(t, func(w http.ResponseWriter, q *http.Request) {
				calls++
				if q.URL.Path != "/api/v1/licenses/preview" {
					t.Error("unexpected mutation")
				}
				respond(w, tc.preview)
			})}
			s := licenseState(t, r)
			var p resource.CreateResponse
			r.Create(ctx, resource.CreateRequest{Plan: planFor(s)}, &p)
			if !p.Diagnostics.HasError() || !strings.Contains(fmt.Sprint(p.Diagnostics), tc.want) || calls != 1 {
				t.Fatalf("missing guard: %v, calls %d", p.Diagnostics, calls)
			}
		})
	}
}
func TestLicensePendingInstallPersistsIdentityAndMetadata(t *testing.T) {
	r := &Resource{kind: "license", client: api(t, func(w http.ResponseWriter, q *http.Request) {
		switch {
		case strings.HasSuffix(q.URL.Path, "preview"):
			respond(w, map[string]any{"already_installed": false})
		case q.Method == "POST":
			w.WriteHeader(202)
			respond(w, map[string]any{"license_id": "lic-1", "state": "pending"})
		case q.Method == "GET":
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": []any{map[string]any{"license_id": "lic-1", "feature": "business"}}, "meta": map[string]any{"page": 1, "size": 100, "total": 1, "pages": 1}})
		}
	})}
	var p resource.CreateResponse
	r.Create(ctx, resource.CreateRequest{Plan: planFor(licenseState(t, r))}, &p)
	if p.Diagnostics.HasError() || stringValue(ctx, p.State, "id") != "lic-1" {
		t.Fatalf("pending mutation lost: %v", p.Diagnostics)
	}
}
func TestLicenseErrorsNeverEchoSignedKey(t *testing.T) {
	r := &Resource{kind: "license", client: api(t, func(w http.ResponseWriter, q *http.Request) { fail(w, 422, "invalid sensitive-license-key") })}
	var p resource.CreateResponse
	r.Create(ctx, resource.CreateRequest{Plan: planFor(licenseState(t, r))}, &p)
	if !p.Diagnostics.HasError() || strings.Contains(fmt.Sprint(p.Diagnostics), "sensitive-license-key") {
		t.Fatalf("unsafe diagnostic %v", p.Diagnostics)
	}
}
func TestLicenseDeleteConfirmation(t *testing.T) {
	for _, confirm := range []bool{false, true} {
		t.Run(strconvBool(confirm), func(t *testing.T) {
			deleted := false
			r := &Resource{kind: "license", client: api(t, func(w http.ResponseWriter, q *http.Request) {
				if q.Method == "POST" {
					respond(w, map[string]any{"requires_confirmation": true})
					return
				}
				deleted = true
				if q.URL.Query().Get("confirm_over_capacity") != "true" {
					t.Error("missing explicit confirmation")
				}
				w.WriteHeader(202)
				respond(w, map[string]any{"license_id": "lic-1", "state": "pending"})
			})}
			s := licenseState(t, r)
			s.SetAttribute(ctx, path.Root("confirm_removal_over_capacity"), confirm)
			var p resource.DeleteResponse
			r.Delete(ctx, resource.DeleteRequest{State: s}, &p)
			if deleted != confirm || p.Diagnostics.HasError() == confirm {
				t.Fatalf("confirmation failed: %v", p.Diagnostics)
			}
		})
	}
}
func strconvBool(v bool) string {
	if v {
		return "confirmed"
	}
	return "unconfirmed"
}
func TestNoteLockAndVersionBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name     string
		locked   bool
		versions []int
		lockCode int
		wantPut  bool
	}{{"success", false, []int{1, 1}, 0, true}, {"existing same-user lock", true, []int{1}, 0, false}, {"stale plan", false, []int{2}, 0, false}, {"changed after acquisition", false, []int{1, 2}, 0, false}, {"other user raced", false, []int{1}, 409, false}} {
		t.Run(tc.name, func(t *testing.T) {
			gets, acquires, releases, puts := 0, 0, 0, 0
			r := &Resource{kind: "note", client: api(t, func(w http.ResponseWriter, q *http.Request) {
				switch q.Method {
				case "GET":
					version := tc.versions[gets]
					gets++
					var editor any
					if tc.locked {
						editor = map[string]any{"user_id": 1}
					}
					respond(w, map[string]any{"latest_version": map[string]any{"version": version, "content": "old"}, "current_editor": editor})
				case "POST":
					acquires++
					if tc.lockCode > 0 {
						fail(w, tc.lockCode, "locked")
					} else {
						respond(w, map[string]any{})
					}
				case "DELETE":
					releases++
					respond(w, nil)
				case "PUT":
					puts++
					var b map[string]any
					json.NewDecoder(q.Body).Decode(&b)
					if b["content"] != "next" || b["content_type"] != "text/markdown" {
						t.Error("bad note body")
					}
					respond(w, map[string]any{"version": 2, "content": "next"})
				}
			})}
			b, e := r.writeNote(ctx, "/api/v1/notes/location/2", 1, "next")
			if (e == nil) != tc.wantPut || (puts == 1) != tc.wantPut {
				t.Fatalf("write guard: puts=%d err=%v body=%s", puts, e, b)
			}
			if tc.locked && acquires != 0 {
				t.Fatal("stole a same-user editor lock")
			}
			if acquires > 0 && tc.lockCode == 0 && releases != 1 {
				t.Fatal("lock not released")
			}
			if tc.lockCode > 0 && releases != 0 {
				t.Fatal("released someone else's lock")
			}
		})
	}
}
func TestNoteDestroyPreservesExistingHistory(t *testing.T) {
	r := &Resource{kind: "note", client: api(t, func(w http.ResponseWriter, q *http.Request) { t.Error("destroy must not mutate/read notes") })}
	var p resource.DeleteResponse
	r.Delete(ctx, resource.DeleteRequest{State: noteState(t, r)}, &p)
	if p.Diagnostics.HasError() {
		t.Fatal(p.Diagnostics)
	}
}
func TestNoteUpdatePersistsNewVersion(t *testing.T) {
	gets := 0
	r := &Resource{kind: "note", client: api(t, func(w http.ResponseWriter, q *http.Request) {
		switch q.Method {
		case "GET":
			gets++
			respond(w, map[string]any{"latest_version": map[string]any{"version": 1, "content": "old"}})
		case "PUT":
			respond(w, map[string]any{"version": 2, "content": "managed note"})
		default:
			respond(w, nil)
		}
	})}
	s := noteState(t, r)
	var p resource.UpdateResponse
	r.Update(ctx, resource.UpdateRequest{State: s, Plan: planFor(s)}, &p)
	if p.Diagnostics.HasError() || intValue(ctx, p.State, "version") != 2 || gets != 2 {
		t.Fatalf("update failed: %v", p.Diagnostics)
	}
}
func TestCatalogPaginationAndNestedIdentity(t *testing.T) {
	calls := 0
	c := api(t, func(w http.ResponseWriter, q *http.Request) {
		calls++
		nodes := []any{}
		switch {
		case q.URL.Query().Get("path") == "nested":
			nodes = append(nodes, map[string]any{"id": 7, "kind": "file", "path": "nested/file"})
		case q.URL.Query().Get("page") == "1":
			for i := 0; i < 99; i++ {
				nodes = append(nodes, map[string]any{"node_type": "location", "location_id": 100 + i})
			}
			nodes = append(nodes, map[string]any{"id": 8, "kind": "folder", "path": "nested"})
		}
		respond(w, map[string]any{"nodes": nodes})
	})
	b, e := catalogEntry(ctx, c, 2, "7")
	if e != nil || !strings.Contains(string(b), "nested/file") || calls != 3 {
		t.Fatalf("bad pagination/traversal: %s %v %d", b, e, calls)
	}
}
func TestFileUploadRawBodyAndMetadata(t *testing.T) {
	content := []byte{0, 1, 2, 255}
	sum := sha256.Sum256(content)
	checksum := hex.EncodeToString(sum[:])
	r := &Resource{kind: "file_catalog_file", client: api(t, func(w http.ResponseWriter, q *http.Request) {
		body, _ := io.ReadAll(q.Body)
		if q.Method != "POST" || q.URL.Path != "/api/v1/files/catalog/upload" || q.Header.Get("Content-Type") != "application/octet-stream" || string(body) != string(content) {
			t.Error("upload contract mismatch")
		}
		if q.URL.Query().Get("name") != "guide.txt" || q.URL.Query().Get("location_id") != "2" {
			t.Error("missing metadata query")
		}
		respond(w, map[string]any{"id": 7, "name": "guide.txt", "path": "guide.txt", "kind": "file", "sha256": checksum, "size": 4})
	})}
	s := fileState(t, r)
	s.SetAttribute(ctx, path.Root("content_base64"), base64.StdEncoding.EncodeToString(content))
	var p resource.CreateResponse
	r.Create(ctx, resource.CreateRequest{Plan: planFor(s)}, &p)
	if p.Diagnostics.HasError() || stringValue(ctx, p.State, "sha256") != checksum || stringValue(ctx, p.State, "id") != "7" {
		t.Fatalf("upload failed %v", p.Diagnostics)
	}
}
func TestCatalogMissingOrOutOfScopePreservesState(t *testing.T) {
	for _, kind := range []string{"file_catalog_file", "file_catalog_folder", "tree_element"} {
		t.Run(kind, func(t *testing.T) {
			r := &Resource{kind: kind, client: api(t, func(w http.ResponseWriter, q *http.Request) {
				if kind == "tree_element" {
					respond(w, []any{})
				} else {
					respond(w, map[string]any{"nodes": []any{}})
				}
			})}
			s := fileState(t, &Resource{kind: "file_catalog_folder"})
			if kind == "tree_element" {
				s = stateFor(t, r, map[string]attr.Value{"id": types.StringValue("7")})
			}
			var p resource.ReadResponse
			r.Read(ctx, resource.ReadRequest{State: s}, &p)
			if !p.Diagnostics.HasError() || p.State.Raw.IsNull() {
				t.Fatal("mistook masked absence for deletion")
			}
		})
	}
}
func TestFolderDestroyRefusesRecursiveDeletion(t *testing.T) {
	deletes := 0
	r := &Resource{kind: "file_catalog_folder", client: api(t, func(w http.ResponseWriter, q *http.Request) {
		if q.Method == "DELETE" {
			deletes++
		}
		nodes := []any{map[string]any{"id": 7, "kind": "folder", "path": "folder"}}
		if q.URL.Query().Get("path") == "folder" {
			nodes = []any{map[string]any{"id": 8, "kind": "file", "path": "folder/unowned.txt"}}
		}
		respond(w, map[string]any{"nodes": nodes})
	})}
	var p resource.DeleteResponse
	r.Delete(ctx, resource.DeleteRequest{State: fileState(t, r)}, &p)
	if !p.Diagnostics.HasError() || deletes != 0 {
		t.Fatal("recursive deletion was allowed")
	}
}
func TestFileChecksumDriftTriggersContentReplacement(t *testing.T) {
	r := &Resource{kind: "file_catalog_file"}
	s := fileState(t, r)
	s.SetAttribute(ctx, path.Root("content_base64"), base64.StdEncoding.EncodeToString([]byte("original")))
	d := r.refresh(ctx, &s, raw(map[string]any{"id": 7, "kind": "file", "name": "guide.txt", "path": "guide.txt", "sha256": "different"}))
	if d.HasError() {
		t.Fatal(d)
	}
	var v types.String
	s.GetAttribute(ctx, path.Root("content_base64"), &v)
	if !v.IsNull() {
		t.Fatal("did not expose changed content to replacement plan")
	}
}
func TestImportIdentityValidation(t *testing.T) {
	for _, tc := range []struct {
		kind, id string
		valid    bool
	}{{"note", "location:2", true}, {"note", "resource:2", true}, {"note", "location:0", false}, {"note", "user:1", false}, {"file_catalog_file", "2:7", true}, {"file_catalog_folder", "7", false}, {"tree_element", "7", true}, {"tree_element", "../7", false}, {"license", "lic-1", true}} {
		t.Run(tc.kind+tc.id, func(t *testing.T) {
			r := &Resource{kind: tc.kind}
			p := resource.ImportStateResponse{State: stateFor(t, r, nil)}
			r.ImportState(ctx, resource.ImportStateRequest{ID: tc.id}, &p)
			if p.Diagnostics.HasError() == tc.valid {
				t.Fatalf("unexpected import result %v", p.Diagnostics)
			}
		})
	}
}

func dataConfig(t *testing.T, d *DataSource, values map[string]attr.Value) tfsdk.Config {
	t.Helper()
	var schema datasource.SchemaResponse
	d.Schema(ctx, datasource.SchemaRequest{}, &schema)
	v := map[string]tftypes.Value{}
	for k, a := range schema.Schema.Attributes {
		v[k] = tftypes.NewValue(a.GetType().TerraformType(ctx), nil)
	}
	for k, a := range values {
		var e error
		v[k], e = a.ToTerraformValue(ctx)
		if e != nil {
			t.Fatal(e)
		}
	}
	return tfsdk.Config{Schema: schema.Schema, Raw: tftypes.NewValue(schema.Schema.Type().TerraformType(ctx), v)}
}
func TestDataSourceLookups(t *testing.T) {
	for _, tc := range []struct {
		kind     string
		values   map[string]attr.Value
		response any
		field    string
	}{{"license_status", nil, map[string]any{"available": false, "unavailable_reason": "leader unavailable", "capacity": []any{}}, "projection"}, {"license", map[string]attr.Value{"id": types.StringValue("lic-1")}, []any{map[string]any{"license_id": "lic-1", "feature": "business"}}, "metadata"}, {"file_catalog", map[string]attr.Value{"location_id": types.Int64Value(2)}, map[string]any{"nodes": []any{map[string]any{"id": 7, "kind": "file"}}}, "entries"}, {"file_catalog_file", map[string]attr.Value{"location_id": types.Int64Value(2), "id": types.StringValue("7")}, map[string]any{"nodes": []any{map[string]any{"id": 7, "kind": "file", "sha256": "hash"}}}, "metadata"}, {"file_catalog_folder", map[string]attr.Value{"location_id": types.Int64Value(2), "id": types.StringValue("7")}, map[string]any{"nodes": []any{map[string]any{"id": 7, "kind": "folder", "path": "docs"}}}, "metadata"}, {"tree_element", map[string]attr.Value{"id": types.StringValue("7")}, []any{map[string]any{"kind": "location", "children": []any{map[string]any{"id": 7, "kind": "element", "label": "Title"}}}}, "metadata"}, {"note", map[string]attr.Value{"scope_type": types.StringValue("location"), "scope_id": types.Int64Value(2)}, map[string]any{"latest_version": map[string]any{"version": 3, "content": "private"}}, "content"}} {
		t.Run(tc.kind, func(t *testing.T) {
			d := &DataSource{kind: tc.kind, client: api(t, func(w http.ResponseWriter, q *http.Request) {
				if q.Method != "GET" {
					t.Error("data source mutated API")
				}
				respond(w, tc.response)
			})}
			var p datasource.ReadResponse
			d.Read(ctx, datasource.ReadRequest{Config: dataConfig(t, d, tc.values)}, &p)
			if p.Diagnostics.HasError() {
				t.Fatal(p.Diagnostics)
			}
			if tc.kind == "note" {
				if stringValue(ctx, p.State, "content") != "private" || intValue(ctx, p.State, "version") != 3 {
					t.Fatal("wrong note lookup")
				}
			} else {
				var v types.Dynamic
				if dd := p.State.GetAttribute(ctx, path.Root(tc.field), &v); dd.HasError() || v.IsNull() {
					t.Fatalf("missing computed result %v", dd)
				}
				if tc.kind == "license_status" {
					raw, e := dynamicValue(ctx, p.State, "projection")
					if e != nil || !strings.Contains(string(raw), `"available":false`) {
						t.Fatal("lost unavailable status")
					}
				}
			}
		})
	}
}
func TestLicenseReadDistinguishesMissingAndForbidden(t *testing.T) {
	for _, forbidden := range []bool{true, false} {
		t.Run(strconvBool(forbidden), func(t *testing.T) {
			r := &Resource{kind: "license", client: api(t, func(w http.ResponseWriter, q *http.Request) {
				if forbidden {
					fail(w, 403, "forbidden")
				} else {
					respond(w, []any{})
				}
			})}
			var p resource.ReadResponse
			r.Read(ctx, resource.ReadRequest{State: licenseState(t, r)}, &p)
			if p.Diagnostics.HasError() != forbidden || p.State.Raw.IsNull() == forbidden {
				t.Fatal("confused access failure with missing license")
			}
		})
	}
}
func TestNoteCleanupFailureRetainsCommittedVersion(t *testing.T) {
	r := &Resource{kind: "note", client: api(t, func(w http.ResponseWriter, q *http.Request) {
		switch q.Method {
		case "GET":
			respond(w, map[string]any{"latest_version": map[string]any{"version": 1, "content": "old"}})
		case "PUT":
			respond(w, map[string]any{"version": 2, "content": "managed note"})
		case "DELETE":
			fail(w, 503, "unavailable")
		default:
			respond(w, nil)
		}
	})}
	s := noteState(t, r)
	var p resource.UpdateResponse
	r.Update(ctx, resource.UpdateRequest{Plan: planFor(s), State: s}, &p)
	if !p.Diagnostics.HasError() || intValue(ctx, p.State, "version") != 2 {
		t.Fatalf("lost committed note on cleanup error: %v", p.Diagnostics)
	}
}
func TestFolderCRUDUsesScopedCatalog(t *testing.T) {
	created, updated, deleted := false, false, false
	name := "guide.txt"
	r := &Resource{kind: "file_catalog_folder", client: api(t, func(w http.ResponseWriter, q *http.Request) {
		entry := func() map[string]any { return map[string]any{"id": 7, "kind": "folder", "name": name, "path": name} }
		switch q.Method {
		case "POST":
			created = true
			respond(w, entry())
		case "PATCH":
			updated = true
			var body map[string]any
			json.NewDecoder(q.Body).Decode(&body)
			name = body["name"].(string)
			respond(w, entry())
		case "GET":
			nodes := []any{}
			if q.URL.Query().Get("path") == "" {
				nodes = append(nodes, entry())
			}
			respond(w, map[string]any{"nodes": nodes})
		case "DELETE":
			deleted = true
			respond(w, nil)
		}
	})}
	s := fileState(t, r)
	var create resource.CreateResponse
	r.Create(ctx, resource.CreateRequest{Plan: planFor(s)}, &create)
	if create.Diagnostics.HasError() {
		t.Fatal(create.Diagnostics)
	}
	var read resource.ReadResponse
	r.Read(ctx, resource.ReadRequest{State: create.State}, &read)
	if read.Diagnostics.HasError() {
		t.Fatal(read.Diagnostics)
	}
	next := read.State
	next.SetAttribute(ctx, path.Root("name"), "renamed")
	var update resource.UpdateResponse
	r.Update(ctx, resource.UpdateRequest{State: read.State, Plan: planFor(next)}, &update)
	if update.Diagnostics.HasError() {
		t.Fatal(update.Diagnostics)
	}
	var destroy resource.DeleteResponse
	r.Delete(ctx, resource.DeleteRequest{State: update.State}, &destroy)
	if destroy.Diagnostics.HasError() || !created || !updated || !deleted {
		t.Fatalf("lifecycle failure %v", destroy.Diagnostics)
	}
}
func TestTreeCRUDPreservesNativeConfiguration(t *testing.T) {
	label := "Initial"
	config := map[string]any{"color": "info", "variant": "text"}
	r := &Resource{kind: "tree_element", client: api(t, func(w http.ResponseWriter, q *http.Request) {
		entry := func() map[string]any {
			return map[string]any{"id": 7, "kind": "element", "parent_key": "location:2", "element_type": "label", "label": label, "config": config}
		}
		switch q.Method {
		case "POST", "PATCH":
			var body map[string]any
			json.NewDecoder(q.Body).Decode(&body)
			label = body["label"].(string)
			if body["config"].(map[string]any)["color"] != "info" {
				t.Error("native config wasn't sent")
			}
			respond(w, entry())
		case "GET":
			respond(w, []any{entry()})
		case "DELETE":
			respond(w, true)
		}
	})}
	cfg := types.DynamicValue(types.ObjectValueMust(map[string]attr.Type{"color": types.StringType, "variant": types.StringType}, map[string]attr.Value{"color": types.StringValue("info"), "variant": types.StringValue("text")}))
	s := stateFor(t, r, map[string]attr.Value{"parent_key": types.StringValue("location:2"), "element_type": types.StringValue("label"), "label": types.StringValue("Initial"), "config": cfg})
	var create resource.CreateResponse
	r.Create(ctx, resource.CreateRequest{Plan: planFor(s)}, &create)
	if create.Diagnostics.HasError() {
		t.Fatal(create.Diagnostics)
	}
	var read resource.ReadResponse
	r.Read(ctx, resource.ReadRequest{State: create.State}, &read)
	if read.Diagnostics.HasError() {
		t.Fatal(read.Diagnostics)
	}
	next := read.State
	next.SetAttribute(ctx, path.Root("label"), "Changed")
	var update resource.UpdateResponse
	r.Update(ctx, resource.UpdateRequest{Plan: planFor(next), State: read.State}, &update)
	if update.Diagnostics.HasError() || stringValue(ctx, update.State, "label") != "Changed" {
		t.Fatal(update.Diagnostics)
	}
	var destroy resource.DeleteResponse
	r.Delete(ctx, resource.DeleteRequest{State: update.State}, &destroy)
	if destroy.Diagnostics.HasError() {
		t.Fatal(destroy.Diagnostics)
	}
}

func TestFailedUpdateKeepsPriorState(t *testing.T) {
	r := &Resource{kind: "note", client: api(t, func(w http.ResponseWriter, q *http.Request) {
		respond(w, map[string]any{"latest_version": map[string]any{"version": 2, "content": "concurrent change"}})
	})}
	prior := noteState(t, r)
	next := noteState(t, r)
	next.SetAttribute(ctx, path.Root("content"), "new desired content")
	var p resource.UpdateResponse
	r.Update(ctx, resource.UpdateRequest{State: prior, Plan: planFor(next)}, &p)
	if !p.Diagnostics.HasError() || !p.State.Raw.Equal(prior.Raw) {
		t.Fatal("failed update recorded unapplied plan")
	}
}
func TestLicensePostCommitReadFailureKeepsIdentity(t *testing.T) {
	r := &Resource{kind: "license", client: api(t, func(w http.ResponseWriter, q *http.Request) {
		if strings.HasSuffix(q.URL.Path, "preview") {
			respond(w, map[string]any{"already_installed": false})
			return
		}
		if q.Method == "POST" {
			respond(w, map[string]any{"license_id": "lic-1", "state": "pending"})
			return
		}
		fail(w, 503, "unavailable")
	})}
	s := licenseState(t, r)
	s.SetAttribute(ctx, path.Root("metadata"), types.DynamicUnknown())
	var p resource.CreateResponse
	r.Create(ctx, resource.CreateRequest{Plan: planFor(s)}, &p)
	var metadata types.Dynamic
	d := p.State.GetAttribute(ctx, path.Root("metadata"), &metadata)
	if !p.Diagnostics.HasError() || stringValue(ctx, p.State, "id") != "lic-1" || d.HasError() || metadata.IsUnknown() {
		t.Fatal("committed license identity not retained safely")
	}
}
func TestMalformedNoteLookupDoesNotRemoveState(t *testing.T) {
	r := &Resource{kind: "note", client: api(t, func(w http.ResponseWriter, q *http.Request) { respond(w, map[string]any{}) })}
	s := noteState(t, r)
	var p resource.ReadResponse
	r.Read(ctx, resource.ReadRequest{State: s}, &p)
	if !p.Diagnostics.HasError() || !p.State.Raw.Equal(s.Raw) {
		t.Fatal("malformed response removed note state")
	}
}

func TestReplacementPayloadPrerequisites(t *testing.T) {
	for _, tc := range []struct {
		name, kind, key string
		before, after   types.String
		field           string
		fieldValue      attr.Value
		wantError       bool
	}{
		{"file removal", "file_catalog_file", "content_base64", types.StringValue("b2xk"), types.StringNull(), "", nil, true},
		{"file malformed", "file_catalog_file", "content_base64", types.StringValue("b2xk"), types.StringValue("%%%"), "", nil, true},
		{"file unknown", "file_catalog_file", "content_base64", types.StringValue("b2xk"), types.StringUnknown(), "", nil, true},
		{"imported MIME change", "file_catalog_file", "content_base64", types.StringNull(), types.StringNull(), "content_type", types.StringValue("text/plain"), true},
		{"imported location change", "file_catalog_file", "content_base64", types.StringNull(), types.StringNull(), "location_id", types.Int64Value(3), true},
		{"imported name change", "file_catalog_file", "content_base64", types.StringNull(), types.StringNull(), "name", types.StringValue("new-name"), false},
		{"file valid replacement", "file_catalog_file", "content_base64", types.StringValue("b2xk"), types.StringValue("bmV3"), "", nil, false},
		{"license removal", "license", "license_key", types.StringValue("old-signed-key"), types.StringNull(), "", nil, true},
		{"license unknown", "license", "license_key", types.StringValue("old-signed-key"), types.StringUnknown(), "", nil, true},
		{"imported license flags", "license", "license_key", types.StringNull(), types.StringNull(), "confirm_over_capacity", types.BoolValue(true), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &Resource{kind: tc.kind}
			var before tfsdk.State
			if tc.kind == "license" {
				before = licenseState(t, r)
			} else {
				before = fileState(t, r)
			}
			before.SetAttribute(ctx, path.Root(tc.key), tc.before)
			after := before
			after.SetAttribute(ctx, path.Root(tc.key), tc.after)
			if tc.field != "" {
				after.SetAttribute(ctx, path.Root(tc.field), tc.fieldValue)
			}
			var p resource.ModifyPlanResponse
			r.ModifyPlan(ctx, resource.ModifyPlanRequest{State: before, Plan: planFor(after)}, &p)
			if p.Diagnostics.HasError() != tc.wantError {
				t.Fatalf("plan result: %v", p.Diagnostics)
			}
		})
	}
}

func TestNoteMalformedResponseNeverExposesContent(t *testing.T) {
	r := &Resource{kind: "note", client: api(t, func(w http.ResponseWriter, q *http.Request) {
		w.WriteHeader(502)
		_, _ = w.Write([]byte("private-note-body-marker: malformed upstream reply"))
	})}
	var p resource.ReadResponse
	r.Read(ctx, resource.ReadRequest{State: noteState(t, r)}, &p)
	if !p.Diagnostics.HasError() || strings.Contains(fmt.Sprint(p.Diagnostics), "private-note-body-marker") {
		t.Fatalf("note content leaked in diagnostics: %v", p.Diagnostics)
	}
}

func TestLicenseReplacementPreviewRejectsIneligibleInputs(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    int
		body      any
		confirm   bool
		wantError bool
	}{
		{"invalid key", 400, nil, false, true},
		{"already installed", 200, map[string]any{"already_installed": true, "requires_confirmation": false}, false, true},
		{"unconfirmed capacity", 200, map[string]any{"already_installed": false, "requires_confirmation": true}, false, true},
		{"confirmed capacity", 200, map[string]any{"already_installed": false, "requires_confirmation": true}, true, false},
		{"eligible", 200, map[string]any{"already_installed": false, "requires_confirmation": false}, false, false},
		{"incomplete preview", 200, map[string]any{}, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			r := &Resource{kind: "license", client: api(t, func(w http.ResponseWriter, q *http.Request) {
				calls++
				if q.Method != "POST" || q.URL.Path != "/api/v1/licenses/preview" {
					t.Error("preflight mutated license")
				}
				if tc.status != 200 {
					fail(w, tc.status, "invalid private-key-marker")
				} else {
					if body, ok := tc.body.(map[string]any); ok && body["already_installed"] != nil {
						body["issued_at"] = time.Now().Add(-time.Hour)
						body["expires_at"] = time.Now().Add(time.Hour)
					}
					respond(w, tc.body)
				}
			})}
			before := licenseState(t, r)
			after := before
			after.SetAttribute(ctx, path.Root("license_key"), "private-key-marker")
			after.SetAttribute(ctx, path.Root("confirm_over_capacity"), tc.confirm)
			var p resource.ModifyPlanResponse
			r.ModifyPlan(ctx, resource.ModifyPlanRequest{State: before, Plan: planFor(after)}, &p)
			if p.Diagnostics.HasError() != tc.wantError || calls != 1 || strings.Contains(fmt.Sprint(p.Diagnostics), "private-key-marker") {
				t.Fatalf("unsafe preview outcome: %v calls=%d", p.Diagnostics, calls)
			}
		})
	}
}
func TestFileReplacementLimitMustBeReadableAndValid(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    int
		value     any
		wantError bool
	}{{"forbidden", 403, nil, true}, {"unavailable", 503, nil, true}, {"missing", 200, nil, true}, {"invalid", 200, "bad", true}, {"zero", 200, 0, true}, {"too small", 200, 2, true}, {"fits", 200, 3, false}} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			r := &Resource{kind: "file_catalog_file", client: api(t, func(w http.ResponseWriter, q *http.Request) {
				calls++
				if q.Method != "GET" || q.URL.Path != "/api/v1/settings/files_max_upload_bytes" {
					t.Error("unexpected replacement preflight")
				}
				if tc.status != 200 {
					fail(w, tc.status, "blocked")
				} else {
					respond(w, map[string]any{"key": "files_max_upload_bytes", "value": tc.value})
				}
			})}
			before := fileState(t, r)
			before.SetAttribute(ctx, path.Root("content_base64"), "b2xk")
			after := before
			after.SetAttribute(ctx, path.Root("content_base64"), "bmV3")
			var p resource.ModifyPlanResponse
			r.ModifyPlan(ctx, resource.ModifyPlanRequest{State: before, Plan: planFor(after)}, &p)
			if p.Diagnostics.HasError() != tc.wantError || calls != 1 {
				t.Fatalf("unsafe limit outcome %v calls=%d", p.Diagnostics, calls)
			}
		})
	}
}

// Preview verifies signatures but intentionally accepts expired and future-issued
// claims. These successful responses must not authorize destructive replacement.
func TestLicenseReplacementChecksPreviewDates(t *testing.T) {
	now := time.Now().UTC()
	for _, tc := range []struct {
		name            string
		issued, expires any
		wantError       bool
	}{
		{"expired", now.Add(-2 * time.Hour), now.Add(-time.Hour), true},
		{"future issuer", now.Add(6 * time.Minute), now.Add(time.Hour), true},
		{"within issuer skew", now.Add(4 * time.Minute), now.Add(time.Hour), false},
		{"valid", now.Add(-time.Hour), now.Add(time.Hour), false},
		{"missing issue date", nil, now.Add(time.Hour), true},
		{"missing expiry date", now.Add(-time.Hour), nil, true},
		{"invalid issue date", "bad", now.Add(time.Hour), true},
		{"invalid expiry date", now.Add(-time.Hour), "bad", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &Resource{kind: "license", client: api(t, func(w http.ResponseWriter, q *http.Request) {
				if q.Method != "POST" || q.URL.Path != "/api/v1/licenses/preview" {
					t.Fatal("unexpected mutation")
				}
				respond(w, map[string]any{"already_installed": false, "requires_confirmation": false, "issued_at": tc.issued, "expires_at": tc.expires})
			})}
			before := licenseState(t, r)
			after := before
			after.SetAttribute(ctx, path.Root("license_key"), "replacement-private-key")
			var response resource.ModifyPlanResponse
			r.ModifyPlan(ctx, resource.ModifyPlanRequest{State: before, Plan: planFor(after)}, &response)
			if response.Diagnostics.HasError() != tc.wantError {
				t.Fatalf("unexpected date eligibility: %v", response.Diagnostics)
			}
		})
	}
}

func TestLicenseReplacementChecksCapacityAfterRemoval(t *testing.T) {
	r := &Resource{kind: "license", client: api(t, func(w http.ResponseWriter, q *http.Request) {
		if q.Method != "POST" {
			t.Error("unexpected mutation")
		}
		if strings.HasSuffix(q.URL.Path, "/removal-preview") {
			respond(w, map[string]any{"requires_confirmation": true})
		} else {
			respond(w, map[string]any{"already_installed": false, "requires_confirmation": false, "issued_at": time.Now().Add(-time.Hour), "expires_at": time.Now().Add(time.Hour)})
		}
	})}
	before := licenseState(t, r)
	before.SetAttribute(ctx, path.Root("confirm_removal_over_capacity"), true)
	after := before
	after.SetAttribute(ctx, path.Root("license_key"), "replacement-private-key")
	var response resource.ModifyPlanResponse
	r.ModifyPlan(ctx, resource.ModifyPlanRequest{State: before, Plan: planFor(after)}, &response)
	if !response.Diagnostics.HasError() {
		t.Fatal("replacement can delete old entitlement then fail installation capacity check")
	}
}
