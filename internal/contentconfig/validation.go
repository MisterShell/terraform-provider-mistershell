package contentconfig

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.ResourceWithValidateConfig = (*Resource)(nil)
var _ resource.ResourceWithModifyPlan = (*Resource)(nil)

// Validate supplied payloads even when an ordinary metadata update would not use
// them. Omitted payloads remain valid configuration for imported objects.
func (r *Resource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	key := r.payloadAttribute()
	if key == "" {
		return
	}
	var value types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root(key), &value)...)
	if resp.Diagnostics.HasError() || value.IsNull() || value.IsUnknown() {
		return
	}
	if r.kind == "file_catalog_file" {
		if _, err := base64.StdEncoding.DecodeString(value.ValueString()); err != nil {
			resp.Diagnostics.AddAttributeError(path.Root(key), "Invalid file content", "content_base64 must contain valid standard base64. No file will be deleted or uploaded.")
		}
	} else if len(strings.TrimSpace(value.ValueString())) < 10 {
		resp.Diagnostics.AddAttributeError(path.Root(key), "Invalid license key", "license_key must contain a signed license of at least 10 characters. No license will be removed or installed.")
	}
}

func (r *Resource) payloadAttribute() string {
	switch r.kind {
	case "file_catalog_file":
		return "content_base64"
	case "license":
		return "license_key"
	default:
		return ""
	}
}

// A destroy-before-create replacement must have its creation input available at
// plan time. Validating only in Create is too late: Delete has already run then.
// Attribute plan modifiers do not expose their replacement decisions here, so
// compare the exact replacement fields owned by these two explicit resources.
func (r *Resource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	key := r.payloadAttribute()
	if key == "" || req.Plan.Raw.IsNull() {
		return // Explicit destroy needs no replacement payload.
	}
	replacing := req.State.Raw.IsNull()
	if !replacing {
		fields := []string{key}
		if r.kind == "file_catalog_file" {
			fields = append(fields, "content_type")
			var before, after types.Int64
			resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("location_id"), &before)...)
			resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("location_id"), &after)...)
			replacing = after.IsUnknown() || !before.Equal(after)
		}
		for _, field := range fields {
			var before, after types.String
			resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root(field), &before)...)
			resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root(field), &after)...)
			replacing = replacing || after.IsUnknown() || !before.Equal(after)
		}
	}
	if resp.Diagnostics.HasError() || !replacing {
		return
	}
	var value types.String
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root(key), &value)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if value.IsUnknown() && req.State.Raw.IsNull() {
		return // No existing object can be lost; Create validates resolved input.
	}
	if value.IsNull() || value.IsUnknown() {
		resp.Diagnostics.AddAttributeError(path.Root(key), "Missing replacement payload", key+" must be configured for creation, and known before replacing an existing object. Resolve generated replacement inputs in a separate apply first. Imported objects may omit this argument only while updating attributes that do not require replacement. Removing a configured payload does not relinquish management; remove the object from Terraform state instead.")
		return
	}
	// ModifyPlan can be called again after unknown configuration resolves; repeat
	// validation against the planned value before accepting a replacement.
	if r.kind == "file_catalog_file" {
		data, err := base64.StdEncoding.DecodeString(value.ValueString())
		if err != nil {
			resp.Diagnostics.AddAttributeError(path.Root(key), "Invalid file content", "content_base64 must contain valid standard base64 before replacement. The existing file is preserved.")
			return
		}
		if !req.State.Raw.IsNull() && r.client != nil {
			r.validateFileReplacement(ctx, int64(len(data)), resp)
		}
	} else if len(strings.TrimSpace(value.ValueString())) < 10 {
		resp.Diagnostics.AddAttributeError(path.Root(key), "Invalid license key", "license_key must contain a signed license of at least 10 characters before replacement. The existing license is preserved.")
	} else if !req.State.Raw.IsNull() && r.client != nil {
		confirmed := boolValue(ctx, req.Plan, "confirm_over_capacity")
		r.validateLicenseReplacement(ctx, value.ValueString(), confirmed, resp)
		if !resp.Diagnostics.HasError() && !confirmed && boolValue(ctx, req.State, "confirm_removal_over_capacity") {
			r.validateReplacementRemoval(ctx, stringValue(ctx, req.State, "id"), resp)
		}
	}
}

