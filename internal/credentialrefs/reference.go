// Package credentialrefs preserves the typed Terraform shape of unresolved vault references.
package credentialrefs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"terraform-provider-mistershell/internal/tfjson"
)

func AttributeTypes() map[string]attr.Type {
	return map[string]attr.Type{"base": types.DynamicType, "fields": types.MapType{ElemType: types.StringType}}
}

func ToJSON(value types.Object) (json.RawMessage, error) {
	if value.IsNull() {
		return nil, nil
	}
	return tfjson.ToRawJSON(types.DynamicValue(value))
}

func FromJSON(raw json.RawMessage, prior types.Object) (types.Object, error) {
	empty := types.ObjectNull(AttributeTypes())
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return empty, nil
	}
	if !prior.IsNull() && !prior.IsUnknown() {
		equal, err := tfjson.SemanticallyEqual(types.DynamicValue(prior), raw)
		if err == nil && equal {
			return prior, nil
		}
	}
	var ref struct {
		Base   json.RawMessage   `json:"base"`
		Fields map[string]string `json:"fields"`
	}
	if err := json.Unmarshal(raw, &ref); err != nil {
		return empty, fmt.Errorf("invalid vault reference: %w", err)
	}
	base, err := tfjson.FromRawJSON(ref.Base)
	if err != nil {
		return empty, err
	}
	fields := make(map[string]attr.Value, len(ref.Fields))
	for k, v := range ref.Fields {
		fields[k] = types.StringValue(v)
	}
	return types.ObjectValueMust(AttributeTypes(), map[string]attr.Value{"base": base, "fields": types.MapValueMust(types.StringType, fields)}), nil
}

// Known recursively checks nested dynamic values so configuration validation can
// defer unresolved expressions until apply without treating them as absent.
func Known(v attr.Value) bool {
	if v == nil || v.IsUnknown() {
		return false
	}
	if v.IsNull() {
		return true
	}
	switch v := v.(type) {
	case types.Dynamic:
		return Known(v.UnderlyingValue())
	case types.Object:
		for _, a := range v.Attributes() {
			if !Known(a) {
				return false
			}
		}
	case types.Map:
		for _, a := range v.Elements() {
			if !Known(a) {
				return false
			}
		}
	case types.List:
		for _, a := range v.Elements() {
			if !Known(a) {
				return false
			}
		}
	case types.Tuple:
		for _, a := range v.Elements() {
			if !Known(a) {
				return false
			}
		}
	case types.Set:
		for _, a := range v.Elements() {
			if !Known(a) {
				return false
			}
		}
	}
	return true
}
