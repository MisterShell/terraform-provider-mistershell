package datasources

import (
	"testing"

	"terraform-provider-mistershell/internal/client"
)

func TestMapNetworkResourceCoordinatesToDataSourceModel(t *testing.T) {
	sourceID := int64(7)
	sourceName := "Paris"
	model := NetworkResourceDataSourceModel{}
	coordinates := &client.EffectiveCoordinates{
		Latitude:           48.8566,
		Longitude:          2.3522,
		SourceLocationID:   &sourceID,
		SourceLocationName: &sourceName,
	}

	mapNetworkResourceCoordinatesToDataSourceModel(coordinates, &model)

	if model.EffectiveLatitude.ValueFloat64() != 48.8566 || model.EffectiveLongitude.ValueFloat64() != 2.3522 {
		t.Fatalf("effective coordinates = %v, %v", model.EffectiveLatitude, model.EffectiveLongitude)
	}
	if model.CoordinateSourceID.ValueInt64() != 7 || model.CoordinateSourceName.ValueString() != "Paris" {
		t.Fatalf("coordinate source = %v, %v", model.CoordinateSourceID, model.CoordinateSourceName)
	}

	coordinates.SourceLocationID = nil
	coordinates.SourceLocationName = nil
	mapNetworkResourceCoordinatesToDataSourceModel(coordinates, &model)
	if model.EffectiveLatitude.IsNull() || model.EffectiveLongitude.IsNull() ||
		!model.CoordinateSourceID.IsNull() || !model.CoordinateSourceName.IsNull() {
		t.Fatalf("redacted coordinate source must preserve coordinates only: %#v", model)
	}

	mapNetworkResourceCoordinatesToDataSourceModel(nil, &model)
	if !model.EffectiveLatitude.IsNull() || !model.EffectiveLongitude.IsNull() ||
		!model.CoordinateSourceID.IsNull() || !model.CoordinateSourceName.IsNull() {
		t.Fatalf("missing effective coordinates must map to null: %#v", model)
	}
}
