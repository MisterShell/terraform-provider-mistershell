package canaryextras

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"strconv"
	"terraform-provider-mistershell/internal/client"
)

type ServiceOverrideResource struct{ client *client.Client }
type ServiceOverrideModel struct {
	ID              types.String `tfsdk:"id"`
	ServiceID       types.Int64  `tfsdk:"service_id"`
	Enabled         types.Bool   `tfsdk:"enabled"`
	OriginalEnabled types.Bool   `tfsdk:"original_enabled"`
}

func NewCanaryServiceOverrideResource() resource.Resource { return &ServiceOverrideResource{} }
func (r *ServiceOverrideResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_canary_service_override"
}
func (r *ServiceOverrideResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{Description: "Manages only the enabled flag of an existing built-in Canary service. Destroy restores the enabled flag captured at create or import; it never deletes the built-in service. Requires MisterShell 2.1 or newer.", Attributes: map[string]schema.Attribute{"id": schema.StringAttribute{Computed: true, Description: "Built-in service ID; also the import identity."}, "service_id": schema.Int64Attribute{Required: true, Description: "Existing built-in service ID from the canary_service data source.", PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()}}, "enabled": schema.BoolAttribute{Required: true, Description: "Whether the built-in service is enabled."}, "original_enabled": schema.BoolAttribute{Computed: true, Description: "Enabled flag captured at create or import, restored on destroy.", PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}}}}
}
func (r *ServiceOverrideResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	var ok bool
	r.client, ok = req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Invalid provider data", "Expected MisterShell API client.")
	}
}
func (r *ServiceOverrideResource) read(ctx context.Context, id int64) (bool, error) {
	if id <= 0 {
		return false, fmt.Errorf("service_id must be positive")
	}
	raw, err := r.client.ConfigurationRequest(ctx, "GET", fmt.Sprintf("/api/v1/canaries/services/%d", id), nil)
	if err != nil {
		return false, err
	}
	var obj struct {
		ID      int64   `json:"id"`
		SeedKey *string `json:"seed_key"`
		Enabled *bool   `json:"enabled"`
	}
	if err = json.Unmarshal(raw, &obj); err != nil {
		return false, err
	}
	if obj.ID != id || obj.SeedKey == nil || obj.Enabled == nil {
		return false, fmt.Errorf("override requires an existing built-in Canary service with readable enabled state; use canary_service for custom services")
	}
	return *obj.Enabled, nil
}
func (r *ServiceOverrideResource) apply(ctx context.Context, m ServiceOverrideModel) error {
	if m.ServiceID.IsUnknown() || m.ServiceID.IsNull() || m.Enabled.IsUnknown() || m.Enabled.IsNull() {
		return fmt.Errorf("service_id and enabled must be known")
	}
	id := m.ServiceID.ValueInt64()
	if _, err := r.read(ctx, id); err != nil {
		return err
	}
	_, err := r.client.ConfigurationRequest(ctx, "PUT", fmt.Sprintf("/api/v1/canaries/services/%d", id), map[string]bool{"enabled": m.Enabled.ValueBool()})
	if err != nil {
		return err
	}
	enabled, err := r.read(ctx, id)
	if err != nil {
		return err
	}
	if enabled != m.Enabled.ValueBool() {
		return fmt.Errorf("enabled state changed concurrently; refresh before retrying")
	}
	return nil
}
func (r *ServiceOverrideResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m ServiceOverrideModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	baseline, err := r.read(ctx, m.ServiceID.ValueInt64())
	if err != nil {
		resp.Diagnostics.AddError("Cannot read original Canary service", err.Error())
		return
	}
	m.OriginalEnabled = types.BoolValue(baseline)
	m.ID = types.StringValue(strconv.FormatInt(m.ServiceID.ValueInt64(), 10))
	// Persist the restoration baseline before a write: a failed verification GET
	// must not strand a changed built-in flag without its original value.
	resp.Diagnostics.Append(resp.State.Set(ctx, m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.apply(ctx, m); err != nil {
		resp.Diagnostics.AddError("Cannot override Canary service", err.Error())
		return
	}
	m.ID = types.StringValue(strconv.FormatInt(m.ServiceID.ValueInt64(), 10))
	resp.Diagnostics.Append(resp.State.Set(ctx, m)...)
}
func (r *ServiceOverrideResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var m ServiceOverrideModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.apply(ctx, m); err != nil {
		resp.Diagnostics.AddError("Cannot override Canary service", err.Error())
		return
	}
	m.ID = types.StringValue(strconv.FormatInt(m.ServiceID.ValueInt64(), 10))
	resp.Diagnostics.Append(resp.State.Set(ctx, m)...)
}
func (r *ServiceOverrideResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var m ServiceOverrideModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	enabled, err := r.read(ctx, m.ServiceID.ValueInt64())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Cannot read Canary service override", err.Error())
		return
	}
	m.Enabled = types.BoolValue(enabled)
	resp.Diagnostics.Append(resp.State.Set(ctx, m)...)
}
func (r *ServiceOverrideResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var m ServiceOverrideModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if m.OriginalEnabled.IsNull() || m.OriginalEnabled.IsUnknown() {
		resp.Diagnostics.AddError("Cannot restore Canary service", "Original enabled flag is unavailable. Import the override to establish its baseline before destroying.")
		return
	}
	m.Enabled = m.OriginalEnabled
	if err := r.apply(ctx, m); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Cannot restore Canary service", err.Error())
	}
}
func (r *ServiceOverrideResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil || id <= 0 {
		resp.Diagnostics.AddError("Invalid import identity", "Expected a positive built-in service ID.")
		return
	}
	baseline, err := r.read(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Cannot import Canary service override", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("original_enabled"), types.BoolValue(baseline))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("enabled"), types.BoolValue(baseline))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), types.StringValue(req.ID))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("service_id"), types.Int64Value(id))...)
}
