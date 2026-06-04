package resources

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-mistershell/internal/client"
)

var (
	_ resource.Resource                = &SessionPolicyAclResource{}
	_ resource.ResourceWithImportState = &SessionPolicyAclResource{}
)

type SessionPolicyAclResource struct {
	client *client.Client
}

type SessionPolicyAclResourceModel struct {
	ID          types.Int64  `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Patterns    types.List   `tfsdk:"patterns"`
	Enabled     types.Bool   `tfsdk:"enabled"`
	IsBuiltin   types.Bool   `tfsdk:"is_builtin"`
	CreatedAt   types.String `tfsdk:"created_at"`
	UpdatedAt   types.String `tfsdk:"updated_at"`
}

func NewSessionPolicyAclResource() resource.Resource {
	return &SessionPolicyAclResource{}
}

func (r *SessionPolicyAclResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_session_policy_acl"
}

func (r *SessionPolicyAclResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	emptyPatterns, _ := types.ListValue(aclPatternObjectType, []attr.Value{})
	resp.Schema = schema.Schema{
		Description: "Manages a MisterShell session-policy command ACL: a named, ordered set of glob/regex command patterns. ACLs are referenced by session-policy rules. The backend exposes no GET-single endpoint for ACLs, so reads go through the list endpoint. Built-in ACLs (is_builtin=true) cannot be edited or deleted.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Description: "ACL ID.",
				Computed:    true,
				PlanModifiers: []planmodifier.Int64{
					int64UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Unique ACL name (1-128 characters).",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, 128),
				},
			},
			"description": schema.StringAttribute{
				Description: "ACL description (up to 255 characters).",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
				Validators: []validator.String{
					stringvalidator.LengthAtMost(255),
				},
			},
			"patterns": schema.ListNestedAttribute{
				Description: "Ordered list of command-match patterns. Order is significant (evaluation merges them in order). Defaults to an empty list.",
				Optional:    true,
				Computed:    true,
				Default:     listdefault.StaticValue(emptyPatterns),
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"pattern": schema.StringAttribute{
							Description: "The match pattern (1-512 characters).",
							Required:    true,
							Validators: []validator.String{
								stringvalidator.LengthBetween(1, 512),
							},
						},
						"type": schema.StringAttribute{
							Description: "Pattern type. One of: glob, regex. Defaults to glob.",
							Optional:    true,
							Computed:    true,
							Default:     stringdefault.StaticString("glob"),
							Validators: []validator.String{
								stringvalidator.OneOf(client.SupportedAclPatternTypes...),
							},
						},
					},
				},
			},
			"enabled": schema.BoolAttribute{
				Description: "Whether the ACL is enabled. Defaults to true.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
			},
			"is_builtin": schema.BoolAttribute{
				Description: "Whether this is a built-in ACL (built-ins cannot be edited or deleted).",
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

func (r *SessionPolicyAclResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *SessionPolicyAclResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan SessionPolicyAclResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	input := client.AclCreateInput{
		Name:        plan.Name.ValueString(),
		Description: plan.Description.ValueString(),
		Patterns:    aclPatternsToSlice(plan.Patterns),
	}

	acl, err := r.client.CreateAcl(ctx, input)
	if err != nil {
		resp.Diagnostics.AddError("Error creating ACL", err.Error())
		return
	}

	// Create always starts enabled=true (server default); enabled is not a create
	// field. If the plan disables it, follow up with an update.
	if !plan.Enabled.IsNull() && !plan.Enabled.IsUnknown() && !plan.Enabled.ValueBool() {
		acl, err = r.client.UpdateAcl(ctx, acl.ID, client.AclUpdateInput{
			Name:        plan.Name.ValueString(),
			Description: plan.Description.ValueString(),
			Patterns:    aclPatternsToSlice(plan.Patterns),
			Enabled:     false,
		})
		if err != nil {
			resp.Diagnostics.AddError("Error disabling ACL", err.Error())
			return
		}
	}

	mapAclResponseToModel(acl, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *SessionPolicyAclResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state SessionPolicyAclResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	acl, err := r.client.GetAcl(ctx, state.ID.ValueInt64())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading ACL", err.Error())
		return
	}

	mapAclResponseToModel(acl, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *SessionPolicyAclResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan SessionPolicyAclResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state SessionPolicyAclResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	input := client.AclUpdateInput{
		Name:        plan.Name.ValueString(),
		Description: plan.Description.ValueString(),
		Patterns:    aclPatternsToSlice(plan.Patterns),
		Enabled:     plan.Enabled.ValueBool(),
	}

	acl, err := r.client.UpdateAcl(ctx, state.ID.ValueInt64(), input)
	if err != nil {
		resp.Diagnostics.AddError("Error updating ACL", err.Error())
		return
	}

	mapAclResponseToModel(acl, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *SessionPolicyAclResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state SessionPolicyAclResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteAcl(ctx, state.ID.ValueInt64()); err != nil {
		resp.Diagnostics.AddError("Error deleting ACL", err.Error())
	}
}

func (r *SessionPolicyAclResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("Expected integer ID, got: %s", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), types.Int64Value(id))...)
}

func mapAclResponseToModel(acl *client.AclResponse, m *SessionPolicyAclResourceModel) {
	m.ID = types.Int64Value(acl.ID)
	m.Name = types.StringValue(acl.Name)
	m.Description = types.StringValue(acl.Description)
	m.Patterns = aclPatternsToList(acl.Patterns)
	m.Enabled = types.BoolValue(acl.Enabled)
	m.IsBuiltin = types.BoolValue(acl.IsBuiltin)
	m.CreatedAt = types.StringValue(acl.CreatedAt)
	m.UpdatedAt = types.StringValue(acl.UpdatedAt)
}
