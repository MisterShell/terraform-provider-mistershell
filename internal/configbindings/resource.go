package configbindings

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"terraform-provider-mistershell/internal/client"
	"terraform-provider-mistershell/internal/tfjson"
)

type bindingResource struct {
	def    definition
	client *client.Client
}

func (r *bindingResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + r.def.name
}
func description(d definition) string {
	return "Binds configuration of an existing backend-owned object. Create captures originals and applies only configured fields; destroy restores those originals and never deletes the object. Removing a field restores and releases it. Import explicitly adopts current readable configuration. One binding per object/configuration surface. " + fieldDescription(d)
}
func fieldDescription(d definition) string {
	switch d.name {
	case "scheduled_task_config":
		return "configuration fields: task_name, is_enabled, interval_minutes (>=1), timeout_seconds (60..86400), config (object or null), next_run (RFC3339 timestamp or null). next_run is a moving scheduler value; configuring it enforces that timestamp."
	case "fact_collection_config":
		return "configuration fields: is_enabled, display_order. ID is the catalog key."
	case "metric_config", "snap_config":
		return "configuration fields: is_enabled, display_order, options (whole override object; {} clears overrides). ID is the catalog key."
	case "metric_customizations":
		return "configuration = { items = [{ tag_id, skip_collection, thresholds }] }. List order is priority; [] clears all rows. Parent must remain enabled for writes and restore. Thresholds must match the metric type; null thresholds cannot be changed to values when that would prevent restoration."
	case "snap_customizations":
		return "configuration = { items = [{ tag_id, skip_collection }] }. List order is priority; [] clears all rows. Parent must remain enabled for writes and restore."
	case "local_auth_policy":
		return "ID must be local. configuration = { mfa_method = none or totp }. Email MFA and adoption of an email baseline require interactive proof and cannot be managed."
	case "builtin_sensor_source":
		return "configuration fields: enabled, parameters (sensitive string object). Built-in sources only. Parameters cannot be introduced or cleared because the API cannot remove them. Changing existing parameters requires restore_parameters with the original plaintext values. Enabling a source that would disable an enabled replacement is rejected."
	}
	return ""
}
func (r *bindingResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	fields := map[string]schema.Attribute{
		"id":                     schema.StringAttribute{Required: true, Description: "Existing object ID, stable catalog key, or local. This binding never creates the backend object.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"configuration":          schema.DynamicAttribute{Required: true, Sensitive: true, Description: fieldDescription(r.def)},
		"original_configuration": schema.DynamicAttribute{Computed: true, Sensitive: true, Description: "Original owned configuration retained in state for restoration. Do not edit; protect Terraform state."},
	}
	if r.def.name == "builtin_sensor_source" {
		fields["restore_parameters"] = schema.DynamicAttribute{Optional: true, Sensitive: true, Description: "Original plaintext for previously configured masked parameters. Required before managing a parameter; never sent as a mask. Captured originals remain fixed for this binding's lifetime."}
	}
	resp.Schema = schema.Schema{Description: description(r.def), Attributes: fields}
}
func (r *bindingResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	var ok bool
	r.client, ok = req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Invalid provider configuration", "Expected MisterShell API client.")
	}
}

type getter interface {
	GetAttribute(context.Context, path.Path, any) diag.Diagnostics
}

