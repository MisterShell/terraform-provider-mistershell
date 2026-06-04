package datasources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-mistershell/internal/client"
)

var _ datasource.DataSource = &TagDataSource{}

type TagDataSource struct {
	client *client.Client
}

type TagDataSourceModel struct {
	ID            types.Int64  `tfsdk:"id"`
	Name          types.String `tfsdk:"name"`
	Color         types.String `tfsdk:"color"`
	Description   types.String `tfsdk:"description"`
	ResourceIDs   types.Set    `tfsdk:"resource_ids"`
	ResourceCount types.Int64  `tfsdk:"resource_count"`
	CreatedAt     types.String `tfsdk:"created_at"`
	UpdatedAt     types.String `tfsdk:"updated_at"`
}

func NewTagDataSource() datasource.DataSource {
	return &TagDataSource{}
}

func (d *TagDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tag"
}

func (d *TagDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a MisterShell tag. Look up by id for a direct fetch, or by name (exact match, must resolve to exactly one tag).",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Description: "Tag ID. Use for direct lookup.",
				Optional:    true,
				Computed:    true,
			},
			"name": schema.StringAttribute{
				Description: "Tag name. Used as an exact-match search filter when id is not specified.",
				Optional:    true,
				Computed:    true,
			},
			"color": schema.StringAttribute{
				Description: "Tag color.",
				Computed:    true,
			},
			"description": schema.StringAttribute{
				Description: "Tag description.",
				Computed:    true,
			},
			"resource_ids": schema.SetAttribute{
				Description: "Set of network resource IDs assigned this tag.",
				Computed:    true,
				ElementType: types.Int64Type,
			},
			"resource_count": schema.Int64Attribute{
				Description: "Number of resources assigned this tag.",
				Computed:    true,
			},
			"created_at": schema.StringAttribute{
				Description: "Creation timestamp.",
				Computed:    true,
			},
			"updated_at": schema.StringAttribute{
				Description: "Last update timestamp.",
				Computed:    true,
			},
		},
	}
}

func (d *TagDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected DataSource Configure Type", "Expected *client.Client")
		return
	}
	d.client = c
}

func (d *TagDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config TagDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	hasID := !config.ID.IsNull() && !config.ID.IsUnknown()

	var id int64
	resourceCount := int64(-1)

	if hasID {
		id = config.ID.ValueInt64()
	} else {
		hasName := !config.Name.IsNull() && !config.Name.IsUnknown()
		if !hasName {
			resp.Diagnostics.AddError("Missing lookup criteria", "Specify 'id' for direct lookup, or 'name' to search.")
			return
		}

		name := config.Name.ValueString()
		tags, err := d.client.ListTags(ctx, client.TagListFilter{Search: name})
		if err != nil {
			resp.Diagnostics.AddError("Error searching tags", err.Error())
			return
		}

		var matched []client.TagListItem
		for _, t := range tags {
			if t.Name == name {
				matched = append(matched, t)
			}
		}
		if len(matched) == 0 {
			resp.Diagnostics.AddError("No matching tag found", "No tag matches the specified name.")
			return
		}
		if len(matched) > 1 {
			resp.Diagnostics.AddError("Multiple tags found",
				fmt.Sprintf("Found %d tags matching the name. Tag names should be unique.", len(matched)))
			return
		}
		id = matched[0].ID
		resourceCount = matched[0].ResourceCount
	}

	// Fetch the full tag (List shape lacks timestamps).
	tag, err := d.client.GetTag(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Error reading tag", err.Error())
		return
	}
	ids, err := d.client.GetTagResourceIDs(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Error reading tag resource ids", err.Error())
		return
	}

	if resourceCount < 0 {
		resourceCount = int64(len(ids))
	}

	config.ID = types.Int64Value(tag.ID)
	config.Name = types.StringValue(tag.Name)
	config.Color = types.StringValue(tag.Color)
	config.Description = stringPtrToValue(tag.Description)
	config.ResourceIDs = int64SliceToSet(ids)
	config.ResourceCount = types.Int64Value(resourceCount)
	config.CreatedAt = types.StringValue(tag.CreatedAt)
	config.UpdatedAt = types.StringValue(tag.UpdatedAt)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
