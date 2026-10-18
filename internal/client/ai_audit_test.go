package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAIAuditOverrideEndpoints(t *testing.T) {
	seen := map[string]bool{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.Path
		seen[key] = true
		switch key {
		case "PATCH /api/v1/ai/skills/4":
			var b map[string]any
			_ = json.NewDecoder(r.Body).Decode(&b)
			if len(b) != 1 || b["is_enabled"] != false {
				t.Errorf("builtin skill patch contains unrelated fields: %#v", b)
			}
			fmt.Fprint(w, `{"success":true,"data":{"id":4,"is_builtin":true,"is_enabled":false}}`)
		case "PATCH /api/v1/ai/agents/3":
			var b map[string]any
			_ = json.NewDecoder(r.Body).Decode(&b)
			if len(b) != 1 || b["model_id"] != nil {
				t.Errorf("builtin agent patch contains unrelated fields: %#v", b)
			}
			fmt.Fprint(w, `{"success":true,"data":{"id":3,"is_builtin":true}}`)
		case "GET /api/v1/ai/guardrails":
			fmt.Fprint(w, `{"success":true,"data":[{"resource_type":"generic_ssh","allowlist_enabled":true}]}`)
		case "PUT /api/v1/ai/guardrails/generic_ssh":
			fmt.Fprint(w, `{"success":true,"data":{"resource_type":"generic_ssh","allowlist_enabled":false}}`)
		case "DELETE /api/v1/ai/guardrails/generic_ssh":
			fmt.Fprint(w, `{"success":true,"data":{"resource_type":"generic_ssh","allowlist_enabled":true}}`)
		case "GET /api/v1/ai/prompt-variables":
			fmt.Fprint(w, `{"success":true,"data":{"universal":[{"name":"now","value_type":"datetime"}],"agents":[]}}`)
		default:
			t.Errorf("unexpected endpoint: %s", key)
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "test", srv.Client())
	ctx := context.Background()
	if _, e := c.UpdateAISkillEnabled(ctx, 4, false); e != nil {
		t.Fatal(e)
	}
	if _, e := c.UpdateAIAgentSettings(ctx, 3, map[string]any{"model_id": nil}); e != nil {
		t.Fatal(e)
	}
	rows, e := c.ListAIGuardrails(ctx)
	if e != nil || len(rows) != 1 {
		t.Fatalf("list: %v %#v", e, rows)
	}
	if _, e := c.SetAIGuardrail(ctx, "generic_ssh", false); e != nil {
		t.Fatal(e)
	}
	if e := c.ResetAIGuardrail(ctx, "generic_ssh"); e != nil {
		t.Fatal(e)
	}
	raw, e := c.GetAIPromptVariables(ctx)
	if e != nil || len(raw) == 0 {
		t.Fatalf("variables: %s %v", raw, e)
	}
	if len(seen) != 6 {
		t.Fatal(seen)
	}
}
