package contentconfig_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"terraform-provider-mistershell/internal/provider"
)

func contentFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){"mistershell": providerserver.NewProtocol6WithError(provider.New("test")())}
}
func contentProvider(url string) string {
	return fmt.Sprintf("provider \"mistershell\" {\n url = %q\n api_key = \"fixture\"\n}\n", url)
}
func contentResponse(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": data})
}

type fileFixture struct {
	mu                        sync.Mutex
	present                   bool
	id                        int
	name                      string
	content                   []byte
	uploads, deletes, patches int
}

func (f *fileFixture) entry() map[string]any {
	sum := sha256.Sum256(f.content)
	return map[string]any{"id": f.id, "location_id": 2, "name": f.name, "path": f.name, "kind": "file", "sha256": hex.EncodeToString(sum[:]), "size": len(f.content), "content_type": "text/plain"}
}
func (f *fileFixture) server(t *testing.T) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if q.Header.Get("Authorization") != "Bearer fixture" {
			t.Error("missing authentication")
		}
		switch {
		case q.Method == "GET" && q.URL.Path == "/api/v1/settings/files_max_upload_bytes":
			contentResponse(w, map[string]any{"key": "files_max_upload_bytes", "value": 4})
		case q.Method == "GET" && q.URL.Path == "/api/v1/files/catalog":
			rows := []any{}
			if f.present {
				rows = append(rows, f.entry())
			}
			contentResponse(w, map[string]any{"nodes": rows})
		case q.Method == "POST" && q.URL.Path == "/api/v1/files/catalog/upload":
			if f.present {
				t.Error("upload before deleting conflicting path")
			}
			f.present = true
			f.id++
			f.name = q.URL.Query().Get("name")
			f.content, _ = io.ReadAll(q.Body)
			f.uploads++
			contentResponse(w, f.entry())
		case q.Method == "PATCH":
			var body map[string]any
			json.NewDecoder(q.Body).Decode(&body)
			f.name = body["name"].(string)
			f.patches++
			contentResponse(w, f.entry())
		case q.Method == "DELETE":
			f.present = false
			f.deletes++
			contentResponse(w, nil)
		default:
			t.Errorf("unexpected fixture request %s %s", q.Method, q.URL.Path)
			w.WriteHeader(500)
		}
	}))
	t.Cleanup(s.Close)
	return s
}
func (f *fileFixture) counts(wantUploads, wantDeletes, wantPatches int) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.uploads != wantUploads || f.deletes != wantDeletes || f.patches != wantPatches {
			return fmt.Errorf("unexpected mutations uploads=%d deletes=%d patches=%d", f.uploads, f.deletes, f.patches)
		}
		return nil
	}
}

