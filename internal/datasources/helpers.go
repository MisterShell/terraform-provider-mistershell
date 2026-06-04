package datasources

import (
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-mistershell/internal/client"
)

func rawJSONToNormalized(raw json.RawMessage) jsontypes.Normalized {
	if raw == nil || string(raw) == "null" || string(raw) == "" {
		return jsontypes.NewNormalizedNull()
	}
	return jsontypes.NewNormalizedValue(string(raw))
}

func stringPtrToValue(s *string) types.String {
	if s == nil {
		return types.StringNull()
	}
	return types.StringValue(*s)
}

func int64PtrToValue(i *int64) types.Int64 {
	if i == nil {
		return types.Int64Null()
	}
	return types.Int64Value(*i)
}

func float64PtrToValue(f *float64) types.Float64 {
	if f == nil {
		return types.Float64Null()
	}
	return types.Float64Value(*f)
}

func optStringPtrToValue(s *string) types.String {
	if s == nil {
		return types.StringNull()
	}
	return types.StringValue(*s)
}

// int64SliceToSet converts []int64 to a types.Set of Int64.
func int64SliceToSet(ids []int64) types.Set {
	elems := make([]attr.Value, 0, len(ids))
	for _, id := range ids {
		elems = append(elems, types.Int64Value(id))
	}
	return types.SetValueMust(types.Int64Type, elems)
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
