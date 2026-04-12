package resources

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
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
