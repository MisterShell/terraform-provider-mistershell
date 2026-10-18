package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Catches dropping later pages, fuzzy lookup, or skipping the masked detail GET.
func TestExternalVaultLookupPagination(t *testing.T) {
	for _, field := range []string{"name", "mnemonic"} {
		t.Run(field, func(t *testing.T) {
			var pages, details int
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v1/external-vaults/7" {
					details++
					fmt.Fprint(w, `{"success":true,"data":{"id":7,"name":"prod","mnemonic":"prod","config":{"token":"****"}}}`)
					return
				}
				if r.URL.Path != "/api/v1/external-vaults" || r.URL.Query().Get("search") != "prod" {
					t.Errorf("unexpected request %s", r.URL)
					w.WriteHeader(400)
					return
				}
				pages++
				if r.URL.Query().Get("page") == "1" {
					fmt.Fprint(w, `{"success":true,"data":[{"id":1,"name":"production","mnemonic":"production"}],"meta":{"page":1,"pages":2}}`)
				} else {
					fmt.Fprint(w, `{"success":true,"data":[{"id":7,"name":"prod","mnemonic":"prod"}],"meta":{"page":2,"pages":2}}`)
				}
			}))
			defer srv.Close()
			c := &Client{BaseURL: srv.URL, APIKey: "test", HTTPClient: srv.Client()}
			name, mnemonic := "", ""
			if field == "name" {
				name = "prod"
			} else {
				mnemonic = "prod"
			}
			got, err := c.FindExternalVault(context.Background(), name, mnemonic)
			if err != nil || got.ID != 7 || pages != 2 || details != 1 || !strings.Contains(string(got.Config), "****") {
				t.Fatalf("got=%+v err=%v pages=%d details=%d", got, err, pages, details)
			}
		})
	}
}

func TestExternalVaultCRUDAndCatalog(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/external-vaults/types":
			fmt.Fprint(w, `{"success":true,"data":[{"vault_type":"hashicorp","secret_config_keys":["token"],"config_schema":{"properties":{"namespace":{"default":""}}}}]}`)
		case r.Method == "POST":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			cfg := body["config"].(map[string]any)
			if cfg["tls_verify"] != false || cfg["kv_version"] != float64(2) {
				t.Errorf("wrong typed config: %v", cfg)
			}
			fmt.Fprint(w, `{"success":true,"data":{"id":7,"config":{"token":"****"}}}`)
		case r.Method == "PATCH":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if len(body) != 1 || body["is_enabled"] != false {
				t.Errorf("PATCH includes untouched fields: %v", body)
			}
			fmt.Fprint(w, `{"success":true,"data":{"id":7}}`)
		case r.Method == "DELETE":
			w.WriteHeader(409)
			fmt.Fprint(w, `{"success":false,"message":"External vault is referenced by 2 credentials"}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
			w.WriteHeader(400)
		}
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, APIKey: "test", HTTPClient: srv.Client()}
	ctx := context.Background()
	created, err := c.CreateExternalVault(ctx, ExternalVaultCreateInput{Name: "prod", Mnemonic: "prod", VaultType: "hashicorp", Config: json.RawMessage(`{"tls_verify":false,"kv_version":2}`)})
	if err != nil || created.ID != 7 {
		t.Fatalf("%+v %v", created, err)
	}
	enabled := false
	if _, err = c.UpdateExternalVault(ctx, 7, ExternalVaultUpdateInput{IsEnabled: &enabled}); err != nil {
		t.Fatal(err)
	}
	catalog, err := c.ListExternalVaultTypes(ctx)
	if err != nil || len(catalog) != 1 || catalog[0].SecretConfigKeys[0] != "token" {
		t.Fatalf("%+v %v", catalog, err)
	}
	if err = c.DeleteExternalVault(ctx, 7); err == nil || !strings.Contains(err.Error(), "referenced") {
		t.Fatalf("lost deletion guard: %v", err)
	}
}

func TestExternalVaultLookupAmbiguous(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"success":true,"data":[{"id":1,"name":"same"},{"id":2,"name":"same"}]}`)
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, APIKey: "test", HTTPClient: srv.Client()}
	if _, err := c.FindExternalVault(context.Background(), "same", ""); err == nil || !strings.Contains(err.Error(), "multiple") {
		t.Fatalf("expected ambiguity, got %v", err)
	}
}
