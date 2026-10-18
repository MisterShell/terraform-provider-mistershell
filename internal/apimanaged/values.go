package apimanaged

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"strings"
	"terraform-provider-mistershell/internal/tfjson"
)

type attributeReader interface {
	GetAttribute(context.Context, path.Path, interface{}) diag.Diagnostics
}

func get(ctx context.Context, reader attributeReader, f Field) (attr.Value, diag.Diagnostics) {
	switch f.Kind {
	case "string":
		var v types.String
		d := reader.GetAttribute(ctx, path.Root(f.Name), &v)
		return v, d
	case "integer":
		var v types.Int64
		d := reader.GetAttribute(ctx, path.Root(f.Name), &v)
		return v, d
	case "boolean":
		var v types.Bool
		d := reader.GetAttribute(ctx, path.Root(f.Name), &v)
		return v, d
	default:
		var v types.Dynamic
		d := reader.GetAttribute(ctx, path.Root(f.Name), &v)
		return v, d
	}
}
func object(raw json.RawMessage) (map[string]json.RawMessage, error) {
	var v map[string]json.RawMessage
	err := json.Unmarshal(raw, &v)
	return v, err
}
func idString(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return string(raw)
}
func rawValue(v attr.Value) (json.RawMessage, error) {
	if v.IsNull() {
		return json.RawMessage("null"), nil
	}
	if v.IsUnknown() {
		return nil, fmt.Errorf("configuration contains an unknown value")
	}
	return tfjson.ToRawJSON(types.DynamicValue(v))
}
func payload(ctx context.Context, d Definition, plan attributeReader, previous attributeReader, creating bool) (map[string]json.RawMessage, diag.Diagnostics) {
	out := map[string]json.RawMessage{}
	var diags diag.Diagnostics
	for _, f := range d.Fields {
		if creating && !f.Create || !creating && !f.Update {
			continue
		}
		v, dd := get(ctx, plan, f)
		diags.Append(dd...)
		if diags.HasError() {
			return nil, diags
		}
		if v.IsUnknown() {
			if f.Computed {
				continue
			}
			diags.AddError("Unknown configuration", f.Name+" must be known before applying.")
			continue
		}
		if creating && v.IsNull() {
			continue
		}
		if previous != nil && d.UpdateMethod != "PUT" {
			old, dd := get(ctx, previous, f)
			diags.Append(dd...)
			if v.Equal(old) {
				continue
			}
		}
		raw, e := rawValue(v)
		if e != nil {
			diags.AddError("Invalid configuration", f.Name+": "+e.Error())
			continue
		}
		if !creating && previous != nil && d.Name == "automation_playbook" && f.Name == "graph" {
			old, dd := get(ctx, previous, f)
			diags.Append(dd...)
			if diags.HasError() {
				return nil, diags
			}
			priorRaw, err := rawValue(old)
			if err == nil {
				raw, err = preserveImportedPlaybookSecrets(priorRaw, raw)
			}
			if err != nil {
				diags.AddError("Invalid webhook authentication update", err.Error())
				continue
			}
		}
		out[f.Name] = raw
	}
	return out, diags
}
func refresh(ctx context.Context, d Definition, state *tfsdk.State, raw json.RawMessage, preservePlannedNull bool) diag.Diagnostics {
	var diags diag.Diagnostics
	obj, err := object(raw)
	if err != nil {
		diags.AddError("Invalid API response", err.Error())
		return diags
	}
	if id, ok := obj["id"]; ok && d.Name != "configure_resource_vars" {
		diags.Append(state.SetAttribute(ctx, path.Root("id"), types.StringValue(idString(id)))...)
	}
	for _, f := range d.Fields {
		data, present := obj[f.Name]
		old, dd := get(ctx, *state, f)
		diags.Append(dd...)
		if diags.HasError() {
			return diags
		}
		if !present {
			if f.Name == "token" {
				continue
			}
			data = json.RawMessage("null")
		}
		if preservePlannedNull && f.Writable() && !f.Computed && old.IsNull() {
			continue
		}
		switch f.Kind {
		case "string":
			var v *string
			if err = json.Unmarshal(data, &v); err != nil {
				diags.AddError("Invalid API response", f.Name+": "+err.Error())
				continue
			}
			if f.Name == "pem" && v != nil && !old.IsNull() && !old.IsUnknown() && strings.TrimSpace(*v) == strings.TrimSpace(old.(types.String).ValueString()) {
				diags.Append(state.SetAttribute(ctx, path.Root(f.Name), old)...)
			} else {
				diags.Append(state.SetAttribute(ctx, path.Root(f.Name), types.StringPointerValue(v))...)
			}
		case "integer":
			var v *int64
			if err = json.Unmarshal(data, &v); err != nil {
				diags.AddError("Invalid API response", f.Name+": "+err.Error())
				continue
			}
			diags.Append(state.SetAttribute(ctx, path.Root(f.Name), types.Int64PointerValue(v))...)
		case "boolean":
			var v *bool
			if err = json.Unmarshal(data, &v); err != nil {
				diags.AddError("Invalid API response", f.Name+": "+err.Error())
				continue
			}
			diags.Append(state.SetAttribute(ctx, path.Root(f.Name), types.BoolPointerValue(v))...)
		default:
			var v types.Dynamic
			if f.Writable() {
				secretKeys := append([]string(nil), f.SecretKeys...)
				if f.Name == "parameters" {
					var keys map[string]any
					_ = json.Unmarshal(data, &keys)
					for k := range keys {
						secretKeys = append(secretKeys, k)
					}
				}
				v, err = tfjson.Reconcile(old.(types.Dynamic), data, secretKeys)
			} else {
				v, err = tfjson.FromRawJSON(data)
			}
			if err != nil {
				diags.AddError("Invalid API configuration", f.Name+": "+err.Error())
				continue
			}
			diags.Append(state.SetAttribute(ctx, path.Root(f.Name), v)...)
		}
	}
	return diags
}
func valueIsEmpty(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || len(raw) == 0
}
