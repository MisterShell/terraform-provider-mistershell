package apimanaged

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"terraform-provider-mistershell/internal/client"
)

// Custom-rule writes implicitly rebuild the global ruleset in automatic mode.
// Refuse that mode rather than publishing changes Terraform does not own.
func (r *ManagedResource) guardCustomRuleMutation(ctx context.Context) error {
	if r.definition.Name != "sensor_custom_rule" {
		return nil
	}
	raw, err := r.client.ConfigurationRequest(ctx, "GET", "/api/v1/sensors/ruleset/staged-changes", nil)
	if err != nil {
		return err
	}
	var status struct {
		Mode string `json:"mode"`
	}
	if err = json.Unmarshal(raw, &status); err != nil {
		return err
	}
	if status.Mode != "manual" {
		return fmt.Errorf("custom rule changes are refused in automatic or unknown ruleset mode because the API publishes all staged changes; select manual mode before applying or destroying this resource")
	}
	return nil
}

func (r *ManagedResource) guardCreate(ctx context.Context, body map[string]json.RawMessage) error {
	if err := r.guardCustomRuleMutation(ctx); err != nil {
		return err
	}
	if r.definition.Name != "configure_resource_vars" {
		return nil
	}
	id := idString(body["resource_id"]) + ":" + idString(body["stack_id"])
	raw, err := r.read(ctx, id)
	if client.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var existing struct {
		Values map[string]json.RawMessage `json:"values"`
	}
	if err = json.Unmarshal(raw, &existing); err != nil {
		return err
	}
	if len(existing.Values) > 0 {
		return fmt.Errorf("resource variables already exist for %s; import them before managing this pair", id)
	}
	return nil
}

func validateSourceParameterChange(prior, planned json.RawMessage) error {
	var old, next map[string]*string
	if len(prior) > 0 {
		if err := json.Unmarshal(prior, &old); err != nil {
			return err
		}
	}
	if len(planned) > 0 {
		if err := json.Unmarshal(planned, &next); err != nil {
			return err
		}
	}
	for key := range old {
		if _, ok := next[key]; !ok {
			return fmt.Errorf("sensor source parameter keys cannot be removed: the API retains omitted secrets; replace the custom source to clear stored parameters")
		}
	}
	for _, value := range next {
		if value == nil || strings.TrimSpace(*value) == "" || *value == "****" || *value == "***" {
			return fmt.Errorf("sensor source parameters must contain non-blank secret values; blank, null and masked values do not clear stored parameters")
		}
	}
	return nil
}

func (r *ManagedResource) validateSourceParameters(ctx context.Context, plan attributeReader, previous attributeReader) diag.Diagnostics {
	var diags diag.Diagnostics
	if r.definition.Name != "sensor_source" {
		return diags
	}
	f := Field{Name: "parameters"}
	value, dd := get(ctx, plan, f)
	diags.Append(dd...)
	if diags.HasError() || value.IsUnknown() {
		return diags
	}
	var prior json.RawMessage
	if previous != nil {
		old, dd := get(ctx, previous, f)
		diags.Append(dd...)
		if diags.HasError() || value.Equal(old) {
			return diags
		}
		var err error
		prior, err = rawValue(old)
		if err != nil {
			diags.AddError("Invalid source parameters", err.Error())
			return diags
		}
	}
	planned, err := rawValue(value)
	if err == nil {
		err = validateSourceParameterChange(prior, planned)
	}
	if err != nil {
		diags.AddError("Cannot clear source parameters", err.Error())
	}
	return diags
}

func (r *ManagedResource) validateImportOwnership(ctx context.Context, id string) error {
	checkOwnership := r.definition.Name == "sensor_source"
	for _, f := range r.definition.Fields {
		checkOwnership = checkOwnership || f.Name == "is_builtin"
	}
	if !checkOwnership {
		return nil
	}
	raw, err := r.read(ctx, id)
	if err != nil {
		return err
	}
	var obj struct {
		IsBuiltin  bool   `json:"is_builtin"`
		SourceType string `json:"source_type"`
	}
	if err = json.Unmarshal(raw, &obj); err != nil {
		return err
	}
	if obj.IsBuiltin || r.definition.Name == "sensor_source" && obj.SourceType != "custom" {
		return fmt.Errorf("backend-owned objects cannot be imported as managed resources; use the matching data source")
	}
	return nil
}

func (r *ManagedResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.definition.Name != "sensor_source" || req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}
	f := Field{Name: "parameters"}
	configured, diags := get(ctx, req.Config, f)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || configured.IsUnknown() {
		return
	}
	if !configured.IsNull() {
		resp.Diagnostics.Append(r.validateSourceParameters(ctx, req.Config, req.State)...)
		return
	}
	prior, diags := get(ctx, req.State, f)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || prior.IsNull() || prior.IsUnknown() {
		return
	}
	raw, err := rawValue(prior)
	var keys map[string]json.RawMessage
	if err == nil {
		err = json.Unmarshal(raw, &keys)
	}
	if err != nil {
		resp.Diagnostics.AddError("Invalid source parameters", err.Error())
		return
	}
	if len(keys) > 0 {
		resp.Diagnostics.AddAttributeError(path.Root("parameters"), "Cannot clear source parameters", "Source parameters cannot be omitted after they have been managed: the API retains stored secrets. Keep the configured parameters or explicitly replace the custom source to clear them.")
	}
}
