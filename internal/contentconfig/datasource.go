package contentconfig

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"strconv"
)

func (d *DataSource) Read(ctx context.Context, q datasource.ReadRequest, p *datasource.ReadResponse) {
	p.State = tfsdk.State{Schema: q.Config.Schema, Raw: q.Config.Raw}
	var b json.RawMessage
	var e error
	field := "metadata"
	switch d.kind {
	case "license":
		b, e = licenseEntry(ctx, d.client, stringValue(ctx, q.Config, "id"))
		if e == nil && b == nil {
			e = fmt.Errorf("license not found")
		}
	case "license_status":
		field = "projection"
		b, e = d.client.ConfigurationRequest(ctx, "GET", "/api/v1/licenses/status", nil)
		p.Diagnostics.Append(set(ctx, &p.State, "id", "license_status")...)
	case "file_catalog":
		field = "entries"
		location := intValue(ctx, q.Config, "location_id")
		parent := stringValue(ctx, q.Config, "parent_path")
		var rows []json.RawMessage
		rows, e = catalogEntries(ctx, d.client, location, parent)
		if rows == nil {
			rows = []json.RawMessage{}
		}
		b = raw(rows)
		p.Diagnostics.Append(set(ctx, &p.State, "id", strconv.FormatInt(location, 10)+":"+parent)...)
	case "file_catalog_folder", "file_catalog_file":
		b, e = catalogEntry(ctx, d.client, intValue(ctx, q.Config, "location_id"), stringValue(ctx, q.Config, "id"))
		if e == nil {
			o, _ := object(b)
			kind := "file"
			if d.kind == "file_catalog_folder" {
				kind = "folder"
			}
			if jsonString(o["kind"]) != kind {
				e = fmt.Errorf("selected catalog entry is not a %s", kind)
			}
		}
	case "tree_element":
		b, e = treeEntry(ctx, d.client, stringValue(ctx, q.Config, "id"))
	case "note":
		scope := stringValue(ctx, q.Config, "scope_type")
		id := intValue(ctx, q.Config, "scope_id")
		var np string
		np, e = notePath(scope, id)
		if e == nil {
			var v int64
			b, v, _, e = latestNote(ctx, d.client, np)
			p.Diagnostics.Append(set(ctx, &p.State, "id", scope+":"+strconv.FormatInt(id, 10))...)
			p.Diagnostics.Append(set(ctx, &p.State, "version", v)...)
			content := types.StringNull()
			if len(b) > 0 {
				o, _ := object(b)
				content = types.StringValue(jsonString(o["content"]))
			}
			p.Diagnostics.Append(set(ctx, &p.State, "content", content)...)
		}
	}
	if e != nil {
		p.Diagnostics.AddError("Error reading "+d.kind, e.Error())
		return
	}
	if d.kind != "note" {
		p.Diagnostics.Append(setDynamic(ctx, &p.State, field, b)...)
	}
}
