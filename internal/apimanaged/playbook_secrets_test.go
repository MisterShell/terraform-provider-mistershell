package apimanaged

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestImportedPlaybookSecretsPreservedByNodeIdentity(t *testing.T) {
	graph := func(id, action, auth string) json.RawMessage {
		return json.RawMessage(`{"nodes":[{"id":"` + id + `","action_id":"` + action + `","config":{"auth":` + auth + `}}]}`)
	}
	for _, tc := range []struct {
		name, oldAuth, newAuth, id, action string
		wantErr, wantMask                  bool
	}{
		{"null imported token", `{"type":"bearer","token":null}`, `{"type":"bearer","token":null}`, "node", "webhook", false, true},
		{"omitted imported token", `{"type":"bearer","token":null}`, `{"type":"bearer"}`, "node", "webhook", false, true},
		{"known secret clear rejected", `{"type":"bearer","token":"known"}`, `{"type":"bearer","token":null}`, "node", "webhook", true, false},
		{"known secret omit rejected", `{"type":"bearer","token":"known"}`, `{"type":"bearer"}`, "node", "webhook", true, false},
		{"explicitly disable auth", `{"type":"bearer","token":"known"}`, `{"type":"none"}`, "node", "webhook", false, false},
		{"new node cannot reuse secret", `{"type":"bearer","token":null}`, `{"type":"bearer"}`, "new-node", "webhook", true, false},
		{"new auth type needs secret", `{"type":"bearer","token":null}`, `{"type":"basic","username":"audit"}`, "node", "webhook", true, false},
		{"null imported password", `{"type":"basic","username":"audit","password":null}`, `{"type":"basic","username":"audit","password":null}`, "node", "webhook", false, true},
		{"omitted imported header value", `{"type":"header","header_name":"X-Key","header_value":null}`, `{"type":"header","header_name":"X-Key"}`, "node", "webhook", false, true},
		{"blank does not clear", `{"type":"bearer","token":"known"}`, `{"type":"bearer","token":" "}`, "node", "webhook", true, false},
		{"configured mask is not a secret", `{"type":"bearer","token":null}`, `{"type":"bearer","token":"****"}`, "node", "webhook", true, false},
		{"rotate secret", `{"type":"bearer","token":null}`, `{"type":"bearer","token":"new"}`, "node", "webhook", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := preserveImportedPlaybookSecrets(graph("node", "webhook", tc.oldAuth), graph(tc.id, tc.action, tc.newAuth))
			if (err != nil) != tc.wantErr {
				t.Fatalf("error=%v wantErr=%v", err, tc.wantErr)
			}
			if err == nil && strings.Contains(string(raw), `"****"`) != tc.wantMask {
				t.Fatalf("preservation sentinel incorrect: %s", raw)
			}
		})
	}
	old := json.RawMessage(`{"nodes":[{"id":"first","action_id":"webhook","config":{"auth":{"type":"bearer","token":null}}},{"id":"second","action_id":"webhook","config":{"auth":{"type":"bearer","token":"known"}}}]}`)
	next := json.RawMessage(`{"nodes":[{"id":"second","action_id":"webhook","config":{"auth":{"type":"none"}}},{"id":"first","action_id":"webhook","config":{"auth":{"type":"bearer","token":null}}}]}`)
	raw, err := preserveImportedPlaybookSecrets(old, next)
	if err != nil || !strings.Contains(string(raw), `"token":"****"`) {
		t.Fatalf("reordered imported node lost secret: %s %v", raw, err)
	}
}
