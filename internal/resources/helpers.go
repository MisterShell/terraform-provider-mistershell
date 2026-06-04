package resources

import (
	"context"
	"encoding/json"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-mistershell/internal/client"
)

// normalizedToRawJSON converts a jsontypes.Normalized to json.RawMessage for API requests.
func normalizedToRawJSON(n jsontypes.Normalized) json.RawMessage {
	if n.IsNull() || n.IsUnknown() {
		return nil
	}
	return json.RawMessage(n.ValueString())
}

// rawJSONToNormalized converts json.RawMessage from API to jsontypes.Normalized.
func rawJSONToNormalized(raw json.RawMessage) jsontypes.Normalized {
	if raw == nil || string(raw) == "null" || string(raw) == "" {
		return jsontypes.NewNormalizedNull()
	}
	return jsontypes.NewNormalizedValue(string(raw))
}

// stringPtrToValue converts *string to types.String.
func stringPtrToValue(s *string) types.String {
	if s == nil {
		return types.StringNull()
	}
	return types.StringValue(*s)
}

// stringValueToPtr converts types.String to *string.
func stringValueToPtr(s types.String) *string {
	if s.IsNull() || s.IsUnknown() {
		return nil
	}
	v := s.ValueString()
	return &v
}

// int64PtrToValue converts *int64 to types.Int64.
func int64PtrToValue(i *int64) types.Int64 {
	if i == nil {
		return types.Int64Null()
	}
	return types.Int64Value(*i)
}

// int64ValueToPtr converts types.Int64 to *int64.
func int64ValueToPtr(i types.Int64) *int64 {
	if i.IsNull() || i.IsUnknown() {
		return nil
	}
	v := i.ValueInt64()
	return &v
}

// float64PtrToValue converts *float64 to types.Float64.
func float64PtrToValue(f *float64) types.Float64 {
	if f == nil {
		return types.Float64Null()
	}
	return types.Float64Value(*f)
}

// float64ValueToPtr converts types.Float64 to *float64.
func float64ValueToPtr(f types.Float64) *float64 {
	if f.IsNull() || f.IsUnknown() {
		return nil
	}
	v := f.ValueFloat64()
	return &v
}

// boolPtrToValue converts *bool to types.Bool.
func boolPtrToValue(b *bool) types.Bool {
	if b == nil {
		return types.BoolNull()
	}
	return types.BoolValue(*b)
}

// boolValueToPtr converts types.Bool to *bool.
func boolValueToPtr(b types.Bool) *bool {
	if b.IsNull() || b.IsUnknown() {
		return nil
	}
	v := b.ValueBool()
	return &v
}

// optStringPtrToValue converts *string to types.String for optional computed timestamps.
func optStringPtrToValue(s *string) types.String {
	if s == nil {
		return types.StringNull()
	}
	return types.StringValue(*s)
}

// int64SetToSlice converts a types.Set of Int64 to []int64. Null/unknown yields nil.
func int64SetToSlice(s types.Set) []int64 {
	if s.IsNull() || s.IsUnknown() {
		return nil
	}
	elems := s.Elements()
	out := make([]int64, 0, len(elems))
	for _, e := range elems {
		if v, ok := e.(types.Int64); ok && !v.IsNull() && !v.IsUnknown() {
			out = append(out, v.ValueInt64())
		}
	}
	return out
}

// int64SliceToSet converts []int64 to a types.Set of Int64.
func int64SliceToSet(ids []int64) types.Set {
	elems := make([]attr.Value, 0, len(ids))
	for _, id := range ids {
		elems = append(elems, types.Int64Value(id))
	}
	return types.SetValueMust(types.Int64Type, elems)
}

// stringSetToSlice converts a types.Set of String to []string. Null/unknown yields nil.
func stringSetToSlice(s types.Set) []string {
	if s.IsNull() || s.IsUnknown() {
		return nil
	}
	elems := s.Elements()
	out := make([]string, 0, len(elems))
	for _, e := range elems {
		if v, ok := e.(types.String); ok && !v.IsNull() && !v.IsUnknown() {
			out = append(out, v.ValueString())
		}
	}
	return out
}

// stringSliceToSet converts []string to a types.Set of String.
func stringSliceToSet(vals []string) types.Set {
	elems := make([]attr.Value, 0, len(vals))
	for _, v := range vals {
		elems = append(elems, types.StringValue(v))
	}
	return types.SetValueMust(types.StringType, elems)
}

