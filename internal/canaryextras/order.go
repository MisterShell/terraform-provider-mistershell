// Package canaryextras implements fixed Canary and Syslog configuration endpoints.
package canaryextras

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"sort"
	"terraform-provider-mistershell/internal/client"
)

type OrderResource struct {
	name, listPath, orderPath string
	client                    *client.Client
}
type OrderModel struct {
	ID         types.String `tfsdk:"id"`
	OrderedIDs types.List   `tfsdk:"ordered_ids"`
}

func NewCanaryRuleOrderResource() resource.Resource {
	return &OrderResource{name: "canary_rule_order", listPath: "/api/v1/canaries/rules", orderPath: "/api/v1/canaries/rules/reorder"}
}
func NewSyslogRuleOrderResource() resource.Resource {
	return &OrderResource{name: "syslog_rule_order", listPath: "/api/v1/collector/policy", orderPath: "/api/v1/collector/policy/order"}
}
func (r *OrderResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + r.name
}
func (r *OrderResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{Description: "Owns the complete policy evaluation order. Use one resource per policy family. Destroy forgets ordering without changing or deleting rules. Requires MisterShell 2.1 or newer.", Attributes: map[string]schema.Attribute{"id": schema.StringAttribute{Computed: true, Description: "Singleton identity: policy."}, "ordered_ids": schema.ListAttribute{Required: true, ElementType: types.Int64Type, Description: "Complete ordered list of existing rule IDs, including rules managed outside Terraform. Duplicates or incomplete sets are rejected before writing."}}}
}
func (r *OrderResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	var ok bool
	r.client, ok = req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Invalid provider data", "Expected MisterShell API client.")
	}
}
func (r *OrderResource) current(ctx context.Context) ([]int64, error) {
	rows, err := r.client.ConfigurationList(ctx, r.listPath, nil)
	if err != nil {
		return nil, err
	}
	type row struct {
		ID       int64 `json:"id"`
		Position int64 `json:"position"`
	}
	parsed := make([]row, 0, len(rows))
	seen := map[int64]bool{}
	for _, raw := range rows {
		var v row
		if err = json.Unmarshal(raw, &v); err != nil {
			return nil, err
		}
		if v.ID <= 0 || seen[v.ID] {
			return nil, fmt.Errorf("invalid or duplicate rule identity in API response")
		}
		seen[v.ID] = true
		parsed = append(parsed, v)
	}
	sort.SliceStable(parsed, func(i, j int) bool { return parsed[i].Position < parsed[j].Position })
	ids := make([]int64, len(parsed))
	for i, v := range parsed {
		ids[i] = v.ID
	}
	return ids, nil
}
func validateOrder(planned, current []int64) error {
	if len(planned) != len(current) {
		return fmt.Errorf("ordering must include every existing rule exactly once; refresh and include all %d rules", len(current))
	}
	remaining := map[int64]bool{}
	for _, id := range current {
		remaining[id] = true
	}
	for _, id := range planned {
		if id <= 0 || !remaining[id] {
			return fmt.Errorf("ordering contains an unknown or repeated rule ID %d; refresh before retrying", id)
		}
		delete(remaining, id)
	}
	return nil
}
func (r *OrderResource) apply(ctx context.Context, model OrderModel) error {
	var ids []int64
	if d := model.OrderedIDs.ElementsAs(ctx, &ids, false); d.HasError() {
		return fmt.Errorf("ordered_ids must contain known integer IDs: %v", d)
	}
	current, err := r.current(ctx)
	if err != nil {
		return err
	}
	if err = validateOrder(ids, current); err != nil {
		return err
	}
	if _, err = r.client.ConfigurationRequest(ctx, "PUT", r.orderPath, map[string]any{"ordered_ids": ids}); err != nil {
		return err
	}
	actual, err := r.current(ctx)
	if err != nil {
		return err
	}
	if len(actual) != len(ids) {
		return fmt.Errorf("policy changed concurrently; refresh ordering before retrying")
	}
	for i, id := range ids {
		if actual[i] != id {
			return fmt.Errorf("policy changed concurrently; refresh ordering before retrying")
		}
	}
	return nil
}
func (r *OrderResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m OrderModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.apply(ctx, m); err != nil {
		resp.Diagnostics.AddError("Cannot order policy", err.Error())
		return
	}
	m.ID = types.StringValue("policy")
	resp.Diagnostics.Append(resp.State.Set(ctx, m)...)
}
func (r *OrderResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var m OrderModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.apply(ctx, m); err != nil {
		resp.Diagnostics.AddError("Cannot order policy", err.Error())
		return
	}
	m.ID = types.StringValue("policy")
	resp.Diagnostics.Append(resp.State.Set(ctx, m)...)
}
func (r *OrderResource) Read(ctx context.Context, _ resource.ReadRequest, resp *resource.ReadResponse) {
	ids, err := r.current(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Cannot read policy order", err.Error())
		return
	}
	v, d := types.ListValueFrom(ctx, types.Int64Type, ids)
	resp.Diagnostics.Append(d...)
	resp.Diagnostics.Append(resp.State.Set(ctx, OrderModel{ID: types.StringValue("policy"), OrderedIDs: v})...)
}
func (r *OrderResource) Delete(context.Context, resource.DeleteRequest, *resource.DeleteResponse) {}
func (r *OrderResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID != "policy" {
		resp.Diagnostics.AddError("Invalid import identity", "Import the singleton using policy.")
		return
	}
	ids, err := r.current(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Cannot import policy order", err.Error())
		return
	}
	v, d := types.ListValueFrom(ctx, types.Int64Type, ids)
	resp.Diagnostics.Append(d...)
	resp.Diagnostics.Append(resp.State.Set(ctx, OrderModel{ID: types.StringValue("policy"), OrderedIDs: v})...)
}
