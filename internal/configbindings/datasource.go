package configbindings

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"terraform-provider-mistershell/internal/client"
	"time"
)

type bindingDataSource struct {
	def    definition
	client *client.Client
}

func (d *bindingDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + d.def.name
}
func (d *bindingDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	fields := map[string]schema.Attribute{"id": schema.StringAttribute{Required: d.def.lookup == "", Optional: d.def.lookup != "", Computed: d.def.lookup != "", Description: "Existing numeric ID, stable configuration key, or local."}, "configuration": schema.DynamicAttribute{Computed: true, Sensitive: true, Description: fieldDescription(d.def) + " Masked secrets are returned as null."}}
	if d.def.lookup != "" {
		fields[d.def.lookup] = schema.StringAttribute{Optional: true, Computed: true, Description: "Exact unique lookup instead of numeric id. When both are supplied they must identify the same object."}
	}
	resp.Schema = schema.Schema{Description: "Reads existing backend-owned configuration without changing it.", Attributes: fields}
}
func (d *bindingDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	var ok bool
	d.client, ok = req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Invalid provider configuration", "Expected MisterShell API client.")
	}
}
func (d *bindingDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var id, name types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("id"), &id)...)
	if d.def.lookup != "" {
		resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root(d.def.lookup), &name)...)
	}
	if resp.Diagnostics.HasError() {
		return
	}
	if id.IsNull() || id.IsUnknown() {
		if name.IsNull() || name.IsUnknown() || name.ValueString() == "" {
			resp.Diagnostics.AddError("Missing lookup identity", "Provide id or an exact lookup name.")
			return
		}
		rows, err := d.client.ConfigurationList(ctx, d.def.path, nil)
		if err != nil {
			resp.Diagnostics.AddError("Error looking up configuration", err.Error())
			return
		}
		matches := 0
		for _, row := range rows {
			obj, err := decode(row)
			if err != nil {
				resp.Diagnostics.AddError("Invalid configuration response", err.Error())
				return
			}
			if textValue(obj[d.def.lookup]) == name.ValueString() {
				matches++
				value := textValue(obj["id"])
				if value == "" {
					value = string(obj["id"])
				}
				id = types.StringValue(value)
			}
		}
		if matches != 1 {
			resp.Diagnostics.AddError("Ambiguous configuration lookup", fmt.Sprintf("Expected one exact match; found %d. Use id to disambiguate.", matches))
			return
		}
	}
	current, err := (engine{api: d.client, def: d.def}).read(ctx, id.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading configuration", err.Error())
		return
	}
	if d.def.lookup != "" && !name.IsNull() && !name.IsUnknown() && textValue(current[d.def.lookup]) != name.ValueString() {
		resp.Diagnostics.AddError("Lookup mismatch", "ID and lookup name identify different objects.")
		return
	}
	resp.State = tfsdk.State{Schema: req.Config.Schema, Raw: req.Config.Raw}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
	if d.def.lookup != "" {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(d.def.lookup), types.StringValue(textValue(current[d.def.lookup])))...)
	}
	resp.Diagnostics.Append(refresh(ctx, d.def, &resp.State, current)...)
}
func equivalentTime(a, b json.RawMessage) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	x, e1 := time.Parse(time.RFC3339Nano, textValue(a))
	y, e2 := time.Parse(time.RFC3339Nano, textValue(b))
	return e1 == nil && e2 == nil && x.Equal(y)
}