func getObject(ctx context.Context, g getter, name string) (object, diag.Diagnostics) {
	var value types.Dynamic
	diags := g.GetAttribute(ctx, path.Root(name), &value)
	if diags.HasError() {
		return nil, diags
	}
	if value.IsNull() {
		return nil, diags
	}
	raw, err := tfjson.ToRawJSON(value)
	if err != nil {
		diags.AddError("Invalid configuration", err.Error())
		return nil, diags
	}
	obj, err := decode(raw)
	if err != nil {
		diags.AddError("Invalid configuration", err.Error())
	}
	return obj, diags
}
func setObject(ctx context.Context, s *tfsdk.State, name string, obj object) diag.Diagnostics {
	value, err := tfjson.FromRawJSON(jsonValue(obj))
	if err != nil {
		var diags diag.Diagnostics
		diags.AddError("Invalid configuration", err.Error())
		return diags
	}
	return s.SetAttribute(ctx, path.Root(name), value)
}
func (r *bindingResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var id types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("id"), &id)...)
	if !id.IsUnknown() && !id.IsNull() {
		if err := validID(r.def, id.ValueString()); err != nil {
			resp.Diagnostics.AddError("Invalid binding identity", err.Error())
		}
	}
	var value types.Dynamic
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("configuration"), &value)...)
	if value.IsNull() || value.IsUnknown() {
		return
	}
	raw, err := tfjson.ToRawJSON(value)
	if err != nil {
		return
	}
	obj, err := decode(raw)
	if err == nil {
		err = validate(r.def, obj)
	}
	if err != nil {
		resp.Diagnostics.AddError("Invalid binding configuration", err.Error())
	}
}
func (r *bindingResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	resp.State = tfsdk.State{Schema: req.Plan.Schema, Raw: tftypes.NewValue(req.Plan.Schema.Type().TerraformType(ctx), nil)}
	r.apply(ctx, req.Plan, nil, &resp.State, &resp.Diagnostics)
}
func (r *bindingResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.State = req.State
	r.apply(ctx, req.Plan, &req.State, &resp.State, &resp.Diagnostics)
}
func (r *bindingResource) apply(ctx context.Context, plan tfsdk.Plan, prior *tfsdk.State, state *tfsdk.State, diags *diag.Diagnostics) {
	var id types.String
	diags.Append(plan.GetAttribute(ctx, path.Root("id"), &id)...)
	desired, dd := getObject(ctx, plan, "configuration")
	diags.Append(dd...)
	var original, restore object
	if prior != nil {
		original, dd = getObject(ctx, *prior, "original_configuration")
		diags.Append(dd...)
	}
	if r.def.name == "builtin_sensor_source" {
		restore, dd = getObject(ctx, plan, "restore_parameters")
		diags.Append(dd...)
	}
	if diags.HasError() {
		return
	}
	e := engine{api: r.client, def: r.def}
	current, err := e.read(ctx, id.ValueString())
	if err != nil {
		diags.AddError("Cannot bind existing configuration", err.Error())
		return
	}
	body, snapshot, err := prepare(r.def, desired, original, current, restore)
	if err != nil {
		diags.AddError("Cannot apply configuration binding", err.Error())
		return
	}
	if err = e.validateRestoration(ctx, id.ValueString(), body, snapshot); err != nil {
		diags.AddError("Cannot apply configuration binding", err.Error())
		return
	}
	*state = tfsdk.State{Schema: plan.Schema, Raw: plan.Raw}
	// Retain recovery information even if an endpoint fails after a partial write.
	recovery := recoverySnapshot(original, snapshot)
	diags.Append(setObject(ctx, state, "original_configuration", recovery)...)
	if diags.HasError() {
		return
	}
	if err = e.write(ctx, id.ValueString(), body); err != nil {
		diags.AddError("Error applying configuration binding", err.Error())
		return
	}
	diags.Append(setObject(ctx, state, "original_configuration", snapshot)...)
	current, err = e.read(ctx, id.ValueString())
	if err != nil {
		diags.AddError("Error reading bound configuration", err.Error())
		return
	}
	diags.Append(refresh(ctx, r.def, state, current)...)
}
func refresh(ctx context.Context, d definition, state *tfsdk.State, current object) diag.Diagnostics {
	var prior types.Dynamic
	diags := state.GetAttribute(ctx, path.Root("configuration"), &prior)
	if diags.HasError() {
		return diags
	}
	filtered := object{}
	for field := range d.fields {
		if raw, ok := current[field]; ok {
			filtered[field] = raw
		}
	}
	var secrets []string
	if raw, ok := filtered["parameters"]; ok {
		var params map[string]any
		_ = json.Unmarshal(raw, &params)
		for k := range params {
			secrets = append(secrets, k)
		}
	}
	// Preserve equivalent timezone spellings used in an explicitly configured tick.
	if d.name == "scheduled_task_config" && !prior.IsNull() && !prior.IsUnknown() {
		raw, _ := tfjson.ToRawJSON(prior)
		old, _ := decode(raw)
		if equivalentTime(old["next_run"], filtered["next_run"]) {
			filtered["next_run"] = old["next_run"]
		}
	}
	value, err := tfjson.Reconcile(prior, jsonValue(filtered), secrets)
	if err != nil {
		diags.AddError("Invalid configuration response", err.Error())
		return diags
	}
	// Options and task config are replaced as whole objects by their API, so
	// added remote keys are drift. Keep selective ownership only at the outer
	// configuration level and within merge-only Sensor parameters.
	wholeField := ""
	if d.name == "metric_config" || d.name == "snap_config" {
		wholeField = "options"
	}
	if d.name == "scheduled_task_config" {
		wholeField = "config"
	}
	if wholeField != "" {
		mergedRaw, conversionErr := tfjson.ToRawJSON(value)
		if conversionErr != nil {
			diags.AddError("Invalid configuration response", conversionErr.Error())
			return diags
		}
		merged, decodeErr := decode(mergedRaw)
		if decodeErr != nil {
			diags.AddError("Invalid configuration response", decodeErr.Error())
			return diags
		}
		if _, owned := merged[wholeField]; owned {
			merged[wholeField] = filtered[wholeField]
			encoded := jsonValue(merged)
			equal, comparisonErr := tfjson.SemanticallyEqual(prior, encoded)
			if comparisonErr != nil && !prior.IsUnknown() {
				diags.AddError("Invalid configuration response", comparisonErr.Error())
				return diags
			}
			if equal {
				value = prior
			} else {
				value, err = tfjson.FromRawJSON(encoded)
			}
			if err != nil {
				diags.AddError("Invalid configuration response", err.Error())
				return diags
			}
		}
	}
	diags.Append(state.SetAttribute(ctx, path.Root("configuration"), value)...)
	return diags
}
func (r *bindingResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var id types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("id"), &id)...)
	if resp.Diagnostics.HasError() {
		return
	}
	current, err := (engine{api: r.client, def: r.def}).read(ctx, id.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading configuration binding", err.Error())
		return
	}
	resp.State = req.State
	resp.Diagnostics.Append(refresh(ctx, r.def, &resp.State, current)...)
}
func (r *bindingResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var id types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("id"), &id)...)
	original, dd := getObject(ctx, req.State, "original_configuration")
	resp.Diagnostics.Append(dd...)
	if resp.Diagnostics.HasError() {
		return
	}
	if len(original) == 0 {
		resp.Diagnostics.AddError("Missing restoration snapshot", "Cannot restore this binding without original_configuration; import again to explicitly adopt current configuration.")
		return
	}
	e := engine{api: r.client, def: r.def}
	if _, err := e.read(ctx, id.ValueString()); client.IsNotFound(err) {
		return
	} else if err != nil {
		resp.Diagnostics.AddError("Error reading configuration for restore", err.Error())
		return
	}
	if err := e.write(ctx, id.ValueString(), restoreBody(original)); err != nil {
		resp.Diagnostics.AddError("Error restoring original configuration", err.Error())
	}
}
func (r *bindingResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	e := engine{api: r.client, def: r.def}
	current, err := e.read(ctx, req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Cannot import configuration binding", err.Error())
		return
	}
	adopted := object{}
	for field := range r.def.fields {
		if raw, ok := current[field]; ok {
			adopted[field] = raw
		}
	}
	// A masked secret cannot become an owned original. Users can explicitly add
	// parameter ownership later with original plaintext in restore_parameters.
	if r.def.name == "builtin_sensor_source" {
		delete(adopted, "parameters")
	}
	if err := validate(r.def, adopted); err != nil {
		resp.Diagnostics.AddError("Cannot adopt current configuration", fmt.Sprintf("%s; configure a binding owning only supported restorable fields instead", err))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), types.StringValue(req.ID))...)
	resp.Diagnostics.Append(setObject(ctx, &resp.State, "configuration", adopted)...)
	resp.Diagnostics.Append(setObject(ctx, &resp.State, "original_configuration", adopted)...)
}

// Successful writes can release nested parameter ownership, but a failed write
// must retain every previous original plus newly acquired parameter originals.
func recoverySnapshot(original, snapshot object) object {
	recovery := object{}
	for k, v := range snapshot {
		recovery[k] = v
	}
	for k, v := range original {
		recovery[k] = v
	}
	if original["parameters"] != nil && snapshot["parameters"] != nil {
		old, _ := decode(original["parameters"])
		next, _ := decode(snapshot["parameters"])
		params := object{}
		for k, v := range next {
			params[k] = v
		}
		for k, v := range old {
			params[k] = v
		}
		recovery["parameters"] = jsonValue(params)
	}
	return recovery
}