// These tests invoke Terraform's actual plan/apply protocol. A Create-only guard
// would fail them by deleting the fixture before discovering absent input.
func TestTerraformFileReplacementValidatesBeforeDestroy(t *testing.T) {
	f := &fileFixture{id: 6}
	s := f.server(t)
	config := func(extra string) string {
		return contentProvider(s.URL) + "resource \"mistershell_file_catalog_file\" \"test\" {\n location_id=2\n name=\"guide.txt\"\n" + extra + "\n}\n"
	}
	good := config("content_base64=\"b2xk\"")
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: contentFactories(), Steps: []resource.TestStep{
		{Config: good, Check: f.counts(1, 0, 0)},
		{Config: config(""), ExpectError: regexp.MustCompile("Missing replacement payload")},
		{Config: config("content_base64=\"%%%\""), ExpectError: regexp.MustCompile("Invalid file content")},
		{Config: config("content_base64=\"dG9vIGxhcmdl\""), ExpectError: regexp.MustCompile("Replacement file exceeds upload limit")},
		{Config: config("content_base64=terraform_data.generated.output") + "resource \"terraform_data\" \"generated\" { input=\"bmV3\" }", ExpectError: regexp.MustCompile("Missing replacement payload")},
		{Config: good, Check: f.counts(1, 0, 0)},
		{Config: config("content_base64=\"bmV3\""), Check: f.counts(2, 1, 0)},
	}})
}
func TestTerraformImportedFileMetadataUpdatesRemainLegal(t *testing.T) {
	f := &fileFixture{present: true, id: 7, name: "guide.txt", content: []byte("original")}
	s := f.server(t)
	config := func(name, extra string) string {
		return contentProvider(s.URL) + fmt.Sprintf("resource \"mistershell_file_catalog_file\" \"test\" {\n location_id=2\n name=%q\n%s\n}\n", name, extra)
	}
	base := config("guide.txt", "")
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: contentFactories(), Steps: []resource.TestStep{
		{Config: base, ResourceName: "mistershell_file_catalog_file.test", ImportState: true, ImportStateId: "2:7", ImportStatePersist: true},
		{Config: base, Check: f.counts(0, 0, 0)},
		{Config: config("guide.txt", "content_type=\"text/markdown\""), ExpectError: regexp.MustCompile("Missing replacement payload")},
		{Config: strings.Replace(base, "location_id=2", "location_id=3", 1), ExpectError: regexp.MustCompile("Missing replacement payload")},
		{Config: config("renamed.txt", ""), Check: f.counts(0, 0, 1)},
	}})
}
func TestTerraformLicensePayloadRemovalPreservesInstallation(t *testing.T) {
	var mu sync.Mutex
	installed := false
	deletes := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case q.URL.Path == "/api/v1/licenses/preview":
			var body map[string]string
			json.NewDecoder(q.Body).Decode(&body)
			if body["license_key"] == "long-malformed-private-license-key" {
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "message": "invalid " + body["license_key"]})
				return
			}
			issued, expires := time.Now().Add(-time.Hour), time.Now().Add(time.Hour)
			if body["license_key"] == "signed-but-expired-private-license-key" {
				issued, expires = time.Now().Add(-2*time.Hour), time.Now().Add(-time.Hour)
			}
			if body["license_key"] == "signed-but-future-private-license-key" {
				issued = time.Now().Add(6 * time.Minute)
			}
			contentResponse(w, map[string]any{"already_installed": false, "requires_confirmation": false, "issued_at": issued, "expires_at": expires})
		case strings.HasSuffix(q.URL.Path, "/removal-preview"):
			contentResponse(w, map[string]any{"requires_confirmation": false})
		case q.Method == "POST":
			installed = true
			contentResponse(w, map[string]any{"license_id": "license-1", "state": "reconciled"})
		case q.Method == "GET":
			rows := []any{}
			if installed {
				rows = append(rows, map[string]any{"license_id": "license-1", "feature": "base", "status": "active"})
			}
			contentResponse(w, rows)
		case q.Method == "DELETE":
			installed = false
			deletes++
			contentResponse(w, map[string]any{"license_id": "license-1", "state": "reconciled"})
		}
	}))
	defer s.Close()
	config := func(key string) string {
		return contentProvider(s.URL) + "resource \"mistershell_license\" \"test\" {\n" + key + "\n}\n"
	}
	good := config("license_key=\"signed-fixture-key\"")
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: contentFactories(), Steps: []resource.TestStep{
		{Config: good},
		{Config: config(""), ExpectError: regexp.MustCompile("Missing replacement payload")},
		{Config: config("license_key=\"short\""), ExpectError: regexp.MustCompile("Invalid license key")},
		{Config: config("license_key=\"long-malformed-private-license-key\""), ExpectError: regexp.MustCompile("Invalid replacement license")},
		{Config: config("license_key=\"signed-but-expired-private-license-key\""), ExpectError: regexp.MustCompile("Invalid replacement license")},
		{Config: config("license_key=\"signed-but-future-private-license-key\""), ExpectError: regexp.MustCompile("Invalid replacement license")},
		{Config: config("license_key=terraform_data.generated.output") + "resource \"terraform_data\" \"generated\" { input=\"next-signed-fixture\" }", ExpectError: regexp.MustCompile("Missing replacement payload")},
		{Config: good, Check: func(_ *terraform.State) error {
			mu.Lock()
			defer mu.Unlock()
			if !installed || deletes != 0 {
				return fmt.Errorf("rejected plan removed license")
			}
			return nil
		}},
	}})
}

func TestTerraformNewFileAcceptsGeneratedContent(t *testing.T) {
	f := &fileFixture{id: 6}
	s := f.server(t)
	config := contentProvider(s.URL) + `resource "terraform_data" "generated" { input = "b2xk" }
resource "mistershell_file_catalog_file" "test" {
 location_id = 2
 name = "guide.txt"
 content_base64 = terraform_data.generated.output
}`
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: contentFactories(), Steps: []resource.TestStep{{Config: config, Check: f.counts(1, 0, 0)}}})
}

func TestTerraformImportedLicenseFlagsDoNotRequireKey(t *testing.T) {
	var mu sync.Mutex
	deletes := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case q.URL.Path == "/api/v1/licenses/preview":
			contentResponse(w, map[string]any{"already_installed": true, "requires_confirmation": false})
		case q.Method == "GET":
			contentResponse(w, []any{map[string]any{"license_id": "license-1", "feature": "base", "status": "active"}})
		case strings.HasSuffix(q.URL.Path, "removal-preview"):
			contentResponse(w, map[string]any{"requires_confirmation": false})
		case q.Method == "DELETE":
			deletes++
			contentResponse(w, map[string]any{"state": "reconciled", "license_id": "license-1"})
		default:
			t.Errorf("unexpected imported license mutation: %s", q.Method)
		}
	}))
	defer s.Close()
	config := func(confirm bool) string {
		return contentProvider(s.URL) + fmt.Sprintf("resource \"mistershell_license\" \"test\" {\n confirm_removal_over_capacity=%t\n}\n", confirm)
	}
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: contentFactories(), Steps: []resource.TestStep{
		{Config: config(false), ResourceName: "mistershell_license.test", ImportState: true, ImportStateId: "license-1", ImportStatePersist: true},
		{Config: strings.Replace(config(false), "confirm_removal_over_capacity=false", "confirm_removal_over_capacity=false\n license_key=\"same-installed-key\"", 1), ExpectError: regexp.MustCompile("Replacement license already installed")},
		{Config: config(true), Check: func(_ *terraform.State) error {
			mu.Lock()
			defer mu.Unlock()
			if deletes != 0 {
				return fmt.Errorf("flag update removed imported license")
			}
			return nil
		}},
	}})
}
