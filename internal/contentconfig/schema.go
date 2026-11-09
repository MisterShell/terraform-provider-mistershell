// Package contentconfig implements the non-CRUD content and licensing workflows.
package contentconfig

import (
	"context"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	ds "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rs "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"terraform-provider-mistershell/internal/client"
)

var resourceKinds = []string{"license", "file_catalog_folder", "file_catalog_file", "tree_element", "note"}
var dataKinds = []string{"license", "license_status", "file_catalog", "file_catalog_folder", "file_catalog_file", "tree_element", "note"}

func Resources() []func() resource.Resource {
	out := []func() resource.Resource{}
	for _, k := range resourceKinds {
		kind := k
		out = append(out, func() resource.Resource { return &Resource{kind: kind} })
	}
	return out
}
func DataSources() []func() datasource.DataSource {
	out := []func() datasource.DataSource{}
	for _, k := range dataKinds {
		kind := k
		out = append(out, func() datasource.DataSource { return &DataSource{kind: kind} })
	}
	return out
}

type Resource struct {
	kind   string
	client *client.Client
}
type DataSource struct {
	kind   string
	client *client.Client
}

func (r *Resource) Metadata(_ context.Context, q resource.MetadataRequest, p *resource.MetadataResponse) {
	p.TypeName = q.ProviderTypeName + "_" + r.kind
}
func (d *DataSource) Metadata(_ context.Context, q datasource.MetadataRequest, p *datasource.MetadataResponse) {
	p.TypeName = q.ProviderTypeName + "_" + d.kind
}
func (r *Resource) Configure(_ context.Context, q resource.ConfigureRequest, p *resource.ConfigureResponse) {
	if q.ProviderData == nil {
		return
	}
	var ok bool
	r.client, ok = q.ProviderData.(*client.Client)
	if !ok {
		p.Diagnostics.AddError("Invalid provider configuration", "Expected MisterShell API client.")
	}
}
func (d *DataSource) Configure(_ context.Context, q datasource.ConfigureRequest, p *datasource.ConfigureResponse) {
	if q.ProviderData == nil {
		return
	}
	var ok bool
	d.client, ok = q.ProviderData.(*client.Client)
	if !ok {
		p.Diagnostics.AddError("Invalid provider configuration", "Expected MisterShell API client.")
	}
}
func str(description string, required, replace bool) rs.StringAttribute {
	a := rs.StringAttribute{Description: description, Required: required, Optional: !required}
	if replace {
		a.PlanModifiers = []planmodifier.String{stringplanmodifier.RequiresReplace()}
	}
	return a
}
func positive(description string, replace bool) rs.Int64Attribute {
	a := rs.Int64Attribute{Description: description, Required: true, Validators: []validator.Int64{int64validator.AtLeast(1)}}
	if replace {
		a.PlanModifiers = []planmodifier.Int64{int64planmodifier.RequiresReplace()}
	}
	return a
}
func resourceFields(kind string) map[string]rs.Attribute {
	a := map[string]rs.Attribute{"id": rs.StringAttribute{Computed: true, Description: "API identity. Catalog imports use location_id:entry_id; notes use scope_type:scope_id."}}
	switch kind {
	case "license":
		k := str("Signed license. Required for new installation; omit for imported licenses. Sensitive but stored in Terraform state. Changing it replaces the installation. Replacement requires a known key, successful server preview and valid signed dates checked against the local clock; removing a configured key is rejected before deletion.", false, true)
		k.Sensitive = true
		a["license_key"] = k
		for _, n := range []string{"confirm_over_capacity", "confirm_removal_over_capacity"} {
			a[n] = rs.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), Description: "Explicitly authorize an over-capacity license change at installation or removal, respectively. Defaults to false. If removal is already authorized despite a capacity deficit, replacement conservatively also requires confirm_over_capacity because the installation preview still includes the old license."}
		}
		a["metadata"] = rs.DynamicAttribute{Computed: true, Description: "Sanitized stored-license summary; never contains the signed license."}
	case "file_catalog_folder", "file_catalog_file":
		a["location_id"] = positive("Location containing this entry; requires file read/write/delete scope.", true)
		a["name"] = str("Entry name, without path separators.", true, false)
		a["parent_path"] = rs.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString(""), Description: "Parent catalog path. Empty means the location root. Create parent folders first."}
		a["path"] = rs.StringAttribute{Computed: true, Description: "Full catalog path."}
		a["metadata"] = rs.DynamicAttribute{Computed: true, Description: "Entry metadata: id, location_id, path, name, kind, size, sha256, content_type, uploaded_by, created_at and updated_at."}
		if kind == "file_catalog_file" {
			k := str("Base64-encoded file bytes, required when creating. Sensitive but persisted in state. Content changes replace the file; default destroy-before-create is required at the same path. Replacement requires known valid base64 and a readable files_max_upload_bytes limit (app.settings.read) at plan time; removing configured bytes or exceeding that limit is rejected before deletion.", false, true)
			k.Sensitive = true
			a["content_base64"] = k
			a["content_type"] = str("Optional MIME type recorded with the upload. Changes replace the file.", false, true)
			a["sha256"] = rs.StringAttribute{Computed: true, Description: "SHA-256 of stored bytes; a mismatch with configured bytes triggers replacement."}
		}
	case "tree_element":
		a["parent_key"] = str("Parent location:<id>; omit for root. Changing placement replaces this element.", false, true)
		t := str("Presentation type: line or label.", true, true)
		t.Validators = []validator.String{stringvalidator.OneOf("line", "label")}
		a["element_type"] = t
		a["label"] = rs.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString(""), Description: "Display label. Empty means no text."}
		a["config"] = rs.DynamicAttribute{Optional: true, Description: "Native object. Optional keys: variant (badge, text); icon (label, flag, star, bookmark, folder_special, dns, cloud, warning, bolt, lan, hub, router, storage, shield, groups); color (primary, secondary, positive, negative, warning, info, purple, teal, indigo, orange, grey-7)."}
	case "note":
		t := str("Scope: resource or location.", true, true)
		t.Validators = []validator.String{stringvalidator.OneOf("resource", "location")}
		a["scope_type"] = t
		a["scope_id"] = positive("Positive scope ID; requires notes permission and access to the scope.", true)
		a["content"] = rs.StringAttribute{Required: true, Sensitive: true, Description: "Markdown content. Sensitive but stored in Terraform state."}
		a["version"] = rs.Int64Attribute{Computed: true, Description: "Latest observed version, used for optimistic checks before writing."}
	}
	return a
}
func (r *Resource) Schema(_ context.Context, _ resource.SchemaRequest, p *resource.SchemaResponse) {
	desc := "Manages a MisterShell " + r.kind + ". Requires the corresponding read, write and delete API permissions."
	switch r.kind {
	case "note":
		desc = "Manages a versioned Markdown note. Acquires an advisory edit lock and rechecks the version before writing. Locks belong to the API key's user: use a dedicated user and serialize writers. The backend has no atomic conditional PUT; clients ignoring locks can still race. Destroy relinquishes management and preserves the current note and all history, including adopted notes. Import scope_type:scope_id."
	case "file_catalog_folder":
		desc += " Destroy refuses a nonempty folder because backend deletion is recursive. This check is not atomic; serialize catalog writers. Import location_id:entry_id. Missing entries fail refresh because listing hides permission loss; reconcile access or remove confirmed-deleted objects from state manually."
	case "file_catalog_file":
		desc += " Upload uses raw bytes. Import location_id:entry_id. Missing entries fail refresh because listing hides permission loss; reconcile access or remove confirmed-deleted objects from state manually."
	case "tree_element":
		desc += " Read uses the scoped browse tree. Missing entries fail refresh because scope loss and deletion are indistinguishable. Import the numeric element ID."
	case "license":
		desc += " Preview and final mutation both enforce over-capacity confirmation. A 202 response is already durably committed. Existing installations must be imported by license ID, with license_key omitted."
	}
	p.Schema = rs.Schema{Description: desc, Attributes: resourceFields(r.kind)}
}
func (d *DataSource) Schema(_ context.Context, _ datasource.SchemaRequest, p *datasource.SchemaResponse) {
	a := map[string]ds.Attribute{}
	switch d.kind {
	case "license":
		a["id"] = ds.StringAttribute{Required: true, Description: "Stored license ID."}
		a["metadata"] = ds.DynamicAttribute{Computed: true, Description: "Sanitized stored-license summary."}
	case "license_status":
		a["id"] = ds.StringAttribute{Computed: true, Description: "Singleton identity."}
		a["projection"] = ds.DynamicAttribute{Computed: true, Description: "Entitlement projection including available, unavailable_reason, core_feature, inherited, extras, capabilities, capacity, licensed_to, warning_level, licensing_email, warning_licenses, ha, instance_id, revision, reconciled_at and licenses. available=false means an unavailable snapshot, not Free entitlement."}
	case "file_catalog":
		a["id"] = ds.StringAttribute{Computed: true, Description: "Catalog identity."}
		a["location_id"] = ds.Int64Attribute{Required: true, Description: "Location ID."}
		a["parent_path"] = ds.StringAttribute{Optional: true, Description: "Folder path; omitted means root."}
		a["entries"] = ds.DynamicAttribute{Computed: true, Description: "All directly contained catalog nodes, including child-location navigation nodes; does not recurse."}
	case "file_catalog_folder", "file_catalog_file":
		a["id"] = ds.StringAttribute{Required: true, Description: "Numeric catalog entry ID."}
		a["location_id"] = ds.Int64Attribute{Required: true, Description: "Containing location ID."}
		a["metadata"] = ds.DynamicAttribute{Computed: true, Description: "Entry metadata including checksum, size, content type and uploader; does not download content."}
	case "tree_element":
		a["id"] = ds.StringAttribute{Required: true, Description: "Numeric element ID."}
		a["metadata"] = ds.DynamicAttribute{Computed: true, Description: "Element from the scoped browse tree including parent_key, element_type, label and config."}
	case "note":
		a["id"] = ds.StringAttribute{Computed: true, Description: "scope_type:scope_id."}
		a["scope_type"] = ds.StringAttribute{Required: true, Validators: []validator.String{stringvalidator.OneOf("resource", "location")}, Description: "resource or location."}
		a["scope_id"] = ds.Int64Attribute{Required: true, Description: "Positive scope ID."}
		a["content"] = ds.StringAttribute{Computed: true, Sensitive: true, Description: "Latest Markdown content, or null when no version exists."}
		a["version"] = ds.Int64Attribute{Computed: true, Description: "Latest version number; zero when no version exists."}
	}
	p.Schema = ds.Schema{Description: "Reads MisterShell " + d.kind + " through the authenticated API. Permissions and scope are enforced by the server.", Attributes: a}
}
