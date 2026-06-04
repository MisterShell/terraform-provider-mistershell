package resources

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-mistershell/internal/client"
)

var (
	_ resource.Resource                = &SessionPolicyRuleResource{}
	_ resource.ResourceWithImportState = &SessionPolicyRuleResource{}
)

type SessionPolicyRuleResource struct {
	client *client.Client
}

type SessionPolicyRuleResourceModel struct {
	ID            types.Int64  `tfsdk:"id"`
	Position      types.Int64  `tfsdk:"position"`
	Name          types.String `tfsdk:"name"`
	Comment       types.String `tfsdk:"comment"`
	ResourceTypes types.Set    `tfsdk:"resource_types"`
	SessionTypes  types.Set    `tfsdk:"session_types"`
	LocationIDs   types.Set    `tfsdk:"location_ids"`
	TagIDs        types.Set    `tfsdk:"tag_ids"`
	RoleIDs       types.Set    `tfsdk:"role_ids"`
	CommandAclIDs types.Set    `tfsdk:"command_acl_ids"`
	Action        types.String `tfsdk:"action"`
	Notify        types.Bool   `tfsdk:"notify"`
	Log           types.Bool   `tfsdk:"log"`
	Enabled       types.Bool   `tfsdk:"enabled"`
	HitCount      types.Int64  `tfsdk:"hit_count"`
	LastHitAt     types.String `tfsdk:"last_hit_at"`
	CreatedAt     types.String `tfsdk:"created_at"`
	UpdatedAt     types.String `tfsdk:"updated_at"`
}

func NewSessionPolicyRuleResource() resource.Resource {
	return &SessionPolicyRuleResource{}
}

func (r *SessionPolicyRuleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_session_policy_rule"
}

func (r *SessionPolicyRuleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	emptyStrSet, _ := types.SetValue(types.StringType, []attr.Value{})
	emptyInt64Set, _ := types.SetValue(types.Int64Type, []attr.Value{})
	resp.Schema = schema.Schema{
		Description: "Manages a MisterShell session-policy rule. Rules form an ordered chain evaluated by position (lower runs first). Ordering is declarative: set distinct position values (e.g. 10, 20, 30) for deterministic order. Empty selector sets mean \"Any\". The backend exposes no GET-single endpoint for rules, so reads go through the list endpoint; the last remaining rule cannot be deleted.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Description: "Rule ID.",
				Computed:    true,
				PlanModifiers: []planmodifier.Int64{
					int64UseStateForUnknown(),
				},
			},
			"position": schema.Int64Attribute{
				Description: "1-based evaluation position (lower runs first). Omit to append at the end (server uses max+10). Assign distinct values for deterministic ordering; ties break by id. Set declaratively (the provider never calls the reorder endpoint).",
				Optional:    true,
				Computed:    true,
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
				},
				PlanModifiers: []planmodifier.Int64{
					int64UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Rule name (up to 128 characters). Defaults to empty. Not required to be unique.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
				Validators: []validator.String{
					stringvalidator.LengthAtMost(128),
				},
			},
			"comment": schema.StringAttribute{
				Description: "Optional free-form comment.",
				Optional:    true,
			},
			"resource_types": schema.SetAttribute{
				Description: "Resource types this rule applies to. Empty set means Any. Values are inventory resource-type strings.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				Default:     setdefault.StaticValue(emptyStrSet),
				Validators: []validator.Set{
					setvalidator.ValueStringsAre(stringvalidator.OneOf(client.SupportedResourceTypes...)),
				},
			},
			"session_types": schema.SetAttribute{
				Description: "Session types this rule applies to. Empty set means Any. One of: shell, graphical.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				Default:     setdefault.StaticValue(emptyStrSet),
				Validators: []validator.Set{
					setvalidator.ValueStringsAre(stringvalidator.OneOf(client.SupportedSessionTypes...)),
				},
			},
			"location_ids": schema.SetAttribute{
				Description: "Location IDs this rule applies to. Empty set means Any.",
				Optional:    true,
				Computed:    true,
				ElementType: types.Int64Type,
				Default:     setdefault.StaticValue(emptyInt64Set),
			},
			"tag_ids": schema.SetAttribute{
				Description: "Tag IDs this rule applies to. Empty set means Any.",
				Optional:    true,
				Computed:    true,
				ElementType: types.Int64Type,
				Default:     setdefault.StaticValue(emptyInt64Set),
			},
			"role_ids": schema.SetAttribute{
				Description: "Role IDs this rule applies to. Empty set means Any.",
				Optional:    true,
				Computed:    true,
				ElementType: types.Int64Type,
				Default:     setdefault.StaticValue(emptyInt64Set),
			},
			"command_acl_ids": schema.SetAttribute{
				Description: "Command ACL IDs referenced by this rule. Empty set means Any.",
				Optional:    true,
				Computed:    true,
				ElementType: types.Int64Type,
				Default:     setdefault.StaticValue(emptyInt64Set),
			},
			"action": schema.StringAttribute{
				Description: "Rule action. One of: accept, deny. Defaults to accept.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("accept"),
				Validators: []validator.String{
					stringvalidator.OneOf(client.SupportedPolicyRuleActions...),
				},
			},
			"notify": schema.BoolAttribute{
				Description: "Whether to notify on match. Defaults to false.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"log": schema.BoolAttribute{
				Description: "Whether to log on match. Defaults to false.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"enabled": schema.BoolAttribute{
				Description: "Whether the rule is enabled. Defaults to true.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
			},
			"hit_count": schema.Int64Attribute{
				Description: "Number of times this rule has matched (runtime telemetry, read-only).",
				Computed:    true,
			},
			"last_hit_at": schema.StringAttribute{
				Description: "Timestamp of the last match (runtime telemetry, read-only).",
				Computed:    true,
			},
			"created_at": schema.StringAttribute{
				Description: "Creation timestamp.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"updated_at": schema.StringAttribute{
				Description: "Last update timestamp.",
				Computed:    true,
			},
		},
	}
}

