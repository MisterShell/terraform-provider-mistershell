package datasources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-mistershell/internal/client"
)

var _ datasource.DataSource = &AISkillDataSource{}

type AISkillDataSource struct {
	client *client.Client
}

type AISkillDataSourceModel struct {
	ID            types.Int64  `tfsdk:"id"`
	Name          types.String `tfsdk:"name"`
	Description   types.String `tfsdk:"description"`
	Body          types.String `tfsdk:"body"`
	AgentTypes    types.Set    `tfsdk:"agent_types"`
	ResourceTypes types.Set    `tfsdk:"resource_types"`
	IsEnabled     types.Bool   `tfsdk:"is_enabled"`
	IsBuiltin     types.Bool   `tfsdk:"is_builtin"`
	CreatedAt     types.String `tfsdk:"created_at"`
	UpdatedAt     types.String `tfsdk:"updated_at"`
}

func NewAISkillDataSource() datasource.DataSource {
	return &AISkillDataSource{}
}

func (d *AISkillDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ai_skill"
}

func (d *AISkillDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a MisterShell AI skill (markdown platform brief surfaced to agents via list_skills). Look up by id for a direct fetch, or by name (exact match). Both user-defined and builtin skills (is_builtin=true) are exposed read-only here.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Description: "AI skill ID. Use for direct lookup.",
				Optional:    true,
				Computed:    true,
			},
			"name": schema.StringAttribute{
				Description: "Skill name. Used as an exact-match search filter when id is not specified.",
				Optional:    true,
				Computed:    true,
			},
			"description": schema.StringAttribute{
				Description: "Human-readable description of the skill.",
				Computed:    true,
			},
			"body": schema.StringAttribute{
				Description: "Markdown platform-brief content surfaced to agents.",
				Computed:    true,
			},
			"agent_types": schema.SetAttribute{
				Description: "Agent types the skill discovery is restricted to (null if unrestricted).",
				Computed:    true,
				ElementType: types.StringType,
			},
			"resource_types": schema.SetAttribute{
				Description: "Resource-type keys the skill discovery is restricted to (null if unrestricted).",
				Computed:    true,
				ElementType: types.StringType,
			},
			"is_enabled": schema.BoolAttribute{
				Description: "Whether the skill is enabled.",
				Computed:    true,
			},
			"is_builtin": schema.BoolAttribute{
				Description: "Whether the skill is a builtin (managed by MisterShell). Builtin skills are read-only except their is_enabled flag.",
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

func (d *AISkillDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *AISkillDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config AISkillDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	hasID := !config.ID.IsNull() && !config.ID.IsUnknown()

	var skill *client.AISkillResponse

	if hasID {
		var err error
		skill, err = d.client.GetAISkill(ctx, config.ID.ValueInt64())
		if err != nil {
			resp.Diagnostics.AddError("Error reading AI skill", err.Error())
			return
		}
	} else {
		hasName := !config.Name.IsNull() && !config.Name.IsUnknown()
		if !hasName {
			resp.Diagnostics.AddError("Missing lookup criteria", "Specify 'id' for direct lookup, or 'name' to search.")
			return
		}

		skills, err := d.client.ListAISkills(ctx, client.AISkillListFilter{Search: config.Name.ValueString()})
		if err != nil {
			resp.Diagnostics.AddError("Error searching AI skills", err.Error())
			return
		}

		name := config.Name.ValueString()
		var filtered []client.AISkillResponse
		for _, s := range skills {
			if s.Name == name {
				filtered = append(filtered, s)
			}
		}

		if len(filtered) == 0 {
			resp.Diagnostics.AddError("No matching AI skill found", "No AI skill matches the specified name.")
			return
		}
		if len(filtered) > 1 {
			resp.Diagnostics.AddError("Multiple AI skills found",
				fmt.Sprintf("Found %d AI skills matching name %q.", len(filtered), name))
			return
		}
		skill = &filtered[0]
	}

	config.ID = types.Int64Value(skill.ID)
	config.Name = types.StringValue(skill.Name)
	config.Description = stringPtrToValue(skill.Description)
	config.Body = types.StringValue(skill.Body)
	config.AgentTypes = stringSliceToSetOrNull(skill.AgentTypes)
	config.ResourceTypes = stringSliceToSetOrNull(skill.ResourceTypes)
	config.IsEnabled = types.BoolValue(skill.IsEnabled)
	config.IsBuiltin = types.BoolValue(skill.IsBuiltin)
	config.CreatedAt = types.StringValue(skill.CreatedAt)
	config.UpdatedAt = types.StringValue(skill.UpdatedAt)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

// stringSliceToSetOrNull converts []string to a types.Set of String, yielding a
// NULL set when the slice is empty/nil — consistent with the resource mapping.
func stringSliceToSetOrNull(vals []string) types.Set {
	if len(vals) == 0 {
		return types.SetNull(types.StringType)
	}
	return stringSliceToSet(vals)
}
