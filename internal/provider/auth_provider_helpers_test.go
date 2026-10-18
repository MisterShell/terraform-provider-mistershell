package provider_test

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"terraform-provider-mistershell/internal/client"
)

func TestAuthProviderLicenseProbeHonorsInsecure(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/licenses/" || r.Header.Get("Authorization") != "Bearer apikey_test" {
			t.Errorf("unexpected license request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = io.WriteString(w, `{"success":true,"data":[{"id":1}]}`)
	}))
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	t.Cleanup(server.Close)
	t.Setenv("MISTERSHELL_URL", server.URL)
	t.Setenv("MISTERSHELL_API_KEY", "apikey_test")
	t.Setenv("MISTERSHELL_INSECURE", "true")
	if !authProviderLicensed(t) {
		t.Fatal("expected the licensed self-signed instance to be detected")
	}
}

// OIDC and SAML derive callback URLs from this instance-wide setting. These
// tests run sequentially; restore the actual prior value, not the default.
func testAccConfigureAppBaseURL(t *testing.T, providerType string) {
	t.Helper()
	if providerType != "OIDC" && providerType != "SAML" {
		return
	}
	c := testAccClient()
	if c == nil {
		t.Fatal("app_base_url fixture requires MISTERSHELL_URL and MISTERSHELL_API_KEY")
	}
	ctx := context.Background()
	original, err := c.GetSetting(ctx, "app_base_url")
	if err != nil {
		t.Fatalf("reading original app_base_url: %v", err)
	}
	t.Cleanup(func() {
		restored, err := c.UpdateSetting(ctx, "app_base_url", client.SettingUpdateInput{Value: original.Value})
		if err != nil {
			t.Errorf("restoring app_base_url: %v", err)
			return
		}
		if !jsonRawEqual(restored.Value, original.Value) {
			t.Errorf("app_base_url was not restored to its original value")
		}
	})
	value, err := json.Marshal(c.BaseURL)
	if err != nil {
		t.Fatalf("encoding app_base_url: %v", err)
	}
	updated, err := c.UpdateSetting(ctx, "app_base_url", client.SettingUpdateInput{Value: value})
	if err != nil {
		t.Fatalf("configuring app_base_url: %v", err)
	}
	if !jsonRawEqual(updated.Value, value) {
		t.Fatal("app_base_url did not match the acceptance endpoint")
	}
}

func TestAuthProviderBaseURLFixtureRestoresSetting(t *testing.T) {
	for _, providerType := range []string{"LDAP", "OIDC", "SAML"} {
		t.Run(providerType, func(t *testing.T) {
			const original = `"https://existing.example.com"`
			value := json.RawMessage(original)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v1/settings/app_base_url" || r.Header.Get("Authorization") != "Bearer apikey_test" {
					t.Errorf("unexpected setting request: %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				switch r.Method {
				case http.MethodPut:
					var input client.SettingUpdateInput
					if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
						t.Error(err)
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					value = input.Value
				case http.MethodGet:
				default:
					t.Errorf("unexpected method: %s", r.Method)
					w.WriteHeader(http.StatusMethodNotAllowed)
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": client.SettingResponse{
					Key: "app_base_url", Value: value, Default: json.RawMessage(`"http://localhost"`),
				}})
			}))
			t.Cleanup(server.Close)
			t.Setenv("MISTERSHELL_URL", server.URL)
			t.Setenv("MISTERSHELL_API_KEY", "apikey_test")
			t.Run("fixture", func(t *testing.T) {
				testAccConfigureAppBaseURL(t, providerType)
				want := original
				if providerType != "LDAP" {
					want = `"` + server.URL + `"`
				}
				if string(value) != want {
					t.Fatalf("setting during fixture = %s, want %s", value, want)
				}
			})
			if string(value) != original {
				t.Fatalf("setting after cleanup = %s, want %s", value, original)
			}
		})
	}
}