func (r *SessionPolicyRuleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Resource Configure Type", "Expected *client.Client")
		return
	}
	r.client = c
}

func (r *SessionPolicyRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan SessionPolicyRuleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	input := client.RuleCreateInput{
		Name:          plan.Name.ValueString(),
		Comment:       stringValueToPtr(plan.Comment),
		Position:      int64ValueToPtr(plan.Position),
		ResourceTypes: stringSetToSlice(plan.ResourceTypes),
		SessionTypes:  stringSetToSlice(plan.SessionTypes),
		LocationIDs:   int64SetToSlice(plan.LocationIDs),
		TagIDs:        int64SetToSlice(plan.TagIDs),
		RoleIDs:       int64SetToSlice(plan.RoleIDs),
		CommandAclIDs: int64SetToSlice(plan.CommandAclIDs),
		Action:        plan.Action.ValueString(),
		Notify:        plan.Notify.ValueBool(),
		Log:           plan.Log.ValueBool(),
		Enabled:       plan.Enabled.ValueBool(),
	}

	rule, err := r.client.CreateRule(ctx, input)
	if err != nil {
		resp.Diagnostics.AddError("Error creating rule", err.Error())
		return
	}

	mapRuleResponseToModel(rule, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *SessionPolicyRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state SessionPolicyRuleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	rule, err := r.client.GetRule(ctx, state.ID.ValueInt64())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading rule", err.Error())
		return
	}

	mapRuleResponseToModel(rule, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *SessionPolicyRuleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan SessionPolicyRuleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state SessionPolicyRuleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	input := client.RuleUpdateInput{
		Name:          plan.Name.ValueString(),
		Comment:       stringValueToPtr(plan.Comment),
		Position:      int64ValueToPtr(plan.Position),
		ResourceTypes: stringSetToSlice(plan.ResourceTypes),
		SessionTypes:  stringSetToSlice(plan.SessionTypes),
		LocationIDs:   int64SetToSlice(plan.LocationIDs),
		TagIDs:        int64SetToSlice(plan.TagIDs),
		RoleIDs:       int64SetToSlice(plan.RoleIDs),
		CommandAclIDs: int64SetToSlice(plan.CommandAclIDs),
		Action:        plan.Action.ValueString(),
		Notify:        plan.Notify.ValueBool(),
		Log:           plan.Log.ValueBool(),
		Enabled:       plan.Enabled.ValueBool(),
	}

	rule, err := r.client.UpdateRule(ctx, state.ID.ValueInt64(), input)
	if err != nil {
		resp.Diagnostics.AddError("Error updating rule", err.Error())
		return
	}

	mapRuleResponseToModel(rule, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *SessionPolicyRuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state SessionPolicyRuleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteRule(ctx, state.ID.ValueInt64()); err != nil {
		resp.Diagnostics.AddError("Error deleting rule", err.Error())
	}
}

func (r *SessionPolicyRuleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("Expected integer ID, got: %s", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), types.Int64Value(id))...)
}

func mapRuleResponseToModel(rule *client.RuleResponse, m *SessionPolicyRuleResourceModel) {
	m.ID = types.Int64Value(rule.ID)
	m.Position = types.Int64Value(rule.Position)
	m.Name = types.StringValue(rule.Name)
	m.Comment = stringPtrToValue(rule.Comment)
	m.ResourceTypes = stringSliceToSet(rule.ResourceTypes)
	m.SessionTypes = stringSliceToSet(rule.SessionTypes)
	m.LocationIDs = int64SliceToSet(rule.LocationIDs)
	m.TagIDs = int64SliceToSet(rule.TagIDs)
	m.RoleIDs = int64SliceToSet(rule.RoleIDs)
	m.CommandAclIDs = int64SliceToSet(rule.CommandAclIDs)
	m.Action = types.StringValue(rule.Action)
	m.Notify = types.BoolValue(rule.Notify)
	m.Log = types.BoolValue(rule.Log)
	m.Enabled = types.BoolValue(rule.Enabled)
	m.HitCount = types.Int64Value(rule.HitCount)
	m.LastHitAt = optStringPtrToValue(rule.LastHitAt)
	m.CreatedAt = types.StringValue(rule.CreatedAt)
	m.UpdatedAt = types.StringValue(rule.UpdatedAt)
}
