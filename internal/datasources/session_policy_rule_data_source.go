package datasources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-mistershell/internal/client"
)

var _ datasource.DataSource = &SessionPolicyRuleDataSource{}

type SessionPolicyRuleDataSource struct {
	client *client.Client
}

type SessionPolicyRuleDataSourceModel struct {
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

func NewSessionPolicyRuleDataSource() datasource.DataSource {
	return &SessionPolicyRuleDataSource{}
}

func (d *SessionPolicyRuleDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_session_policy_rule"
}

func (d *SessionPolicyRuleDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a MisterShell session-policy rule. Look up by id or by exact name. Rule names are not unique server-side, so name lookup errors on more than one match. Reads resolve through the list endpoint (there is no GET-single for rules).",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Description: "Rule ID. Use for direct lookup.",
				Optional:    true,
				Computed:    true,
			},
			"position": schema.Int64Attribute{
				Description: "1-based evaluation position.",
				Computed:    true,
			},
			"name": schema.StringAttribute{
				Description: "Rule name (exact match). Used as a lookup filter when id is not specified.",
				Optional:    true,
				Computed:    true,
			},
			"comment": schema.StringAttribute{
				Description: "Free-form comment.",
				Computed:    true,
			},
			"resource_types": schema.SetAttribute{
				Description: "Resource types this rule applies to (empty means Any).",
				Computed:    true,
				ElementType: types.StringType,
			},
			"session_types": schema.SetAttribute{
				Description: "Session types this rule applies to (empty means Any).",
				Computed:    true,
				ElementType: types.StringType,
			},
			"location_ids": schema.SetAttribute{
				Description: "Location IDs this rule applies to (empty means Any).",
				Computed:    true,
				ElementType: types.Int64Type,
			},
			"tag_ids": schema.SetAttribute{
				Description: "Tag IDs this rule applies to (empty means Any).",
				Computed:    true,
				ElementType: types.Int64Type,
			},
			"role_ids": schema.SetAttribute{
				Description: "Role IDs this rule applies to (empty means Any).",
				Computed:    true,
				ElementType: types.Int64Type,
			},
			"command_acl_ids": schema.SetAttribute{
				Description: "Command ACL IDs referenced by this rule (empty means Any).",
				Computed:    true,
				ElementType: types.Int64Type,
			},
			"action": schema.StringAttribute{
				Description: "Rule action (accept or deny).",
				Computed:    true,
			},
			"notify": schema.BoolAttribute{
				Description: "Whether to notify on match.",
				Computed:    true,
			},
			"log": schema.BoolAttribute{
				Description: "Whether to log on match.",
				Computed:    true,
			},
			"enabled": schema.BoolAttribute{
				Description: "Whether the rule is enabled.",
				Computed:    true,
			},
			"hit_count": schema.Int64Attribute{
				Description: "Number of times this rule has matched (runtime telemetry).",
				Computed:    true,
			},
			"last_hit_at": schema.StringAttribute{
				Description: "Timestamp of the last match (runtime telemetry).",
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

func (d *SessionPolicyRuleDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *SessionPolicyRuleDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config SessionPolicyRuleDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	hasID := !config.ID.IsNull() && !config.ID.IsUnknown()
	hasName := !config.Name.IsNull() && !config.Name.IsUnknown()

	if !hasID && !hasName {
		resp.Diagnostics.AddError("Missing lookup criteria", "Specify 'id' or 'name'.")
		return
	}

	var rule *client.RuleResponse

	if hasID {
		var err error
		rule, err = d.client.GetRule(ctx, config.ID.ValueInt64())
		if err != nil {
			resp.Diagnostics.AddError("Error reading rule", err.Error())
			return
		}
	} else {
		name := config.Name.ValueString()
		rules, err := d.client.ListRules(ctx)
		if err != nil {
			resp.Diagnostics.AddError("Error listing rules", err.Error())
			return
		}
		var filtered []client.RuleResponse
		for _, ru := range rules {
			if ru.Name == name {
				filtered = append(filtered, ru)
			}
		}
		if len(filtered) == 0 {
			resp.Diagnostics.AddError("No matching rule found", "No rule matches the specified name.")
			return
		}
		if len(filtered) > 1 {
			resp.Diagnostics.AddError("Multiple rules found",
				fmt.Sprintf("Found %d rules named %q. Use id for a unique lookup.", len(filtered), name))
			return
		}
		rule = &filtered[0]
	}

	config.ID = types.Int64Value(rule.ID)
	config.Position = types.Int64Value(rule.Position)
	config.Name = types.StringValue(rule.Name)
	config.Comment = optStringPtrToValue(rule.Comment)
	config.ResourceTypes = stringSliceToSet(rule.ResourceTypes)
	config.SessionTypes = stringSliceToSet(rule.SessionTypes)
	config.LocationIDs = int64SliceToSet(rule.LocationIDs)
	config.TagIDs = int64SliceToSet(rule.TagIDs)
	config.RoleIDs = int64SliceToSet(rule.RoleIDs)
	config.CommandAclIDs = int64SliceToSet(rule.CommandAclIDs)
	config.Action = types.StringValue(rule.Action)
	config.Notify = types.BoolValue(rule.Notify)
	config.Log = types.BoolValue(rule.Log)
	config.Enabled = types.BoolValue(rule.Enabled)
	config.HitCount = types.Int64Value(rule.HitCount)
	config.LastHitAt = optStringPtrToValue(rule.LastHitAt)
	config.CreatedAt = types.StringValue(rule.CreatedAt)
	config.UpdatedAt = types.StringValue(rule.UpdatedAt)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
