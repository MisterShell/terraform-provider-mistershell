package datasources

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-mistershell/internal/client"
)

var _ datasource.DataSource = &SettingDataSource{}

type SettingDataSource struct {
	client *client.Client
}

type SettingDataSourceModel struct {
	Key         types.String         `tfsdk:"key"`
	Value       jsontypes.Normalized `tfsdk:"value"`
	Description types.String         `tfsdk:"description"`
	IsSecret    types.Bool           `tfsdk:"is_secret"`
	Default     jsontypes.Normalized `tfsdk:"default"`
	CreatedAt   types.String         `tfsdk:"created_at"`
	UpdatedAt   types.String         `tfsdk:"updated_at"`
}

func NewSettingDataSource() datasource.DataSource {
	return &SettingDataSource{}
}

func (d *SettingDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_setting"
}

func (d *SettingDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a MisterShell application setting by key. value is JSON (heterogeneous bool/int/string). Secret-key values and defaults are masked as '****' by the API. Unknown keys return an error.",
		Attributes: map[string]schema.Attribute{
			"key": schema.StringAttribute{
				Description: "Setting key to read. Must be a key in the backend settings registry.",
				Required:    true,
			},
			"value": schema.StringAttribute{
				Description: "Current value as JSON. Masked as '****' for secret keys.",
				Computed:    true,
				Sensitive:   true,
				CustomType:  jsontypes.NormalizedType{},
			},
			"description": schema.StringAttribute{
				Description: "Human description of the setting.",
				Computed:    true,
			},
			"is_secret": schema.BoolAttribute{
				Description: "Whether the value is a secret.",
				Computed:    true,
			},
			"default": schema.StringAttribute{
				Description: "Registry default value as JSON. Masked for non-empty secret keys.",
				Computed:    true,
				CustomType:  jsontypes.NormalizedType{},
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

func (d *SettingDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *SettingDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config SettingDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	key := config.Key.ValueString()
	setting, err := d.client.GetSetting(ctx, key)
	if err != nil {
		if client.IsNotFound(err) {
			resp.Diagnostics.AddError("No such setting key", "No setting found for key "+key+". Valid keys are fixed by the backend registry.")
			return
		}
		resp.Diagnostics.AddError("Error reading setting", err.Error())
		return
	}

	config.Key = types.StringValue(setting.Key)
	config.Value = rawJSONToNormalized(setting.Value)
	config.Description = types.StringValue(setting.Description)
	config.IsSecret = types.BoolValue(setting.IsSecret)
	config.Default = rawJSONToNormalized(setting.Default)
	config.CreatedAt = types.StringValue(setting.CreatedAt)
	config.UpdatedAt = types.StringValue(setting.UpdatedAt)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
