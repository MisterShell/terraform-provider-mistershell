package tfjson

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Reconcile refreshes readable JSON while preserving masked secrets and the
// shape of managed objects. Server-added default keys are not adopted into an
// already managed object. Imports (null prior state) hydrate readable fields;
// unknown masked secrets become null, never the API mask sentinel.
func Reconcile(prior types.Dynamic, raw json.RawMessage, secretKeys []string) (types.Dynamic, error) {
	if prior.IsUnknown() {
		prior = types.DynamicNull()
	}
	remote, err := FromRawJSON(raw)
	if err != nil {
		return types.DynamicNull(), err
	}
	if remote.IsNull() {
		return remote, nil
	}
	priorRaw, err := ToRawJSON(prior)
	if err != nil {
		return types.DynamicNull(), err
	}
	decode := func(b []byte) (any, error) {
		if len(b) == 0 {
			return nil, nil
		}
		var v any
		d := json.NewDecoder(bytes.NewReader(b))
		d.UseNumber()
		e := d.Decode(&v)
		return v, e
	}
	old, err := decode(priorRaw)
	if err != nil {
		return types.DynamicNull(), err
	}
	current, err := decode(raw)
	if err != nil {
		return types.DynamicNull(), err
	}
	secrets := make(map[string]bool, len(secretKeys))
	for _, k := range secretKeys {
		secrets[strings.ToLower(k)] = true
	}
	merged := reconcileJSON(old, current, secrets, "")
	encoded, err := json.Marshal(merged)
	if err != nil {
		return types.DynamicNull(), err
	}
	equal, err := SemanticallyEqual(prior, encoded)
	if err != nil {
		return types.DynamicNull(), err
	}
	if equal {
		return prior, nil
	}
	return FromRawJSON(encoded)
}

func reconcileJSON(prior, current any, secrets map[string]bool, key string) any {
	if s, ok := current.(string); ok && secrets[strings.ToLower(key)] && (s == "***" || s == "****") {
		return prior
	}
	switch current := current.(type) {
	case map[string]any:
		old, managed := prior.(map[string]any)
		result := map[string]any{}
		if managed {
			for k, v := range old {
				result[k] = reconcileJSON(v, current[k], secrets, k)
			}
		} else {
			for k, v := range current {
				result[k] = reconcileJSON(nil, v, secrets, k)
			}
		}
		return result
	case []any:
		old, _ := prior.([]any)
		result := make([]any, len(current))
		for i, v := range current {
			var p any
			if i < len(old) {
				p = old[i]
			}
			result[i] = reconcileJSON(p, v, secrets, key)
		}
		return result
	default:
		return current
	}
}
