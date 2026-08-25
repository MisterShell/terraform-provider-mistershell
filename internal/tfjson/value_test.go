package tfjson

import (
	"encoding/json"
	"math/big"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestToRawJSONEncodesNativeTerraformValues(t *testing.T) {
	t.Parallel()

	value := types.DynamicValue(types.ObjectValueMust(
		map[string]attr.Type{
			"enabled": types.BoolType,
			"host":    types.StringType,
			"ports": types.TupleType{ElemTypes: []attr.Type{
				types.NumberType,
				types.NumberType,
			}},
			"ratio": types.NumberType,
		},
		map[string]attr.Value{
			"enabled": types.BoolValue(true),
			"host":    types.StringValue("10.0.1.1"),
			"ports": types.TupleValueMust(
				[]attr.Type{types.NumberType, types.NumberType},
				[]attr.Value{numberValue("22"), numberValue("2222")},
			),
			"ratio": numberValue("1.25"),
		},
	))

	got, err := ToRawJSON(value)
	if err != nil {
		t.Fatalf("ToRawJSON() error: %v", err)
	}

	const want = `{"enabled":true,"host":"10.0.1.1","ports":[22,2222],"ratio":1.25}`
	if string(got) != want {
		t.Fatalf("ToRawJSON() = %s, want %s", got, want)
	}
}

func TestToRawJSONSupportsTerraformCollectionTypes(t *testing.T) {
	t.Parallel()

	tests := map[string]types.Dynamic{
		"map": types.DynamicValue(types.MapValueMust(types.StringType, map[string]attr.Value{
			"region": types.StringValue("eu-west"),
		})),
		"list": types.DynamicValue(types.ListValueMust(types.BoolType, []attr.Value{
			types.BoolValue(true), types.BoolValue(false),
		})),
		"set": types.DynamicValue(types.SetValueMust(types.StringType, []attr.Value{
			types.StringValue("alpha"), types.StringValue("beta"),
		})),
	}

	for name, value := range tests {
		name, value := name, value
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := ToRawJSON(value)
			if err != nil {
				t.Fatalf("ToRawJSON() error: %v", err)
			}

			var decoded any
			if err := json.Unmarshal(got, &decoded); err != nil {
				t.Fatalf("ToRawJSON() returned invalid JSON %q: %v", got, err)
			}
			if decoded == nil {
				t.Fatal("ToRawJSON() decoded to nil")
			}
		})
	}
}

func TestToRawJSONMapsNullToAbsentAPIValue(t *testing.T) {
	t.Parallel()

	got, err := ToRawJSON(types.DynamicNull())
	if err != nil {
		t.Fatalf("ToRawJSON() error: %v", err)
	}
	if got != nil {
		t.Fatalf("ToRawJSON() = %q, want nil", got)
	}
}

func TestToRawJSONWritesIntegralNumbersWithoutExponentNotation(t *testing.T) {
	t.Parallel()

	got, err := ToRawJSON(types.DynamicValue(numberValue("9007199254740993")))
	if err != nil {
		t.Fatalf("ToRawJSON() error: %v", err)
	}
	const want = `9007199254740993`
	if string(got) != want {
		t.Fatalf("ToRawJSON() = %s, want %s", got, want)
	}
}

func TestToRawJSONRejectsUnknownValues(t *testing.T) {
	t.Parallel()

	tests := map[string]types.Dynamic{
		"top-level": types.DynamicUnknown(),
		"nested": types.DynamicValue(types.ObjectValueMust(
			map[string]attr.Type{"host": types.StringType},
			map[string]attr.Value{"host": types.StringUnknown()},
		)),
	}

	for name, value := range tests {
		name, value := name, value
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := ToRawJSON(value)
			if err == nil {
				t.Fatalf("ToRawJSON() = %q without an error diagnostic", got)
			}
			if got != nil {
				t.Fatalf("ToRawJSON() = %q, want nil on error", got)
			}
		})
	}
}

