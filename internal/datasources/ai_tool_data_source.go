package datasources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-mistershell/internal/client"
)

var _ datasource.DataSource = &AIToolDataSource{}

type AIToolDataSource struct {
	client *client.Client
}

type AIToolDataSourceModel struct {
	ID                 types.Int64          `tfsdk:"id"`
	Name               types.String         `tfsdk:"name"`
	Description        types.String         `tfsdk:"description"`
	Handler            types.String         `tfsdk:"handler"`
	InputSchema        jsontypes.Normalized `tfsdk:"input_schema"`
	RequiredPermission types.String         `tfsdk:"required_permission"`
	CreatedAt          types.String         `tfsdk:"created_at"`
}

func NewAIToolDataSource() datasource.DataSource {
	return &AIToolDataSource{}
}

func (d *AIToolDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ai_tool"
}

func (d *AIToolDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a MisterShell AI tool (read-only, backend-builtin). Look up by id or by exact name to obtain the tool id for an mistershell_ai_agent's tool_ids.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Description: "AI tool ID. Use for direct lookup.",
				Optional:    true,
				Computed:    true,
			},
			"name": schema.StringAttribute{
				Description: "AI tool name. Used as an exact-match search filter when id is not specified.",
				Optional:    true,
				Computed:    true,
			},
			"description": schema.StringAttribute{
				Description: "Human-readable description of the tool.",
				Computed:    true,
			},
			"handler": schema.StringAttribute{
				Description: "Backend handler identifier for the tool.",
				Computed:    true,
			},
			"input_schema": schema.StringAttribute{
				Description: "The tool's JSON Schema describing its input parameters.",
				Computed:    true,
				CustomType:  jsontypes.NormalizedType{},
			},
			"required_permission": schema.StringAttribute{
				Description: "Permission required to invoke the tool, if any.",
				Computed:    true,
			},
			"created_at": schema.StringAttribute{
				Description: "Creation timestamp.",
				Computed:    true,
			},
		},
	}
}

func (d *AIToolDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *AIToolDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config AIToolDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	hasID := !config.ID.IsNull() && !config.ID.IsUnknown()

	var tool *client.AIToolResponse

	if hasID {
		var err error
		tool, err = d.client.GetAITool(ctx, config.ID.ValueInt64())
		if err != nil {
			resp.Diagnostics.AddError("Error reading AI tool", err.Error())
			return
		}
	} else {
		hasName := !config.Name.IsNull() && !config.Name.IsUnknown()
		if !hasName {
			resp.Diagnostics.AddError("Missing lookup criteria", "Specify 'id' for direct lookup, or 'name' to search.")
			return
		}

		tools, err := d.client.ListAITools(ctx, client.AIToolListFilter{Search: config.Name.ValueString()})
		if err != nil {
			resp.Diagnostics.AddError("Error searching AI tools", err.Error())
			return
		}

		name := config.Name.ValueString()
		var filtered []client.AIToolResponse
		for _, t := range tools {
			if t.Name == name {
				filtered = append(filtered, t)
			}
		}

		if len(filtered) == 0 {
			resp.Diagnostics.AddError("No matching AI tool found", "No AI tool matches the specified name.")
			return
		}
		if len(filtered) > 1 {
			resp.Diagnostics.AddError("Multiple AI tools found",
				fmt.Sprintf("Found %d AI tools matching name %q.", len(filtered), name))
			return
		}
		tool = &filtered[0]
	}

	config.ID = types.Int64Value(tool.ID)
	config.Name = types.StringValue(tool.Name)
	config.Description = types.StringValue(tool.Description)
	config.Handler = types.StringValue(tool.Handler)
	config.InputSchema = rawJSONToNormalized(tool.InputSchema)
	config.RequiredPermission = stringPtrToValue(tool.RequiredPermission)
	config.CreatedAt = types.StringValue(tool.CreatedAt)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
