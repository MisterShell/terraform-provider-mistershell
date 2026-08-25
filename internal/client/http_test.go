package client

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestPaginateFollowsMetaPages(t *testing.T) {
	var pages []int

	got, err := paginate(context.Background(), func(page int) ([]int, *PaginationMeta, error) {
		pages = append(pages, page)
		return []int{page}, &PaginationMeta{Page: page, Pages: 3, Total: 3}, nil
	})
	if err != nil {
		t.Fatalf("paginate returned an error: %v", err)
	}
	if want := []int{1, 2, 3}; !reflect.DeepEqual(got, want) {
		t.Fatalf("items = %v, want %v", got, want)
	}
	if want := []int{1, 2, 3}; !reflect.DeepEqual(pages, want) {
		t.Fatalf("requested pages = %v, want %v", pages, want)
	}
}

func TestAllPaginatedListMethodsFollowMetaPages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		if page != "1" && page != "2" {
			t.Errorf("%s requested page %q", r.URL.Path, page)
		}
		_, _ = fmt.Fprintf(w, `{"success":true,"message":"ok","data":[{"id":%s,"name":"item-%s"}],"meta":{"page":%s,"size":1,"total":2,"pages":2}}`, page, page, page)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "test", srv.Client())
	ctx := context.Background()
	tests := []struct {
		name string
		list func() (int, error)
	}{
		{"locations", func() (int, error) { v, e := c.ListLocations(ctx, LocationListFilter{}); return len(v), e }},
		{"resources", func() (int, error) {
			v, e := c.ListNetworkResources(ctx, NetworkResourceListFilter{})
			return len(v), e
		}},
		{"credentials", func() (int, error) { v, e := c.ListCredentials(ctx, CredentialListFilter{}); return len(v), e }},
		{"tags", func() (int, error) { v, e := c.ListTags(ctx, TagListFilter{}); return len(v), e }},
		{"roles", func() (int, error) { v, e := c.ListRoles(ctx, RoleListFilter{}); return len(v), e }},
		{"permissions", func() (int, error) { v, e := c.ListPermissions(ctx, PermissionListFilter{}); return len(v), e }},
		{"users", func() (int, error) { v, e := c.ListUsers(ctx, UserListFilter{}); return len(v), e }},
		{"log destinations", func() (int, error) { v, e := c.ListLogDestinations(ctx, LogDestinationListFilter{}); return len(v), e }},
		{"settings", func() (int, error) { v, e := c.ListSettings(ctx, SettingListFilter{}); return len(v), e }},
		{"ACLs", func() (int, error) { v, e := c.ListAcls(ctx, AclListFilter{}); return len(v), e }},
		{"auth providers", func() (int, error) { v, e := c.ListAuthProviders(ctx, AuthProviderListFilter{}); return len(v), e }},
		{"workers", func() (int, error) { v, e := c.ListWorkers(ctx, WorkerListFilter{}); return len(v), e }},
		{"AI models", func() (int, error) { v, e := c.ListAIModels(ctx, AIModelListFilter{}); return len(v), e }},
		{"AI prompts", func() (int, error) { v, e := c.ListAIPrompts(ctx, AIPromptListFilter{}); return len(v), e }},
		{"AI agents", func() (int, error) { v, e := c.ListAIAgents(ctx, AIAgentListFilter{}); return len(v), e }},
		{"AI skills", func() (int, error) { v, e := c.ListAISkills(ctx, AISkillListFilter{}); return len(v), e }},
		{"AI tools", func() (int, error) { v, e := c.ListAITools(ctx, AIToolListFilter{}); return len(v), e }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			count, err := test.list()
			if err != nil {
				t.Fatal(err)
			}
			if count != 2 {
				t.Fatalf("item count = %d, want 2", count)
			}
		})
	}
}

func TestDoRequestWithMetaDecodesPagination(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"success":true,"message":"ok","data":[{"id":1}],"meta":{"page":1,"size":1,"total":2,"pages":2}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "test", srv.Client())
	data, meta, err := c.doRequestWithMeta(context.Background(), http.MethodGet, "/items", nil)
	if err != nil {
		t.Fatalf("doRequestWithMeta returned an error: %v", err)
	}
	if string(data) != `[{"id":1}]` {
		t.Fatalf("data = %s", data)
	}
	if meta == nil || meta.Page != 1 || meta.Size != 1 || meta.Total != 2 || meta.Pages != 2 {
		t.Fatalf("meta = %#v", meta)
	}
}

func TestNewHTTPClientClonesDefaultTransport(t *testing.T) {
	got := NewHTTPClient(true)

	transport, ok := got.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport type = %T, want *http.Transport", got.Transport)
	}
	if transport == http.DefaultTransport {
		t.Fatal("transport must be a clone, not http.DefaultTransport")
	}
	if transport.TLSClientConfig == nil || !transport.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("insecure client must disable TLS certificate verification")
	}
	if got.Timeout == 0 {
		t.Fatal("HTTP timeout must be configured")
	}
}
