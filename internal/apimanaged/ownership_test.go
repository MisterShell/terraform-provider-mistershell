package apimanaged

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"terraform-provider-mistershell/internal/client"
	"testing"
)

func TestMutationOwnershipGuards(t *testing.T) {
	for _, tc := range []struct{ name, body, want string }{
		{"sensor_custom_rule", `{"mode":"auto"}`, "automatic"},
		{"configure_resource_vars", `{"values":{"hostname":"owned"}}`, "import"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					t.Errorf("guard mutated API: %s", r.Method)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"success":true,"data":` + tc.body + `}`))
			}))
			defer s.Close()
			r := &ManagedResource{definition: Definition{Name: tc.name}, client: &client.Client{BaseURL: s.URL, HTTPClient: s.Client()}}
			err := r.guardCreate(context.Background(), map[string]json.RawMessage{"resource_id": json.RawMessage(`1`), "stack_id": json.RawMessage(`2`)})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %s, got %v", tc.want, err)
			}
		})
	}
}

func TestSensorSourceParameterRemovalRejected(t *testing.T) {
	for _, raw := range []string{`{}`, `null`, `{"code":""}`, `{"code":null}`, `{"code":"****"}`} {
		if err := validateSourceParameterChange(json.RawMessage(`{"code":"old"}`), json.RawMessage(raw)); err == nil {
			t.Errorf("accepted retained secret transition %s", raw)
		}
	}
	if err := validateSourceParameterChange(json.RawMessage(`{"code":"old"}`), json.RawMessage(`{"code":"new"}`)); err != nil {
		t.Fatal(err)
	}
}
