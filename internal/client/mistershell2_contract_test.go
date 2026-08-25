package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMisterShell2ManagedContracts(t *testing.T) {
	t.Run("role permission uses PUT", func(t *testing.T) {
		c := singleRequestClient(t, http.MethodPut, "/api/v1/roles/5/permissions/app.resources.read", nil)
		if err := c.AddRolePermission(context.Background(), 5, "app.resources.read"); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("user role uses PUT", func(t *testing.T) {
		c := singleRequestClient(t, http.MethodPut, "/api/v1/users/4/roles/6", nil)
		if err := c.AddUserRole(context.Background(), 4, 6); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("ACL update uses PATCH", func(t *testing.T) {
		c := singleRequestClient(t, http.MethodPatch, "/api/v1/session-policy/acls/3", map[string]any{"name": "acl"})
		if _, err := c.UpdateAcl(context.Background(), 3, AclUpdateInput{Name: "acl"}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("rule update uses PATCH", func(t *testing.T) {
		c := singleRequestClient(t, http.MethodPatch, "/api/v1/session-policy/rules/8", map[string]any{"name": "rule"})
		if _, err := c.UpdateRule(context.Background(), 8, RuleUpdateInput{Name: "rule"}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("auth provider create includes mnemonic", func(t *testing.T) {
		c := singleRequestClient(t, http.MethodPost, "/api/v1/auth-providers", map[string]any{"name": "Corporate", "mnemonic": "corporate", "provider_type": "OIDC"})
		if _, err := c.CreateAuthProvider(context.Background(), AuthProviderCreateInput{
			Name: "Corporate", Mnemonic: "corporate", ProviderType: "OIDC", Config: json.RawMessage(`{}`),
		}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("worker create includes false is_enabled", func(t *testing.T) {
		c := singleRequestClient(t, http.MethodPost, "/api/v1/workers", map[string]any{"name": "worker", "location_id": float64(2), "is_enabled": false})
		if _, err := c.CreateWorker(context.Background(), WorkerCreateInput{Name: "worker", LocationID: 2, IsEnabled: false}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("user create includes false is_active", func(t *testing.T) {
		c := singleRequestClient(t, http.MethodPost, "/api/v1/users", map[string]any{"email": "user@example.com", "first_name": "Test", "last_name": "User", "password": "password", "is_active": false})
		if _, err := c.CreateUser(context.Background(), UserCreateInput{Email: "user@example.com", FirstName: "Test", LastName: "User", Password: "password", IsActive: false}); err != nil {
			t.Fatal(err)
		}
	})
}

func singleRequestClient(t *testing.T, method, path string, expectedFields map[string]any) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method || r.URL.Path != path {
			t.Fatalf("request = %s %s, want %s %s", r.Method, r.URL.Path, method, path)
		}
		if expectedFields != nil {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			for key, want := range expectedFields {
				if got := body[key]; got != want {
					t.Fatalf("body[%q] = %#v, want %#v; body=%#v", key, got, want, body)
				}
			}
		}
		_, _ = w.Write([]byte(`{"success":true,"message":"ok","data":{"id":1}}`))
	}))
	t.Cleanup(srv.Close)
	return NewClient(srv.URL, "test", srv.Client())
}
