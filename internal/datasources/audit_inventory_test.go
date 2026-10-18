package datasources

import (
	"context"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"testing"
)

func TestAuditInventoryHealthSchema(t *testing.T) {
	var resp datasource.SchemaResponse
	(&NetworkResourceDataSource{}).Schema(context.Background(), datasource.SchemaRequest{}, &resp)
	for _, name := range []string{"health_status", "last_health_at"} {
		if resp.Schema.Attributes[name] == nil {
			t.Errorf("missing health observation %s", name)
		}
	}
}
