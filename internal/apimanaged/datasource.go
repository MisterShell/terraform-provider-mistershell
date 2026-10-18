package apimanaged

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
)

type ManagedDataSource struct {
	definition Definition
	client     *client.Client
}

func NewDataSource(d Definition) datasource.DataSource { return &ManagedDataSource{definition: d} }
func (d *ManagedDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + d.definition.Name
}
func (d *ManagedDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{Description: "Reads a MisterShell " + d.definition.Name + " by ID or exact name.", Attributes: dataFields(d.definition)}
}
func (d *ManagedDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	var ok bool
	d.client, ok = req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Invalid provider configuration", "Expected MisterShell API client.")
	}
}
func (d *ManagedDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var id types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("id"), &id)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var name types.String
	_, hasName := dataFields(d.definition)["name"]
	if hasName {
		resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("name"), &name)...)
	}
	r := ManagedResource{definition: d.definition, client: d.client}
	var raw json.RawMessage
	var err error
	if !id.IsNull() && !id.IsUnknown() {
		raw, err = r.read(ctx, id.ValueString())
	} else if hasName && !name.IsNull() && !name.IsUnknown() {
		var rows []json.RawMessage
		rows, err = d.client.ConfigurationList(ctx, d.definition.ListPath, nil)
		if err == nil {
			matches := 0
			for _, row := range rows {
				o, e := object(row)
				if e != nil {
					err = e
					break
				}
				var n string
				_ = json.Unmarshal(o["name"], &n)
				if n == name.ValueString() {
					matches++
					id = types.StringValue(idString(o["id"]))
				}
			}
			if matches != 1 {
				err = fmt.Errorf("expected one exact-name match, found %d; use an ID to disambiguate", matches)
			} else {
				raw, err = r.read(ctx, id.ValueString())
			}
		}
	} else {
		resp.Diagnostics.AddError("Missing lookup identity", "Specify an object ID or an exact name. Resource variables require resource_id:stack_id.")
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading "+d.definition.Name, err.Error())
		return
	}
	if !name.IsNull() && !name.IsUnknown() {
		o, e := object(raw)
		if e != nil {
			resp.Diagnostics.AddError("Invalid API response", e.Error())
			return
		}
		var actual string
		_ = json.Unmarshal(o["name"], &actual)
		if actual != name.ValueString() {
			resp.Diagnostics.AddError("Lookup mismatch", "ID and name do not identify the same object.")
			return
		}
	}
	resp.State = tfsdk.State{Schema: req.Config.Schema, Raw: req.Config.Raw}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
	resp.Diagnostics.Append(refresh(ctx, d.definition, &resp.State, raw, false)...)
}
