package datasources

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-mistershell/internal/client"
)

var _ datasource.DataSource = &TagsDataSource{}

type TagsDataSource struct {
	client *client.Client
}

type TagsDataSourceModel struct {
	Search types.String `tfsdk:"search"`
	Tags   types.List   `tfsdk:"tags"`
}

func NewTagsDataSource() datasource.DataSource {
	return &TagsDataSource{}
}

func (d *TagsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tags"
}

// tagsCatalogAttrTypes is the element object type for the catalog `tags` list. It
// includes resource_count (the List endpoint exposes it; the per-resource tag
// read-back in tagObjectType does not).
var tagsCatalogAttrTypes = map[string]attr.Type{
	"id":             types.Int64Type,
	"name":           types.StringType,
	"color":          types.StringType,
	"description":    types.StringType,
	"resource_count": types.Int64Type,
}

var tagsCatalogObjectType = types.ObjectType{AttrTypes: tagsCatalogAttrTypes}

func (d *TagsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Lists MisterShell tags. With no filter it returns every tag; set `search` to a substring to narrow the list (server-side fuzzy match). Use the singular `mistershell_tag` data source to look up exactly one tag.",
		Attributes: map[string]schema.Attribute{
			"search": schema.StringAttribute{
				Description: "Optional case-insensitive substring filter on the tag name. Omit to return all tags.",
				Optional:    true,
			},
			"tags": schema.ListNestedAttribute{
				Description: "All matching tags, as objects with id, name, color, description, and resource_count.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":             schema.Int64Attribute{Computed: true, Description: "Tag ID."},
						"name":           schema.StringAttribute{Computed: true, Description: "Tag name."},
						"color":          schema.StringAttribute{Computed: true, Description: "Tag color."},
						"description":    schema.StringAttribute{Computed: true, Description: "Tag description."},
						"resource_count": schema.Int64Attribute{Computed: true, Description: "Number of resources currently assigned this tag."},
					},
				},
			},
		},
	}
}

func (d *TagsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *TagsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config TagsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	filter := client.TagListFilter{}
	if !config.Search.IsNull() && !config.Search.IsUnknown() {
		filter.Search = config.Search.ValueString()
	}

	items, err := d.client.ListTags(ctx, filter)
	if err != nil {
		resp.Diagnostics.AddError("Error listing tags", err.Error())
		return
	}

	elems := make([]attr.Value, 0, len(items))
	for _, it := range items {
		obj := types.ObjectValueMust(tagsCatalogAttrTypes, map[string]attr.Value{
			"id":             types.Int64Value(it.ID),
			"name":           types.StringValue(it.Name),
			"color":          types.StringValue(it.Color),
			"description":    stringPtrToValue(it.Description),
			"resource_count": types.Int64Value(it.ResourceCount),
		})
		elems = append(elems, obj)
	}
	config.Tags = types.ListValueMust(tagsCatalogObjectType, elems)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