// aclPatternAttrTypes is the element object type for the ACL patterns list.
var aclPatternAttrTypes = map[string]attr.Type{
	"pattern": types.StringType,
	"type":    types.StringType,
}

// aclPatternObjectType is the object type for one ACL pattern.
var aclPatternObjectType = types.ObjectType{AttrTypes: aclPatternAttrTypes}

// aclPatternsToSlice converts a List[Object] of {pattern, type} to []client.AclPattern.
// Null/unknown yields nil.
func aclPatternsToSlice(l types.List) []client.AclPattern {
	if l.IsNull() || l.IsUnknown() {
		return nil
	}
	elems := l.Elements()
	out := make([]client.AclPattern, 0, len(elems))
	for _, e := range elems {
		obj, ok := e.(types.Object)
		if !ok {
			continue
		}
		attrs := obj.Attributes()
		var p client.AclPattern
		if v, ok := attrs["pattern"].(types.String); ok && !v.IsNull() && !v.IsUnknown() {
			p.Pattern = v.ValueString()
		}
		if v, ok := attrs["type"].(types.String); ok && !v.IsNull() && !v.IsUnknown() {
			p.Type = v.ValueString()
		}
		out = append(out, p)
	}
	return out
}

// aclPatternsToList converts []client.AclPattern to a List[Object] of {pattern, type}.
func aclPatternsToList(patterns []client.AclPattern) types.List {
	elems := make([]attr.Value, 0, len(patterns))
	for _, p := range patterns {
		obj := types.ObjectValueMust(aclPatternAttrTypes, map[string]attr.Value{
			"pattern": types.StringValue(p.Pattern),
			"type":    types.StringValue(p.Type),
		})
		elems = append(elems, obj)
	}
	return types.ListValueMust(aclPatternObjectType, elems)
}

// diffStrings returns the names present in want but not in have (added) and the
// names present in have but not in want (removed).
func diffStrings(have, want []string) (added, removed []string) {
	haveSet := make(map[string]struct{}, len(have))
	for _, h := range have {
		haveSet[h] = struct{}{}
	}
	wantSet := make(map[string]struct{}, len(want))
	for _, w := range want {
		wantSet[w] = struct{}{}
	}
	for w := range wantSet {
		if _, ok := haveSet[w]; !ok {
			added = append(added, w)
		}
	}
	for h := range haveSet {
		if _, ok := wantSet[h]; !ok {
			removed = append(removed, h)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	return added, removed
}

// int64UseStateForUnknown returns a plan modifier that copies the prior state value
// for unknown values during planning.
func int64UseStateForUnknown() planmodifier.Int64 {
	return &useStateForUnknownInt64Modifier{}
}

type useStateForUnknownInt64Modifier struct{}

func (m *useStateForUnknownInt64Modifier) Description(_ context.Context) string {
	return "Use state value for unknown."
}

func (m *useStateForUnknownInt64Modifier) MarkdownDescription(_ context.Context) string {
	return "Use state value for unknown."
}

func (m *useStateForUnknownInt64Modifier) PlanModifyInt64(_ context.Context, req planmodifier.Int64Request, resp *planmodifier.Int64Response) {
	if !req.PlanValue.IsUnknown() {
		return
	}
	if req.StateValue.IsNull() {
		return
	}
	resp.PlanValue = req.StateValue
}

// stringUseStateForUnknown returns a plan modifier that copies the prior state
// value for unknown string values during planning (string analogue of
// int64UseStateForUnknown — used for a computed id that mirrors a key).
func stringUseStateForUnknown() planmodifier.String {
	return &useStateForUnknownStringModifier{}
}

type useStateForUnknownStringModifier struct{}

func (m *useStateForUnknownStringModifier) Description(_ context.Context) string {
	return "Use state value for unknown."
}

func (m *useStateForUnknownStringModifier) MarkdownDescription(_ context.Context) string {
	return "Use state value for unknown."
}

func (m *useStateForUnknownStringModifier) PlanModifyString(_ context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if !req.PlanValue.IsUnknown() {
		return
	}
	if req.StateValue.IsNull() {
		return
	}
	resp.PlanValue = req.StateValue
}
