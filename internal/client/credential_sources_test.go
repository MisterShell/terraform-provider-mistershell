package client

import (
	"encoding/json"
	"testing"
)

func TestCredentialResponseReadsIndependentSources(t *testing.T) {
	var c CredentialResponse
	if err := json.Unmarshal([]byte(`{"source":"external","user_source":"local","external_vault_id":3,"external_vault_ref":{"base":{"version":2,"path":"{{ resource.name }}"},"fields":{"password":"password"}},"external_vault_name":"vault","external_vault_type":"hashicorp","user_credential_data":null}`), &c); err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	json.Unmarshal(b, &decoded)
	if decoded["source"] != "external" || decoded["user_source"] != "local" {
		t.Fatalf("lost dual source metadata: %s", b)
	}
}
