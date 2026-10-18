package apimanaged

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"net/url"
	"strconv"
	"strings"
	"terraform-provider-mistershell/internal/client"
)

type ManagedResource struct {
	definition Definition
	client     *client.Client
}

func NewResource(d Definition) resource.Resource { return &ManagedResource{definition: d} }
func (r *ManagedResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + r.definition.Name
}
func (r *ManagedResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{Description: r.definition.Description, Attributes: resourceFields(r.definition)}
}
func (r *ManagedResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	var ok bool
	r.client, ok = req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Invalid provider configuration", "Expected MisterShell API client.")
	}
}
func (r *ManagedResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	if r.definition.Name != "ca_certificate" {
		return
	}
	resp.Diagnostics.Append(validateCACertificatePEM(ctx, req.Config, true)...)
}
func (r *ManagedResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	d := r.definition
	if d.Name == "ca_certificate" {
		resp.Diagnostics.Append(validateCACertificatePEM(ctx, req.Plan, false)...)
	}
	resp.Diagnostics.Append(r.validateSourceParameters(ctx, req.Plan, nil)...)
	body, diags := payload(ctx, d, req.Plan, nil, true)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.guardCreate(ctx, body); err != nil {
		resp.Diagnostics.AddError("Cannot create "+d.Name, err.Error())
		return
	}
	raw, err := r.client.ConfigurationRequest(ctx, d.Method, d.Path, body)
	if err != nil {
		resp.Diagnostics.AddError("Error creating "+d.Name, err.Error())
		return
	}
	obj, err := object(raw)
	if err != nil {
		resp.Diagnostics.AddError("Invalid create response", err.Error())
		return
	}
	if d.Name == "ca_certificate" {
		raw, err = r.singleCreatedCertificate(ctx, obj)
		if err != nil {
			resp.Diagnostics.AddError("Certificate was not created", err.Error())
			return
		}
		obj, _ = object(raw)
	}
	id := idString(obj["id"])
	if d.Name == "configure_resource_vars" {
		id = idString(body["resource_id"]) + ":" + idString(body["stack_id"])
	}
	if id == "" || id == "null" {
		resp.Diagnostics.AddError("Invalid create response", "API did not return an object identity.")
		return
	}
	resp.State = tfsdk.State{Schema: req.Plan.Schema, Raw: req.Plan.Raw}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), types.StringValue(id))...)
	// Preserve one-time enrollment tokens before the detail GET omits them.
	if tok, ok := obj["token"]; ok {
		var v string
		_ = json.Unmarshal(tok, &v)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("token"), types.StringValue(v))...)
	}
	extra := map[string]json.RawMessage{}
	for _, f := range d.Fields {
		if f.Update && !f.Create {
			v, dd := get(ctx, req.Plan, f)
			resp.Diagnostics.Append(dd...)
			if !v.IsUnknown() && !v.IsNull() {
				extra[f.Name], err = rawValue(v)
				if err != nil {
					resp.Diagnostics.AddError("Invalid configuration", err.Error())
					return
				}
			}
		}
	}
	if len(extra) > 0 {
		if err = r.mutate(ctx, id, extra); err != nil {
			resp.Diagnostics.AddError("Error configuring "+d.Name, err.Error())
			return
		}
	}
	raw, err = r.read(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Error reading created "+d.Name, err.Error())
		return
	}
	resp.Diagnostics.Append(refresh(ctx, d, &resp.State, raw, true)...)
}
func (r *ManagedResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var id types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("id"), &id)...)
	if resp.Diagnostics.HasError() {
		return
	}
	raw, err := r.read(ctx, id.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading "+r.definition.Name, err.Error())
		return
	}
	resp.State = req.State
	resp.Diagnostics.Append(refresh(ctx, r.definition, &resp.State, raw, false)...)
}
func (r *ManagedResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	d := r.definition
	if d.Name == "ca_certificate" {
		resp.Diagnostics.Append(validateCACertificatePEM(ctx, req.Plan, false)...)
	}
	resp.Diagnostics.Append(r.validateSourceParameters(ctx, req.Plan, req.State)...)
	var id types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("id"), &id)...)
	body, dd := payload(ctx, d, req.Plan, req.State, false)
	resp.Diagnostics.Append(dd...)
	if resp.Diagnostics.HasError() {
		return
	}
	if len(body) > 0 {
		if err := r.guardCustomRuleMutation(ctx); err != nil {
			resp.Diagnostics.AddError("Cannot update "+d.Name, err.Error())
			return
		}
		if err := r.mutate(ctx, id.ValueString(), body); err != nil {
			resp.Diagnostics.AddError("Error updating "+d.Name, err.Error())
			return
		}
	}
	resp.State = tfsdk.State{Schema: req.Plan.Schema, Raw: req.Plan.Raw}
	if d.Name == "sensor" || d.Name == "proxy" {
		var token types.String
		resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("token"), &token)...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("token"), token)...)
	}
	raw, err := r.read(ctx, id.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading updated "+d.Name, err.Error())
		return
	}
	resp.Diagnostics.Append(refresh(ctx, d, &resp.State, raw, true)...)
}
func (r *ManagedResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var id types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("id"), &id)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.guardCustomRuleMutation(ctx); err != nil {
		resp.Diagnostics.AddError("Cannot delete "+r.definition.Name, err.Error())
		return
	}
	var err error
	if r.definition.Name == "configure_resource_vars" {
		a, b, e := pairID(id.ValueString())
		if e != nil {
			resp.Diagnostics.AddError("Invalid identity", e.Error())
			return
		}
		_, err = r.client.ConfigurationRequest(ctx, "PUT", r.definition.Path, map[string]any{"resource_id": a, "stack_id": b, "values": map[string]any{}})
	} else {
		_, err = r.client.ConfigurationRequest(ctx, "DELETE", itemPath(r.definition, id.ValueString()), nil)
	}
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting "+r.definition.Name, err.Error())
	}
}
func (r *ManagedResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if r.definition.Name == "configure_resource_vars" {
		if _, _, err := pairID(req.ID); err != nil {
			resp.Diagnostics.AddError("Invalid import ID", err.Error())
			return
		}
	} else {
		n, e := strconv.ParseInt(req.ID, 10, 64)
		if e != nil || n <= 0 {
			resp.Diagnostics.AddError("Invalid import ID", "Expected a positive integer object ID.")
			return
		}
	}
	if err := r.validateImportOwnership(ctx, req.ID); err != nil {
		resp.Diagnostics.AddError("Cannot import "+r.definition.Name, err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), types.StringValue(req.ID))...)
}
func itemPath(d Definition, id string) string {
	return strings.ReplaceAll(d.ItemPath, "{id}", url.PathEscape(id))
}
func pairID(id string) (int64, int64, error) {
	p := strings.Split(id, ":")
	if len(p) == 2 {
		a, e := strconv.ParseInt(p[0], 10, 64)
		b, e2 := strconv.ParseInt(p[1], 10, 64)
		if e == nil && e2 == nil && a > 0 && b > 0 {
			return a, b, nil
		}
	}
	return 0, 0, fmt.Errorf("expected positive resource_id:stack_id")
}
func (r *ManagedResource) read(ctx context.Context, id string) (json.RawMessage, error) {
	d := r.definition
	if d.Name == "configure_resource_vars" {
		a, b, e := pairID(id)
		if e != nil {
			return nil, e
		}
		return r.client.ConfigurationRequest(ctx, "GET", d.Path+fmt.Sprintf("?resource_id=%d&stack_id=%d", a, b), nil)
	}
	if d.ListOnly {
		rows, e := r.client.ConfigurationList(ctx, d.ListPath, nil)
		if e != nil {
			return nil, e
		}
		for _, raw := range rows {
			o, e := object(raw)
			if e != nil {
				return nil, e
			}
			if idString(o["id"]) == id {
				return raw, nil
			}
		}
		return nil, &client.NotFoundError{Path: itemPath(d, id)}
	}
	return r.client.ConfigurationRequest(ctx, "GET", itemPath(d, id), nil)
}
func (r *ManagedResource) mutate(ctx context.Context, id string, body map[string]json.RawMessage) error {
	d := r.definition
	if d.Name == "sensor_source" {
		if raw, ok := body["enabled"]; ok {
			var enabled bool
			if err := json.Unmarshal(raw, &enabled); err != nil {
				return err
			}
			verb := "disable"
			if enabled {
				verb = "enable"
			}
			if _, err := r.client.ConfigurationRequest(ctx, "POST", itemPath(d, id)+"/"+verb, nil); err != nil {
				return err
			}
		}
		if raw, ok := body["parameters"]; ok {
			_, err := r.client.ConfigurationRequest(ctx, "PATCH", itemPath(d, id)+"/params", map[string]json.RawMessage{"parameters": raw})
			return err
		}
		return nil
	}
	_, err := r.client.ConfigurationRequest(ctx, d.UpdateMethod, itemPath(d, id), body)
	return err
}
