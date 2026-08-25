package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListResourceTypesUsesRuntimeCatalog(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/resources/types" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"success":true,"message":"ok","data":[{"type":"linux","engine_id":"ssh","display_name":"Linux","description":"Linux host","connector_schema":{"type":"object"},"credential_types":["username_password"]}]}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "test", srv.Client())
	got, err := c.ListResourceTypes(context.Background())
	if err != nil {
		t.Fatalf("ListResourceTypes returned an error: %v", err)
	}
	if len(got) != 1 || got[0].Type != "linux" || got[0].EngineID != "ssh" {
		t.Fatalf("resource types = %#v", got)
	}
	if string(got[0].ConnectorSchema) != `{"type":"object"}` {
		t.Fatalf("connector schema = %s", got[0].ConnectorSchema)
	}
	if len(got[0].CredentialTypes) != 1 || got[0].CredentialTypes[0] != "username_password" {
		t.Fatalf("credential types = %v", got[0].CredentialTypes)
	}
}

func TestFindResourceTypeResolvesEngine(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"success":true,"message":"ok","data":[{"type":"cisco_ios","engine_id":"ssh","display_name":"Cisco IOS","description":"IOS","connector_schema":{},"credential_types":["username_password"]}]}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "test", srv.Client())
	got, err := c.FindResourceType(context.Background(), "cisco_ios")
	if err != nil {
		t.Fatalf("FindResourceType returned an error: %v", err)
	}
	if got.EngineID != "ssh" {
		t.Fatalf("engine = %q, want ssh", got.EngineID)
	}
	if _, err := c.FindResourceType(context.Background(), "missing"); err == nil {
		t.Fatal("unknown resource type must return an error")
	}
}

func TestListCredentialTypesUsesRuntimeCatalog(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/credentials/types" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"success":true,"message":"ok","data":[{"type":"ssh_key","display_name":"SSH key","description":"Private key","schema":{"type":"object","required":["username","private_key"]}}]}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "test", srv.Client())
	got, err := c.ListCredentialTypes(context.Background())
	if err != nil {
		t.Fatalf("ListCredentialTypes returned an error: %v", err)
	}
	if len(got) != 1 || got[0].Type != "ssh_key" || got[0].DisplayName != "SSH key" {
		t.Fatalf("credential types = %#v", got)
	}
	if string(got[0].Schema) != `{"type":"object","required":["username","private_key"]}` {
		t.Fatalf("credential schema = %s", got[0].Schema)
	}
}