func TestFromRawJSONBuildsNativeTerraformValues(t *testing.T) {
	t.Parallel()

	got, err := FromRawJSON(json.RawMessage(
		`{"enabled":true,"host":"10.0.1.1","metadata":null,"ports":[22,2222],"ratio":1.25}`,
	))
	if err != nil {
		t.Fatalf("FromRawJSON() error: %v", err)
	}

	want := types.DynamicValue(types.ObjectValueMust(
		map[string]attr.Type{
			"enabled":  types.BoolType,
			"host":     types.StringType,
			"metadata": types.DynamicType,
			"ports": types.TupleType{ElemTypes: []attr.Type{
				types.NumberType,
				types.NumberType,
			}},
			"ratio": types.NumberType,
		},
		map[string]attr.Value{
			"enabled":  types.BoolValue(true),
			"host":     types.StringValue("10.0.1.1"),
			"metadata": types.DynamicNull(),
			"ports": types.TupleValueMust(
				[]attr.Type{types.NumberType, types.NumberType},
				[]attr.Value{numberValue("22"), numberValue("2222")},
			),
			"ratio": numberValue("1.25"),
		},
	))

	if !got.Equal(want) {
		t.Fatalf("FromRawJSON() = %s, want %s", got, want)
	}
}

func TestFromRawJSONSupportsHeterogeneousSettingScalars(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want types.Dynamic
	}{
		{name: "bool", raw: `true`, want: types.DynamicValue(types.BoolValue(true))},
		{name: "number", raw: `120`, want: types.DynamicValue(numberValue("120"))},
		{name: "string", raw: `"alerts@example.com"`, want: types.DynamicValue(types.StringValue("alerts@example.com"))},
		{name: "null", raw: `null`, want: types.DynamicNull()},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := FromRawJSON(json.RawMessage(test.raw))
			if err != nil {
				t.Fatalf("FromRawJSON() error: %v", err)
			}
			if !got.Equal(test.want) {
				t.Fatalf("FromRawJSON() = %s, want %s", got, test.want)
			}
		})
	}
}

func TestFromRawJSONRejectsMalformedAPIJSON(t *testing.T) {
	t.Parallel()

	got, err := FromRawJSON(json.RawMessage(`{"host":`))
	if err == nil {
		t.Fatalf("FromRawJSON() = %s without an error diagnostic", got)
	}
}

func TestSemanticallyEqualPreservesEquivalentTypedCollections(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value types.Dynamic
		raw   string
		want  bool
	}{
		{
			name: "map and object",
			value: types.DynamicValue(types.MapValueMust(types.StringType, map[string]attr.Value{
				"environment": types.StringValue("production"),
			})),
			raw:  `{"environment":"production"}`,
			want: true,
		},
		{
			name: "list and array",
			value: types.DynamicValue(types.ListValueMust(types.NumberType, []attr.Value{
				numberValue("22"), numberValue("2222"),
			})),
			raw:  `[22,2222]`,
			want: true,
		},
		{
			name: "equivalent number representations",
			value: types.DynamicValue(types.ObjectValueMust(
				map[string]attr.Type{"ratio": types.NumberType},
				map[string]attr.Value{"ratio": numberValue("1")},
			)),
			raw:  `{"ratio":1.0}`,
			want: true,
		},
		{
			name: "different value",
			value: types.DynamicValue(types.ObjectValueMust(
				map[string]attr.Type{"environment": types.StringType},
				map[string]attr.Value{"environment": types.StringValue("production")},
			)),
			raw:  `{"environment":"staging"}`,
			want: false,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := SemanticallyEqual(test.value, json.RawMessage(test.raw))
			if err != nil {
				t.Fatalf("SemanticallyEqual() error: %v", err)
			}
			if got != test.want {
				t.Fatalf("SemanticallyEqual() = %t, want %t", got, test.want)
			}
		})
	}
}

func numberValue(text string) types.Number {
	n, _, err := big.ParseFloat(text, 10, 512, big.ToNearestEven)
	if err != nil {
		panic(err)
	}
	return types.NumberValue(n)
}
