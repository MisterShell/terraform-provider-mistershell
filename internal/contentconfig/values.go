package contentconfig

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"strconv"
	"terraform-provider-mistershell/internal/tfjson"
)

type reader interface {
	GetAttribute(context.Context, path.Path, interface{}) diag.Diagnostics
}

func stringValue(ctx context.Context, r reader, key string) string {
	var v types.String
	r.GetAttribute(ctx, path.Root(key), &v)
	return v.ValueString()
}
func intValue(ctx context.Context, r reader, key string) int64 {
	var v types.Int64
	r.GetAttribute(ctx, path.Root(key), &v)
	return v.ValueInt64()
}
func boolValue(ctx context.Context, r reader, key string) bool {
	var v types.Bool
	r.GetAttribute(ctx, path.Root(key), &v)
	return v.ValueBool()
}
func dynamicValue(ctx context.Context, r reader, key string) (json.RawMessage, error) {
	var v types.Dynamic
	d := r.GetAttribute(ctx, path.Root(key), &v)
	if d.HasError() {
		return nil, fmt.Errorf("cannot read %s", key)
	}
	return tfjson.ToRawJSON(v)
}
func set(ctx context.Context, s *tfsdk.State, key string, value any) diag.Diagnostics {
	return s.SetAttribute(ctx, path.Root(key), value)
}
func setDynamic(ctx context.Context, s *tfsdk.State, key string, raw json.RawMessage) diag.Diagnostics {
	var d diag.Diagnostics
	v, e := tfjson.FromRawJSON(raw)
	if e != nil {
		d.AddError("Invalid API response", e.Error())
		return d
	}
	return set(ctx, s, key, v)
}
func object(raw json.RawMessage) (map[string]json.RawMessage, error) {
	var v map[string]json.RawMessage
	e := json.Unmarshal(raw, &v)
	if e == nil && v == nil {
		e = fmt.Errorf("expected API response object")
	}
	return v, e
}
func jsonString(raw json.RawMessage) string {
	var v string
	if json.Unmarshal(raw, &v) == nil {
		return v
	}
	return string(raw)
}
func jsonInt(raw json.RawMessage) int64 { var v int64; _ = json.Unmarshal(raw, &v); return v }
func raw(v any) json.RawMessage         { b, _ := json.Marshal(v); return b }
func numericID(id string) error {
	n, e := strconv.ParseInt(id, 10, 64)
	if e != nil || n < 1 {
		return fmt.Errorf("expected positive numeric ID")
	}
	return nil
}
func reconcileConfig(ctx context.Context, s *tfsdk.State, b json.RawMessage) diag.Diagnostics {
	var d diag.Diagnostics
	var old types.Dynamic
	d.Append(s.GetAttribute(ctx, path.Root("config"), &old)...)
	v, e := tfjson.Reconcile(old, b, nil)
	if e != nil {
		d.AddError("Invalid tree configuration", e.Error())
		return d
	}
	d.Append(set(ctx, s, "config", v)...)
	return d
}
func knownString(ctx context.Context, r reader, key string) (string, error) {
	var v types.String
	d := r.GetAttribute(ctx, path.Root(key), &v)
	if d.HasError() || v.IsNull() || v.IsUnknown() {
		return "", fmt.Errorf("%s must be provided and known when creating", key)
	}
	return v.ValueString(), nil
}
