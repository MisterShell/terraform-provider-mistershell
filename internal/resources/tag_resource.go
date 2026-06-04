package resources

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-mistershell/internal/client"
)

var (
	_ resource.Resource                = &TagResource{}
	_ resource.ResourceWithImportState = &TagResource{}
)

type TagResource struct {
	client *client.Client
}

type TagResourceModel struct {
	ID          types.Int64  `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Color       types.String `tfsdk:"color"`
	Description types.String `tfsdk:"description"`
	ResourceIDs types.Set    `tfsdk:"resource_ids"`
	CreatedAt   types.String `tfsdk:"created_at"`
	UpdatedAt   types.String `tfsdk:"updated_at"`
}

func NewTagResource() resource.Resource {
	return &TagResource{}
}

func (r *TagResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tag"
}

func (r *TagResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a MisterShell tag. Tags are free-form labels (name, color, description) that can be assigned to network resources. The optional resource_ids set manages, with whole-set replace semantics, exactly which network resources carry this tag.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Description: "Tag ID.",
				Computed:    true,
				PlanModifiers: []planmodifier.Int64{
					int64UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Tag name (1-64 characters).",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, 64),
				},
			},
			"color": schema.StringAttribute{
				Description: "Tag color. A free-form string (1-16 characters); there is no fixed palette or enum. Defaults to 'grey'.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("grey"),
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, 16),
				},
			},
			"description": schema.StringAttribute{
				Description: "Tag description (up to 2000 characters).",
				Optional:    true,
				Validators: []validator.String{
					stringvalidator.LengthAtMost(2000),
				},
			},
			"resource_ids": schema.SetAttribute{
				Description: "Set of network resource IDs assigned this tag. Managing this attribute takes exclusive ownership of the tag's resource membership (whole-set replace).",
				Optional:    true,
				Computed:    true,
				ElementType: types.Int64Type,
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

func (r *TagResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *TagResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan TagResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	input := client.TagCreateInput{
		Name:  plan.Name.ValueString(),
		Color: plan.Color.ValueString(),
	}
	if !plan.Description.IsNull() {
		v := plan.Description.ValueString()
		input.Description = &v
	}

	tag, err := r.client.CreateTag(ctx, input)
	if err != nil {
		resp.Diagnostics.AddError("Error creating tag", err.Error())
		return
	}

	// Apply resource assignments if specified.
	if !plan.ResourceIDs.IsNull() && !plan.ResourceIDs.IsUnknown() {
		ids := int64SetToSlice(plan.ResourceIDs)
		if err := r.client.SetTagAssignments(ctx, tag.ID, ids); err != nil {
			resp.Diagnostics.AddError("Error assigning tag resources", err.Error())
			return
		}
	}

	if err := r.finalizeState(ctx, tag.ID, &plan); err != nil {
		resp.Diagnostics.AddError("Error reading tag after create", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *TagResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state TagResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tag, err := r.client.GetTag(ctx, state.ID.ValueInt64())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading tag", err.Error())
		return
	}

	ids, err := r.client.GetTagResourceIDs(ctx, state.ID.ValueInt64())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading tag resource ids", err.Error())
		return
	}

	mapTagResponseToModel(tag, ids, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *TagResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan TagResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state TagResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := state.ID.ValueInt64()

	input := client.TagUpdateInput{}
	if !plan.Name.Equal(state.Name) {
		v := plan.Name.ValueString()
		input.Name = &v
	}
	if !plan.Color.Equal(state.Color) {
		v := plan.Color.ValueString()
		input.Color = &v
	}
	input.Description = stringValueToPtr(plan.Description)

	if _, err := r.client.UpdateTag(ctx, id, input); err != nil {
		resp.Diagnostics.AddError("Error updating tag", err.Error())
		return
	}

	// Reconcile resource assignments if the set changed.
	if !plan.ResourceIDs.Equal(state.ResourceIDs) {
		ids := int64SetToSlice(plan.ResourceIDs)
		if err := r.client.SetTagAssignments(ctx, id, ids); err != nil {
			resp.Diagnostics.AddError("Error updating tag resources", err.Error())
			return
		}
	}

	if err := r.finalizeState(ctx, id, &plan); err != nil {
		resp.Diagnostics.AddError("Error reading tag after update", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *TagResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state TagResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteTag(ctx, state.ID.ValueInt64()); err != nil {
		resp.Diagnostics.AddError("Error deleting tag", err.Error())
	}
}

func (r *TagResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("Expected integer ID, got: %s", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), types.Int64Value(id))...)
}

// finalizeState re-GETs the tag and its resource ids to populate model state.
func (r *TagResource) finalizeState(ctx context.Context, id int64, m *TagResourceModel) error {
	tag, err := r.client.GetTag(ctx, id)
	if err != nil {
		return err
	}
	ids, err := r.client.GetTagResourceIDs(ctx, id)
	if err != nil {
		return err
	}
	mapTagResponseToModel(tag, ids, m)
	return nil
}

func mapTagResponseToModel(tag *client.TagResponse, resourceIDs []int64, m *TagResourceModel) {
	m.ID = types.Int64Value(tag.ID)
	m.Name = types.StringValue(tag.Name)
	m.Color = types.StringValue(tag.Color)
	m.Description = stringPtrToValue(tag.Description)
	m.ResourceIDs = int64SliceToSet(resourceIDs)
	m.CreatedAt = types.StringValue(tag.CreatedAt)
	m.UpdatedAt = types.StringValue(tag.UpdatedAt)
}
