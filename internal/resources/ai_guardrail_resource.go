package resources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-mistershell/internal/client"
)

type AIGuardrailResource struct{ client *client.Client }
type AIGuardrailResourceModel struct {
	OriginalAllowlistEnabled types.Bool   `tfsdk:"original_allowlist_enabled"`
	ID                       types.String `tfsdk:"id"`
	ResourceType             types.String `tfsdk:"resource_type"`
	AllowlistEnabled         types.Bool   `tfsdk:"allowlist_enabled"`
	DisplayName              types.String `tfsdk:"display_name"`
	Description              types.String `tfsdk:"description"`
}

func NewAIGuardrailResource() resource.Resource { return &AIGuardrailResource{} }
func (r *AIGuardrailResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ai_guardrail"
}
func (r *AIGuardrailResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{Description: "Manages an AI command allowlist toggle for one registered resource type. Destroy restores the setting captured at adoption or import.", Attributes: map[string]schema.Attribute{
		"id":                         schema.StringAttribute{Computed: true},
		"resource_type":              schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
		"allowlist_enabled":          schema.BoolAttribute{Required: true},
		"original_allowlist_enabled": schema.BoolAttribute{Computed: true, PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}, Description: "Setting captured at adoption/import and restored on destroy."},
		"display_name":               schema.StringAttribute{Computed: true},
		"description":                schema.StringAttribute{Computed: true},
	}}
}
func (r *AIGuardrailResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", "Expected *client.Client")
		return
	}
	r.client = c
}
func (r *AIGuardrailResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan AIGuardrailResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rows, err := r.client.ListAIGuardrails(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading original AI guardrail", err.Error())
		return
	}
	found := false
	for _, row := range rows {
		if row.ResourceType == plan.ResourceType.ValueString() {
			plan.OriginalAllowlistEnabled = types.BoolValue(row.AllowlistEnabled)
			found = true
			break
		}
	}
	if !found {
		resp.Diagnostics.AddError("AI guardrail not found", "The requested resource type has no registered guardrail.")
		return
	}
	row, err := r.client.SetAIGuardrail(ctx, plan.ResourceType.ValueString(), plan.AllowlistEnabled.ValueBool())
	if err != nil {
		resp.Diagnostics.AddError("Error setting AI guardrail", err.Error())
		return
	}
	mapAIGuardrail(row, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}
func (r *AIGuardrailResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state AIGuardrailResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	plan.OriginalAllowlistEnabled = state.OriginalAllowlistEnabled
	if resp.Diagnostics.HasError() {
		return
	}
	row, err := r.client.SetAIGuardrail(ctx, plan.ResourceType.ValueString(), plan.AllowlistEnabled.ValueBool())
	if err != nil {
		resp.Diagnostics.AddError("Error setting AI guardrail", err.Error())
		return
	}
	mapAIGuardrail(row, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}
func (r *AIGuardrailResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state AIGuardrailResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rows, err := r.client.ListAIGuardrails(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading AI guardrail", err.Error())
		return
	}
	for _, row := range rows {
		if row.ResourceType == state.ResourceType.ValueString() {
			if state.OriginalAllowlistEnabled.IsNull() || state.OriginalAllowlistEnabled.IsUnknown() {
				state.OriginalAllowlistEnabled = types.BoolValue(row.AllowlistEnabled)
			}
			mapAIGuardrail(&row, &state)
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}
func (r *AIGuardrailResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state AIGuardrailResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if state.OriginalAllowlistEnabled.IsNull() || state.OriginalAllowlistEnabled.IsUnknown() {
		resp.Diagnostics.AddError("Missing restoration baseline", "Refresh or reimport this guardrail before destroying it.")
		return
	}
	var err error
	if state.OriginalAllowlistEnabled.ValueBool() {
		err = r.client.ResetAIGuardrail(ctx, state.ResourceType.ValueString())
	} else {
		_, err = r.client.SetAIGuardrail(ctx, state.ResourceType.ValueString(), false)
	}
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error restoring AI guardrail", err.Error())
	}
}
func (r *AIGuardrailResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprint("Expected a resource type"))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("resource_type"), req.ID)...)
}
func mapAIGuardrail(row *client.AIGuardrailResponse, m *AIGuardrailResourceModel) {
	m.ID = types.StringValue(row.ResourceType)
	m.ResourceType = m.ID
	m.AllowlistEnabled = types.BoolValue(row.AllowlistEnabled)
	m.DisplayName = types.StringValue(row.DisplayName)
	m.Description = types.StringValue(row.Description)
}
