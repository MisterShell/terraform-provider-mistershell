package datasources

import (
	"testing"

	"terraform-provider-mistershell/internal/client"
)

func TestFilterLocationsExactAppliesNameAndKind(t *testing.T) {
	locations := []client.LocationResponse{
		{ID: 1, Name: "Paris", Kind: "geo"},
		{ID: 2, Name: "Paris", Kind: "org"},
		{ID: 3, Name: "Lyon", Kind: "geo"},
	}
	got := filterLocationsExact(locations, "Paris", "org")
	if len(got) != 1 || got[0].ID != 2 {
		t.Fatalf("filtered locations = %#v", got)
	}
}