// These previews are advisory: the API revalidates at mutation time. Their
// purpose is to reject already-known failures before destroy-before-create.
func (r *Resource) validateFileReplacement(ctx context.Context, size int64, resp *resource.ModifyPlanResponse) {
	setting, err := r.client.GetSetting(ctx, "files_max_upload_bytes")
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("content_base64"), "Cannot validate file replacement", "Reading files_max_upload_bytes failed. File replacements require app.settings.read so the upload limit can be checked before deletion. Restore that permission or API availability and retry; the existing file is preserved.")
		return
	}
	var limit int64
	if err := json.Unmarshal(setting.Value, &limit); err != nil || limit <= 0 {
		resp.Diagnostics.AddAttributeError(path.Root("content_base64"), "Cannot validate file replacement", "The server returned an invalid files_max_upload_bytes limit. Correct the setting before retrying; the existing file is preserved.")
		return
	}
	if size > limit {
		resp.Diagnostics.AddAttributeError(path.Root("content_base64"), "Replacement file exceeds upload limit", fmt.Sprintf("Replacement content is %d bytes, exceeding files_max_upload_bytes (%d). Reduce the content or change the server limit before planning replacement; the existing file is preserved.", size, limit))
	}
}

func (r *Resource) validateLicenseReplacement(ctx context.Context, key string, confirmed bool, resp *resource.ModifyPlanResponse) {
	raw, err := r.client.ConfigurationRequest(ctx, "POST", "/api/v1/licenses/preview", map[string]any{"license_key": key})
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("license_key"), "Invalid replacement license", "License preview failed. Verify the signed key, its validity period, license write permission and API availability before retrying. The existing license is preserved; server details are withheld to protect the key.")
		return
	}
	var preview struct {
		AlreadyInstalled     *bool     `json:"already_installed"`
		RequiresConfirmation *bool     `json:"requires_confirmation"`
		IssuedAt             time.Time `json:"issued_at"`
		ExpiresAt            time.Time `json:"expires_at"`
	}
	if err := json.Unmarshal(raw, &preview); err != nil || preview.AlreadyInstalled == nil || preview.RequiresConfirmation == nil {
		resp.Diagnostics.AddAttributeError(path.Root("license_key"), "Invalid replacement license", "License preview returned an incomplete response. Replacement is refused and the existing license is preserved.")
		return
	}
	if *preview.AlreadyInstalled {
		resp.Diagnostics.AddAttributeError(path.Root("license_key"), "Replacement license already installed", "The supplied license is already installed. Replacing it would remove existing entitlement and fail creation. Keep the managed key unchanged; for an imported license keep license_key omitted. Manage a separately installed license by its own import identity.")
		return
	}
	// The API preview verifies signatures but deliberately skips install-time
	// checks. Mirror core/licensing/contract.py validate_for_install using the
	// verified claim dates so known expiry/skew failures precede deletion.
	now := time.Now().UTC()
	if preview.IssuedAt.IsZero() || preview.ExpiresAt.IsZero() || !now.Before(preview.ExpiresAt) || preview.IssuedAt.After(now.Add(5*time.Minute)) {
		resp.Diagnostics.AddAttributeError(path.Root("license_key"), "Invalid replacement license", "The verified license dates are missing, expired, or issued more than five minutes in the future. Replacement is refused and the existing license is preserved.")
		return
	}
	if *preview.RequiresConfirmation && !confirmed {
		resp.Diagnostics.AddAttributeError(path.Root("confirm_over_capacity"), "Replacement license requires confirmation", "The preview requires confirm_over_capacity=true. Review the projected capacity effect and explicitly confirm it before planning replacement. The existing license is preserved.")
	}
}

// Addition preview still includes the old license. If removal is explicitly
// permitted despite a deficit, require installation confirmation as well: the
// replacement's post-removal projection cannot be obtained atomically by API.
func (r *Resource) validateReplacementRemoval(ctx context.Context, id string, resp *resource.ModifyPlanResponse) {
	raw, err := r.client.ConfigurationRequest(ctx, "POST", "/api/v1/licenses/"+url.PathEscape(id)+"/removal-preview", nil)
	var preview struct {
		RequiresConfirmation *bool `json:"requires_confirmation"`
	}
	if err != nil || json.Unmarshal(raw, &preview) != nil || preview.RequiresConfirmation == nil {
		resp.Diagnostics.AddAttributeError(path.Root("confirm_over_capacity"), "Cannot validate replacement capacity", "Removal preview could not confirm replacement capacity. Restore API availability and permissions before retrying; the existing license is preserved.")
		return
	}
	if *preview.RequiresConfirmation {
		resp.Diagnostics.AddAttributeError(path.Root("confirm_over_capacity"), "Replacement license requires confirmation", "Removing the existing license leaves a capacity deficit. Installation preview still includes that license, so replacement also requires confirm_over_capacity=true before deletion. Review and explicitly accept this conservative confirmation; the existing license is preserved.")
	}
}
