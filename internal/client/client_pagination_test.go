package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListMethodsUseSize10000(t *testing.T) {
	var gotSize string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSize = r.URL.Query().Get("size")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true,"message":"ok","data":[]}`))
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, APIKey: "test", HTTPClient: srv.Client()}

	cases := []struct {
		name string
		call func() error
	}{
		{"locations", func() error { _, e := c.ListLocations(context.Background(), LocationListFilter{}); return e }},
		{"resources", func() error {
			_, e := c.ListNetworkResources(context.Background(), NetworkResourceListFilter{})
			return e
		}},
		{"credentials", func() error { _, e := c.ListCredentials(context.Background(), CredentialListFilter{}); return e }},
		{"authproviders", func() error { _, e := c.ListAuthProviders(context.Background(), AuthProviderListFilter{}); return e }},
	}
	for _, tc := range cases {
		gotSize = ""
		if err := tc.call(); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if gotSize != "10000" {
			t.Errorf("%s: size = %q, want 10000", tc.name, gotSize)
		}
	}
}
