package resources

import (
	"encoding/json"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"strings"
	"terraform-provider-mistershell/internal/tfjson"
)

// PATCH merges config server-side. Explicitly reset fields Terraform previously
// managed, so deleting a key does not silently retain its old effective value.
func authProviderConfigPatch(providerType string, prior, planned types.Dynamic) (json.RawMessage, error) {
	raw, err := dynamicToRawJSON(planned)
	if err != nil {
		return nil, err
	}
	oldRaw, err := dynamicToRawJSON(prior)
	if err != nil {
		return nil, err
	}
	var old, next map[string]any
	if err := json.Unmarshal(raw, &next); err != nil {
		return nil, err
	}
	if len(oldRaw) > 0 {
		if err := json.Unmarshal(oldRaw, &old); err != nil {
			return nil, err
		}
	}
	defaults := authProviderConfigDefaults[providerType]
	for key := range old {
		if _, exists := next[key]; exists {
			continue
		}
		value, resettable := defaults[key]
		if !resettable {
			return nil, fmt.Errorf("config.%s is required by %s and cannot be removed", key, providerType)
		}
		next[key] = value
	}
	return json.Marshal(next)
}

var authProviderConfigDefaults = map[string]map[string]any{
	"LDAP": {
		"mfa_method": "none", "user_search_filter": "(uid={username})",
		"user_attributes":   map[string]string{"email": "mail", "first_name": "givenName", "last_name": "sn"},
		"group_search_base": nil, "group_search_filter": "(member={user_dn})", "use_tls": true, "tls_verify": false, "connection_timeout": 10,
	},
	"OIDC": {
		"scopes":            []string{"openid", "profile", "email"},
		"claims_mapping":    map[string]string{"email": "email", "first_name": "given_name", "last_name": "family_name", "groups": "groups"},
		"additional_params": map[string]string{}, "tls_verify": true, "ldap_group_provider_id": nil, "mcp_audience": nil, "mcp_email_claim": nil,
	},
	"SAML": {
		"attribute_mapping":      map[string]string{"email": "urn:oid:0.9.2342.19200300.100.1.3", "first_name": "urn:oid:2.5.4.42", "last_name": "urn:oid:2.5.4.4", "groups": "memberOf"},
		"ldap_group_provider_id": nil, "sign_requests": false, "want_assertions_signed": true, "sp_private_key": nil, "sp_x509_cert": nil,
	},
}

// The SAML API trims PEM boundaries. Preserve the configured representation
// only when that normalization explains the difference; replaced certificates
// and other non-secret changes must still be refreshed into state.
func authProviderRefreshConfig(providerType string, prior types.Dynamic, raw json.RawMessage) (types.Dynamic, error) {
	if providerType == "SAML" && !prior.IsNull() && !prior.IsUnknown() {
		oldRaw, err := dynamicToRawJSON(prior)
		if err != nil {
			return types.DynamicNull(), err
		}
		var old, current map[string]any
		if err = json.Unmarshal(oldRaw, &old); err != nil {
			return types.DynamicNull(), err
		}
		if err = json.Unmarshal(raw, &current); err != nil {
			return types.DynamicNull(), err
		}
		for _, key := range []string{"idp_x509_cert", "sp_x509_cert"} {
			previous, previousString := old[key].(string)
			remote, remoteString := current[key].(string)
			if previousString && remoteString && strings.TrimSpace(previous) == strings.TrimSpace(remote) {
				current[key] = previous
			}
		}
		raw, err = json.Marshal(current)
		if err != nil {
			return types.DynamicNull(), err
		}
	}
	return tfjson.Reconcile(prior, raw, []string{"bind_password", "client_secret", "sp_private_key"})
}
