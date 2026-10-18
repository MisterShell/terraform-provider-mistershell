package apimanaged

import (
	"context"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"testing"
)

func TestContractsExposeWritableAndReadableFields(t *testing.T) {
	defs := Definitions()
	if len(defs) < 18 {
		t.Fatalf("missing audited API entities: %d", len(defs))
	}
	for _, d := range defs {
		t.Run(d.Name, func(t *testing.T) {
			r := NewResource(d)
			var rs resource.SchemaResponse
			r.Schema(context.Background(), resource.SchemaRequest{}, &rs)
			ds := NewDataSource(d)
			var dr datasource.SchemaResponse
			ds.Schema(context.Background(), datasource.SchemaRequest{}, &dr)
			for _, f := range d.Fields {
				if _, ok := rs.Schema.Attributes[f.Name]; !ok {
					t.Errorf("resource missing %s", f.Name)
				}
				if _, ok := dr.Schema.Attributes[f.Name]; !ok {
					t.Errorf("data source missing %s", f.Name)
				}
			}
			if _, ok := rs.Schema.Attributes["id"]; !ok {
				t.Error("no import identity")
			}
		})
	}
}
