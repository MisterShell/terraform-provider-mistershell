package resources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"terraform-provider-mistershell/internal/client"
)

var (
	_ resource.Resource                = &SettingResource{}
	_ resource.ResourceWithImportState = &SettingResource{}
)

type SettingResource struct {
	client *client.Client
}

type SettingResourceModel struct {
	ID          types.String  `tfsdk:"id"`
	Key         types.String  `tfsdk:"key"`
	Value       types.Dynamic `tfsdk:"value"`
	Description types.String  `tfsdk:"description"`
	IsSecret    types.Bool    `tfsdk:"is_secret"`
	Default     types.Dynamic `tfsdk:"default"`
	Schema      types.Dynamic `tfsdk:"schema"`
	IsEncrypted types.Bool    `tfsdk:"is_encrypted"`
	CreatedAt   types.String  `tfsdk:"created_at"`
	UpdatedAt   types.String  `tfsdk:"updated_at"`
}

func NewSettingResource() resource.Resource {
	return &SettingResource{}
}

func (r *SettingResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_setting"
}

func (r *SettingResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a MisterShell application setting (a predefined key/value pair). Settings cannot be created or deleted — the key set is fixed by the backend registry. Create/Update PUT the value; Delete (terraform destroy) RESETS the key to its registry default rather than removing it. value uses its native Terraform type because settings are heterogeneous (bool/number/string). Secret keys are masked as '****' by the API and stored from config, not read back. See the valid keys table on this page.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Setting ID (mirrors key).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringUseStateForUnknown(),
				},
			},
			"key": schema.StringAttribute{
				Description: "Setting key (the resource ID). Must be a key in the backend settings registry; unknown keys return 404. Cannot be changed after creation.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"value": schema.DynamicAttribute{
				Description: "Setting value using its native Terraform type, for example true, 90, or \"localhost\". For secret keys the API masks the value as '****'; it is stored from config, not read back.",
				Required:    true,
				Sensitive:   true,
			},
			"description": schema.StringAttribute{
				Description: "Human description of the setting (from the registry).",
				Computed:    true,
			},
			"is_secret": schema.BoolAttribute{
				Description: "Whether the value is a secret (masked as '****' in responses).",
				Computed:    true,
			},
			"default": schema.DynamicAttribute{
				Description: "Registry default value using its native Terraform type. Masked as '****' for non-empty secret keys.",
				Computed:    true,
			},
			"schema": schema.DynamicAttribute{
				Description: "JSON Schema used by MisterShell to validate this setting.",
				Computed:    true,
			},
			"is_encrypted": schema.BoolAttribute{
				Description: "Whether the stored value is encrypted at rest.",
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

func (r *SettingResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *SettingResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan SettingResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	setting, err := r.putValue(ctx, &plan)
	if err != nil {
		resp.Diagnostics.AddError("Error updating setting", err.Error())
		return
	}
	if err := applySettingValue(setting, &plan); err != nil {
		resp.Diagnostics.AddError("Error decoding setting", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *SettingResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state SettingResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	setting, err := r.client.GetSetting(ctx, state.Key.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading setting", err.Error())
		return
	}

	savedValue := state.Value
	if err := mapSettingResponseToModel(setting, &state); err != nil {
		resp.Diagnostics.AddError("Error decoding setting", err.Error())
		return
	}
	if setting.IsSecret {
		// Secret value is masked **** — keep the user's plaintext from state.
		state.Value = savedValue
	} else {
		valueUnchanged, err := dynamicJSONEqual(savedValue, setting.Value)
		if err != nil {
			resp.Diagnostics.AddError("Error comparing setting value", err.Error())
			return
		}
		if valueUnchanged {
			state.Value = savedValue
		} else {
			state.Value, err = rawJSONToDynamic(setting.Value)
			if err != nil {
				resp.Diagnostics.AddError("Error decoding setting value", err.Error())
				return
			}
		}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *SettingResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan SettingResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	setting, err := r.putValue(ctx, &plan)
	if err != nil {
		resp.Diagnostics.AddError("Error updating setting", err.Error())
		return
	}
	if err := applySettingValue(setting, &plan); err != nil {
		resp.Diagnostics.AddError("Error decoding setting", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete resets the setting to its registry default — settings are never
// destroyed (the key set is fixed). The object persists server-side.
func (r *SettingResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state SettingResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	key := state.Key.ValueString()
	if _, err := r.client.ResetSetting(ctx, key); err != nil {
		if client.IsNotFound(err) {
			// Key no longer registered — nothing to reset.
			return
		}
		resp.Diagnostics.AddError("Error resetting setting", err.Error())
		return
	}
	tflog.Info(ctx, "mistershell_setting delete reset the setting to its registry default (settings cannot be destroyed)", map[string]interface{}{"key": key})
}

func (r *SettingResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("key"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

// putValue PUTs the planned value (shared by Create/Update).
func (r *SettingResource) putValue(ctx context.Context, plan *SettingResourceModel) (*client.SettingResponse, error) {
	value, err := dynamicToRawJSON(plan.Value)
	if err != nil {
		return nil, fmt.Errorf("value: %w", err)
	}
	input := client.SettingUpdateInput{Value: value}
	return r.client.UpdateSetting(ctx, plan.Key.ValueString(), input)
}

// applySettingValue maps the response into the model, preserving the planned
// value for all keys. Read reflects later non-secret drift from the server.
func applySettingValue(setting *client.SettingResponse, plan *SettingResourceModel) error {
	plannedValue := plan.Value
	if err := mapSettingResponseToModel(setting, plan); err != nil {
		return err
	}
	plan.Value = plannedValue
	return nil
}

func mapSettingResponseToModel(setting *client.SettingResponse, m *SettingResourceModel) error {
	m.ID = types.StringValue(setting.Key)
	m.Key = types.StringValue(setting.Key)
	// value is set by the caller per the secret/non-secret rules.
	m.Description = types.StringValue(setting.Description)
	m.IsSecret = types.BoolValue(setting.IsSecret)
	var err error
	m.Default, err = rawJSONToDynamic(setting.Default)
	if err != nil {
		return fmt.Errorf("default: %w", err)
	}
	m.Schema, err = rawJSONToDynamic(setting.Schema)
	if err != nil {
		return fmt.Errorf("schema: %w", err)
	}
	m.IsEncrypted = types.BoolValue(setting.IsEncrypted)
	m.CreatedAt = types.StringValue(setting.CreatedAt)
	m.UpdatedAt = types.StringValue(setting.UpdatedAt)
	return nil
}
