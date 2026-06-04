package resources

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
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
	_ resource.Resource                = &LocationResource{}
	_ resource.ResourceWithImportState = &LocationResource{}
)

type LocationResource struct {
	client *client.Client
}

type LocationResourceModel struct {
	ID          types.Int64          `tfsdk:"id"`
	Name        types.String         `tfsdk:"name"`
	Kind        types.String         `tfsdk:"kind"`
	Description types.String         `tfsdk:"description"`
	ParentID    types.Int64          `tfsdk:"parent_id"`
	Latitude    types.Float64        `tfsdk:"latitude"`
	Longitude   types.Float64        `tfsdk:"longitude"`
	ExtraData   jsontypes.Normalized `tfsdk:"extra_data"`
	CreatedAt   types.String         `tfsdk:"created_at"`
	UpdatedAt   types.String         `tfsdk:"updated_at"`
}

func NewLocationResource() resource.Resource {
	return &LocationResource{}
}

func (r *LocationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_location"
}

func (r *LocationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a MisterShell location. Locations form a hierarchy for organizing network resources by geography or organizational structure.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Description: "Location ID.",
				Computed:    true,
				PlanModifiers: []planmodifier.Int64{
					int64UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Location name.",
				Required:    true,
			},
			"kind": schema.StringAttribute{
				Description: "Location type: 'geo' for geographic or 'org' for organizational.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("geo"),
				Validators: []validator.String{
					stringvalidator.OneOf("geo", "org"),
				},
			},
			"description": schema.StringAttribute{
				Description: "Location description.",
				Optional:    true,
			},
			"parent_id": schema.Int64Attribute{
				Description: "Parent location ID. Required — every location must be nested under an existing location (the root location has id 1 on a standard install); creating additional root locations is not allowed.",
				Required:    true,
			},
			"latitude": schema.Float64Attribute{
				Description: "Geographic latitude.",
				Optional:    true,
			},
			"longitude": schema.Float64Attribute{
				Description: "Geographic longitude.",
				Optional:    true,
			},
			"extra_data": schema.StringAttribute{
				Description: "Additional metadata as JSON. Use jsonencode() in HCL.",
				Optional:    true,
				CustomType:  jsontypes.NormalizedType{},
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

func (r *LocationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *LocationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan LocationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	input := client.LocationCreateInput{
		Name: plan.Name.ValueString(),
		Kind: plan.Kind.ValueString(),
	}
	if !plan.Description.IsNull() {
		v := plan.Description.ValueString()
		input.Description = &v
	}
	if !plan.ParentID.IsNull() {
		v := plan.ParentID.ValueInt64()
		input.ParentID = &v
	}
	if !plan.Latitude.IsNull() {
		v := plan.Latitude.ValueFloat64()
		input.Latitude = &v
	}
	if !plan.Longitude.IsNull() {
		v := plan.Longitude.ValueFloat64()
		input.Longitude = &v
	}
	input.ExtraData = normalizedToRawJSON(plan.ExtraData)

	loc, err := r.client.CreateLocation(ctx, input)
	if err != nil {
		resp.Diagnostics.AddError("Error creating location", err.Error())
		return
	}

	mapLocationResponseToModel(loc, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *LocationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state LocationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	loc, err := r.client.GetLocation(ctx, state.ID.ValueInt64())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading location", err.Error())
		return
	}

	mapLocationResponseToModel(loc, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *LocationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan LocationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state LocationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	input := client.LocationUpdateInput{}
	if !plan.Name.Equal(state.Name) {
		v := plan.Name.ValueString()
		input.Name = &v
	}
	if !plan.Kind.Equal(state.Kind) {
		v := plan.Kind.ValueString()
		input.Kind = &v
	}
	input.Description = stringValueToPtr(plan.Description)
	input.ParentID = int64ValueToPtr(plan.ParentID)
	input.Latitude = float64ValueToPtr(plan.Latitude)
	input.Longitude = float64ValueToPtr(plan.Longitude)
	input.ExtraData = normalizedToRawJSON(plan.ExtraData)

	loc, err := r.client.UpdateLocation(ctx, state.ID.ValueInt64(), input)
	if err != nil {
		resp.Diagnostics.AddError("Error updating location", err.Error())
		return
	}

	mapLocationResponseToModel(loc, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *LocationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state LocationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteLocation(ctx, state.ID.ValueInt64()); err != nil {
		resp.Diagnostics.AddError("Error deleting location", err.Error())
	}
}

func (r *LocationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("Expected integer ID, got: %s", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), types.Int64Value(id))...)
}

func mapLocationResponseToModel(loc *client.LocationResponse, m *LocationResourceModel) {
	m.ID = types.Int64Value(loc.ID)
	m.Name = types.StringValue(loc.Name)
	m.Kind = types.StringValue(loc.Kind)
	m.Description = stringPtrToValue(loc.Description)
	m.ParentID = int64PtrToValue(loc.ParentID)
	m.Latitude = float64PtrToValue(loc.Latitude)
	m.Longitude = float64PtrToValue(loc.Longitude)
	m.ExtraData = rawJSONToNormalized(loc.ExtraData)
	m.CreatedAt = types.StringValue(loc.CreatedAt)
	m.UpdatedAt = types.StringValue(loc.UpdatedAt)
}
