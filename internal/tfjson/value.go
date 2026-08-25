// Package tfjson converts free-form native Terraform values to and from the
// JSON payloads used by the MisterShell API.
package tfjson

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/big"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// ToRawJSON converts a native Terraform dynamic value to an API JSON payload.
// A null value represents an absent API value. Unknown values are rejected.
func ToRawJSON(value types.Dynamic) (json.RawMessage, error) {
	if value.IsNull() {
		return nil, nil
	}
	if value.IsUnknown() {
		return nil, fmt.Errorf("MisterShell API values must be fully known before they can be sent")
	}

	encoded, err := terraformValueToGo(value.UnderlyingValue())
	if err != nil {
		return nil, fmt.Errorf("cannot encode Terraform value as JSON: %w", err)
	}

	raw, err := json.Marshal(encoded)
	if err != nil {
		return nil, fmt.Errorf("cannot encode Terraform value as JSON: %w", err)
	}
	return json.RawMessage(raw), nil
}

// FromRawJSON converts an API JSON payload to a native Terraform dynamic value.
// An absent or JSON null value becomes a Terraform dynamic null.
func FromRawJSON(raw json.RawMessage) (types.Dynamic, error) {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return types.DynamicNull(), nil
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return types.DynamicNull(), fmt.Errorf("cannot decode MisterShell JSON value: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return types.DynamicNull(), fmt.Errorf("cannot decode MisterShell JSON value: %w", err)
	}

	value, err := goValueToTerraform(decoded)
	if err != nil {
		return types.DynamicNull(), fmt.Errorf("cannot decode MisterShell JSON value: %w", err)
	}
	if value == nil || value.IsNull() {
		return types.DynamicNull(), nil
	}
	return types.DynamicValue(value), nil
}

// SemanticallyEqual reports whether a native Terraform value and an API JSON
// payload represent the same JSON value, regardless of Terraform collection
// type or insignificant JSON formatting and number representation differences.
func SemanticallyEqual(value types.Dynamic, raw json.RawMessage) (bool, error) {
	terraformJSON, err := ToRawJSON(value)
	if err != nil {
		return false, err
	}
	apiValue, err := FromRawJSON(raw)
	if err != nil {
		return false, err
	}
	apiJSON, err := ToRawJSON(apiValue)
	if err != nil {
		return false, err
	}
	return bytes.Equal(terraformJSON, apiJSON), nil
}

func terraformValueToGo(value attr.Value) (any, error) {
	if value == nil {
		return nil, nil
	}
	if value.IsUnknown() {
		return nil, fmt.Errorf("value contains an unknown element")
	}
	if value.IsNull() {
		return nil, nil
	}

	switch value := value.(type) {
	case types.Dynamic:
		return terraformValueToGo(value.UnderlyingValue())
	case types.String:
		return value.ValueString(), nil
	case types.Bool:
		return value.ValueBool(), nil
	case types.Number:
		number := value.ValueBigFloat()
		if number.IsInt() {
			return json.Number(number.Text('f', 0)), nil
		}
		return json.Number(number.Text('g', -1)), nil
	case types.Int64:
		return value.ValueInt64(), nil
	case types.Int32:
		return value.ValueInt32(), nil
	case types.Float64:
		return value.ValueFloat64(), nil
	case types.Float32:
		return value.ValueFloat32(), nil
	case types.Object:
		result := make(map[string]any, len(value.Attributes()))
		for key, element := range value.Attributes() {
			converted, err := terraformValueToGo(element)
			if err != nil {
				return nil, fmt.Errorf("attribute %q: %w", key, err)
			}
			result[key] = converted
		}
		return result, nil
	case types.Map:
		result := make(map[string]any, len(value.Elements()))
		for key, element := range value.Elements() {
			converted, err := terraformValueToGo(element)
			if err != nil {
				return nil, fmt.Errorf("element %q: %w", key, err)
			}
			result[key] = converted
		}
		return result, nil
	case types.Tuple:
		return terraformElementsToGo(value.Elements())
	case types.List:
		return terraformElementsToGo(value.Elements())
	case types.Set:
		return terraformElementsToGo(value.Elements())
	default:
		return nil, fmt.Errorf("unsupported Terraform value type %T", value)
	}
}

func terraformElementsToGo(elements []attr.Value) ([]any, error) {
	result := make([]any, len(elements))
	for index, element := range elements {
		converted, err := terraformValueToGo(element)
		if err != nil {
			return nil, fmt.Errorf("element %d: %w", index, err)
		}
		result[index] = converted
	}
	return result, nil
}

func goValueToTerraform(value any) (attr.Value, error) {
	switch value := value.(type) {
	case nil:
		return types.DynamicNull(), nil
	case string:
		return types.StringValue(value), nil
	case bool:
		return types.BoolValue(value), nil
	case json.Number:
		number, _, err := big.ParseFloat(string(value), 10, 512, big.ToNearestEven)
		if err != nil {
			return nil, fmt.Errorf("invalid JSON number %q: %w", value, err)
		}
		return types.NumberValue(number), nil
	case map[string]any:
		attributeTypes := make(map[string]attr.Type, len(value))
		attributes := make(map[string]attr.Value, len(value))
		for key, element := range value {
			converted, err := goValueToTerraform(element)
			if err != nil {
				return nil, fmt.Errorf("attribute %q: %w", key, err)
			}
			attributeTypes[key] = converted.Type(context.Background())
			attributes[key] = converted
		}
		return types.ObjectValueMust(attributeTypes, attributes), nil
	case []any:
		elementTypes := make([]attr.Type, len(value))
		elements := make([]attr.Value, len(value))
		for index, element := range value {
			converted, err := goValueToTerraform(element)
			if err != nil {
				return nil, fmt.Errorf("element %d: %w", index, err)
			}
			elementTypes[index] = converted.Type(context.Background())
			elements[index] = converted
		}
		return types.TupleValueMust(elementTypes, elements), nil
	default:
		return nil, fmt.Errorf("unsupported decoded JSON value type %T", value)
	}
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); err == io.EOF {
		return nil
	} else if err != nil {
		return err
	}
	return fmt.Errorf("unexpected trailing JSON value")
}
