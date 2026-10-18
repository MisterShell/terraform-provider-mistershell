package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"terraform-provider-mistershell/internal/client"
)

// externalVaultConfigPatch translates key removal to an explicit backend reset.
// PATCH overlays config, and omission alone would silently retain the old value.
func externalVaultConfigPatch(prior, planned types.Dynamic, catalog client.ExternalVaultType) (json.RawMessage, error) {
	oldRaw, err := dynamicToRawJSON(prior)
	if err != nil {
		return nil, err
	}
	newRaw, err := dynamicToRawJSON(planned)
	if err != nil {
		return nil, err
	}
	var old, next map[string]json.RawMessage
	if len(oldRaw) > 0 {
		if err = json.Unmarshal(oldRaw, &old); err != nil {
			return nil, err
		}
	}
	if err = json.Unmarshal(newRaw, &next); err != nil || next == nil {
		return nil, fmt.Errorf("config must be a known object")
	}
	// Null imported secrets mean the plaintext is unavailable, not a request to
	// clear it. Omit unchanged null values (and omitted unknown keys) from the
	// overlay so unrelated config edits keep the server's stored credential.
	for _, key := range catalog.SecretConfigKeys {
		if string(old[key]) == "null" {
			delete(old, key)
			if string(next[key]) == "null" {
				delete(next, key)
			}
		}
	}
	var schema struct {
		Properties map[string]struct {
			Default json.RawMessage `json:"default"`
			Type    json.RawMessage `json:"type"`
		} `json:"properties"`
	}
	if err = json.Unmarshal(catalog.ConfigSchema, &schema); err != nil {
		return nil, fmt.Errorf("decoding vault config schema: %w", err)
	}
	// Resolve removed selector defaults before checking dependent keys. Map iteration
	// order must not determine whether a removed identity role can be pruned.
	effective := make(map[string]json.RawMessage, len(next))
	for key, value := range next {
		effective[key] = value
	}
	for key := range old {
		if _, supplied := next[key]; !supplied && len(schema.Properties[key].Default) > 0 {
			effective[key] = schema.Properties[key].Default
		}
	}
	for key := range old {
		if _, exists := next[key]; exists {
			continue
		}
		if externalVaultPrunesKey(catalog.VaultType, key, effective) {
			continue
		}
		property, exists := schema.Properties[key]
		if exists && len(property.Default) > 0 {
			next[key] = property.Default
			continue
		}
		var kinds []string
		if json.Unmarshal(property.Type, &kinds) == nil {
			nullable := false
			for _, kind := range kinds {
				if kind == "null" {
					nullable = true
				}
			}
			if nullable {
				next[key] = json.RawMessage("null")
				continue
			}
		}
		return nil, fmt.Errorf("config.%s cannot be removed safely: the API overlays config and its catalog provides no reset value. Keep the key, switch the relevant authentication mode, or replace the vault", key)
	}
	return json.Marshal(next)
}

// These providers explicitly prune obsolete identity keys after validating a switch.
func externalVaultPrunesKey(kind, key string, next map[string]json.RawMessage) bool {
	var auth, identity string
	_ = json.Unmarshal(next["auth_method"], &auth)
	_ = json.Unmarshal(next["user_identity_mode"], &identity)
	switch kind {
	case "hashicorp", "openbao":
		if key == "user_identity_role" && identity == "off" {
			return true
		}
		owners := map[string]string{"token": "token", "approle_role_id": "approle", "approle_secret_id": "approle", "kubernetes_role": "kubernetes"}
		owner, ok := owners[key]
		return ok && auth != "" && auth != owner
	case "aws_secrets_manager":
		return auth == "ambient" && (key == "access_key_id" || key == "secret_access_key" || key == "session_token")
	case "azure_key_vault":
		return auth == "ambient" && (key == "tenant_id" || key == "client_id" || key == "client_secret")
	}
	return false
}

// Refresh only managed keys, preserving known secrets and Terraform value types.
// A fresh import hydrates readable fields and represents masked secrets as null.
func externalVaultRefreshConfig(ctx context.Context, prior types.Dynamic, raw json.RawMessage, secretKeys []string) (types.Dynamic, error) {
	if prior.IsNull() || prior.IsUnknown() {
		var imported map[string]json.RawMessage
		if err := json.Unmarshal(raw, &imported); err != nil {
			return types.DynamicNull(), err
		}
		for _, key := range secretKeys {
			if string(imported[key]) == `"****"` {
				imported[key] = json.RawMessage("null")
			}
		}
		sanitized, err := json.Marshal(imported)
		if err != nil {
			return types.DynamicNull(), err
		}
		return rawJSONToDynamic(sanitized)
	}
	var remote map[string]json.RawMessage
	if err := json.Unmarshal(raw, &remote); err != nil {
		return types.DynamicNull(), err
	}
	var attrs map[string]attr.Value
	switch v := prior.UnderlyingValue().(type) {
	case types.Object:
		attrs = v.Attributes()
	case types.Map:
		attrs = v.Elements()
	default:
		return types.DynamicNull(), fmt.Errorf("config must be an object")
	}
	for key, old := range attrs {
		value, exists := remote[key]
		if externalVaultEquivalentString(key, old, value) {
			continue
		}
		if string(value) == `"****"` && slices.Contains(secretKeys, key) {
			continue
		}
		if !exists {
			value = json.RawMessage("null")
		}
		tf, err := tftypes.ValueFromJSON(value, old.Type(ctx).TerraformType(ctx))
		if err != nil {
			return types.DynamicNull(), fmt.Errorf("decoding config.%s: %w", key, err)
		}
		converted, err := old.Type(ctx).ValueFromTerraform(ctx, tf)
		if err != nil {
			return types.DynamicNull(), fmt.Errorf("decoding config.%s: %w", key, err)
		}
		attrs[key] = converted
	}
	switch v := prior.UnderlyingValue().(type) {
	case types.Object:
		obj, d := types.ObjectValue(v.AttributeTypes(ctx), attrs)
		if d.HasError() {
			return types.DynamicNull(), fmt.Errorf("decoding config: %s", d)
		}
		return types.DynamicValue(obj), nil
	case types.Map:
		m, d := types.MapValue(v.ElementType(ctx), attrs)
		if d.HasError() {
			return types.DynamicNull(), fmt.Errorf("decoding config: %s", d)
		}
		return types.DynamicValue(m), nil
	}
	return prior, nil
}

// Match only transformations performed by Core's validate_origin and Azure
// authority_host normalization. Other URL changes still represent remote drift.
func externalVaultEquivalentString(key string, prior attr.Value, raw json.RawMessage) bool {
	value, ok := prior.(types.String)
	if !ok || value.IsNull() || value.IsUnknown() {
		return false
	}
	var remote string
	if json.Unmarshal(raw, &remote) != nil {
		return false
	}
	configured := value.ValueString()
	switch key {
	case "address", "endpoint_url", "vault_url":
		parsed, err := url.Parse(configured)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.User != nil || strings.ContainsAny(configured, "\\?#") || (parsed.EscapedPath() != "" && parsed.EscapedPath() != "/") {
			return false
		}
		for _, character := range configured {
			if character < 32 || unicode.IsSpace(character) {
				return false
			}
		}
		if parsed.Port() != "" {
			port, err := strconv.Atoi(parsed.Port())
			if err != nil || port < 0 || port > 65535 {
				return false
			}
		}
		return strings.TrimSuffix(configured, "/") == remote
	case "authority_host":
		return strings.TrimSuffix(strings.TrimPrefix(configured, "https://"), "/") == remote
	default:
		return false
	}
}
