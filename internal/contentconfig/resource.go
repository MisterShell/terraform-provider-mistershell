package contentconfig

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"net/url"
	"strconv"
	"strings"
	"terraform-provider-mistershell/internal/client"
	"time"
)

func (r *Resource) Create(ctx context.Context, q resource.CreateRequest, p *resource.CreateResponse) {
	state := tfsdk.State{Schema: q.Plan.Schema, Raw: q.Plan.Raw}
	var b json.RawMessage
	var e error
	switch r.kind {
	case "license":
		var key string
		key, e = knownString(ctx, q.Plan, "license_key")
		if e != nil {
			break
		}
		var preview json.RawMessage
		preview, e = r.client.ConfigurationRequest(ctx, "POST", "/api/v1/licenses/preview", map[string]any{"license_key": key})
		if e != nil {
			e = fmt.Errorf("license preview failed; verify the signed key and license write permission (server details withheld to protect the key)")
			break
		}
		var v struct {
			Already      bool `json:"already_installed"`
			Confirmation bool `json:"requires_confirmation"`
		}
		e = json.Unmarshal(preview, &v)
		if e != nil {
			break
		}
		if v.Already {
			e = fmt.Errorf("license is already installed; import it by license ID with license_key omitted to explicitly adopt ownership")
			break
		}
		if v.Confirmation && !boolValue(ctx, q.Plan, "confirm_over_capacity") {
			e = fmt.Errorf("installation requires confirm_over_capacity=true")
			break
		}
		b, e = r.client.ConfigurationRequest(ctx, "POST", "/api/v1/licenses", map[string]any{"license_key": key, "confirm_over_capacity": boolValue(ctx, q.Plan, "confirm_over_capacity")})
		if e != nil {
			e = fmt.Errorf("license installation failed; verify key, capacity confirmation and write permission (server details withheld to protect the key)")
			break
		}
		var o map[string]json.RawMessage
		o, e = object(b)
		if e == nil {
			id := jsonString(o["license_id"])
			if id == "" {
				e = fmt.Errorf("installation returned no license ID")
				break
			}
			p.Diagnostics.Append(set(ctx, &state, "id", id)...)
			p.Diagnostics.Append(set(ctx, &state, "metadata", types.DynamicNull())...)
			p.State = state
			b, e = licenseEntry(ctx, r.client, id)
			if e == nil && b == nil {
				e = fmt.Errorf("installed license is not yet visible; installation identity retained for recovery")
			}
		}
	case "file_catalog_folder":
		b, e = r.client.ConfigurationRequest(ctx, "POST", "/api/v1/files/catalog/folders", map[string]any{"location_id": intValue(ctx, q.Plan, "location_id"), "parent_path": stringValue(ctx, q.Plan, "parent_path"), "name": stringValue(ctx, q.Plan, "name")})
	case "file_catalog_file":
		var content string
		content, e = knownString(ctx, q.Plan, "content_base64")
		if e != nil {
			break
		}
		var data []byte
		data, e = base64.StdEncoding.DecodeString(content)
		if e != nil {
			e = fmt.Errorf("content_base64 must contain valid standard base64")
			break
		}
		params := url.Values{"location_id": {strconv.FormatInt(intValue(ctx, q.Plan, "location_id"), 10)}, "parent_path": {stringValue(ctx, q.Plan, "parent_path")}, "name": {stringValue(ctx, q.Plan, "name")}}
		if ct := stringValue(ctx, q.Plan, "content_type"); ct != "" {
			params.Set("content_type", ct)
		}
		b, e = r.client.ConfigurationUpload(ctx, "/api/v1/files/catalog/upload?"+params.Encode(), bytes.NewReader(data))
	case "tree_element":
		var config json.RawMessage
		config, e = dynamicValue(ctx, q.Plan, "config")
		if e != nil {
			break
		}
		body := map[string]any{"element_type": stringValue(ctx, q.Plan, "element_type"), "label": stringValue(ctx, q.Plan, "label"), "config": config}
		if parent := stringValue(ctx, q.Plan, "parent_key"); parent != "" {
			body["parent_key"] = parent
		}
		b, e = r.client.ConfigurationRequest(ctx, "POST", "/api/v1/tree-elements", body)
	case "note":
		var np string
		np, e = notePath(stringValue(ctx, q.Plan, "scope_type"), intValue(ctx, q.Plan, "scope_id"))
		if e != nil {
			break
		}
		var version int64
		var locked bool
		_, version, locked, e = latestNote(ctx, r.client, np)
		if e != nil {
			break
		}
		if locked {
			e = fmt.Errorf("note already has an editor; refusing to refresh or release another session's user-owned lock")
			break
		}
		b, e = r.writeNote(ctx, np, version, stringValue(ctx, q.Plan, "content"))
		if len(b) > 0 {
			p.Diagnostics.Append(set(ctx, &state, "id", stringValue(ctx, q.Plan, "scope_type")+":"+strconv.FormatInt(intValue(ctx, q.Plan, "scope_id"), 10))...)
		}
	}
	// Persist the successful mutation even if advisory-lock cleanup failed.
	if len(b) > 0 {
		p.Diagnostics.Append(r.refresh(ctx, &state, b)...)
		p.State = state
	}
	if e != nil {
		p.Diagnostics.AddError("Error creating "+r.kind, e.Error())
	}
}
func (r *Resource) Read(ctx context.Context, q resource.ReadRequest, p *resource.ReadResponse) {
	p.State = q.State
	var b json.RawMessage
	var e error
	id := stringValue(ctx, q.State, "id")
	switch r.kind {
	case "license":
		b, e = licenseEntry(ctx, r.client, id)
		if e == nil && b == nil {
			p.State.RemoveResource(ctx)
			return
		}
	case "file_catalog_folder", "file_catalog_file":
		b, e = catalogEntry(ctx, r.client, intValue(ctx, q.State, "location_id"), id)
	case "tree_element":
		b, e = treeEntry(ctx, r.client, id)
	case "note":
		var np string
		np, e = notePath(stringValue(ctx, q.State, "scope_type"), intValue(ctx, q.State, "scope_id"))
		if e == nil {
			b, _, _, e = latestNote(ctx, r.client, np)
			if e == nil && b == nil {
				p.State.RemoveResource(ctx)
				return
			}
		}
	}
	if e != nil {
		p.Diagnostics.AddError("Error reading "+r.kind, e.Error())
		return
	}
	p.Diagnostics.Append(r.refresh(ctx, &p.State, b)...)
}
func (r *Resource) Update(ctx context.Context, q resource.UpdateRequest, p *resource.UpdateResponse) {
	p.State = q.State
	id := stringValue(ctx, q.State, "id")
	var b json.RawMessage
	var e error
	switch r.kind {
	case "license":
		b, e = licenseEntry(ctx, r.client, id)
		if e == nil && len(b) == 0 {
			e = fmt.Errorf("license disappeared before update; refresh and review")
		}
	case "file_catalog_folder", "file_catalog_file":
		b, e = r.client.ConfigurationRequest(ctx, "PATCH", "/api/v1/files/catalog/"+url.PathEscape(id), map[string]any{"name": stringValue(ctx, q.Plan, "name"), "parent_path": stringValue(ctx, q.Plan, "parent_path")})
	case "tree_element":
		var config json.RawMessage
		config, e = dynamicValue(ctx, q.Plan, "config")
		if e == nil {
			b, e = r.client.ConfigurationRequest(ctx, "PATCH", "/api/v1/tree-elements/"+url.PathEscape(id), map[string]any{"label": stringValue(ctx, q.Plan, "label"), "config": config})
		}
	case "note":
		var np string
		np, e = notePath(stringValue(ctx, q.Plan, "scope_type"), intValue(ctx, q.Plan, "scope_id"))
		if e == nil {
			b, e = r.writeNote(ctx, np, intValue(ctx, q.State, "version"), stringValue(ctx, q.Plan, "content"))
		}
	}
	if len(b) > 0 {
		p.State = tfsdk.State{Schema: q.Plan.Schema, Raw: q.Plan.Raw}
		p.Diagnostics.Append(set(ctx, &p.State, "id", id)...)
		p.Diagnostics.Append(r.refresh(ctx, &p.State, b)...)
	}
	if e != nil {
		p.Diagnostics.AddError("Error updating "+r.kind, e.Error())
	}
}
func (r *Resource) Delete(ctx context.Context, q resource.DeleteRequest, p *resource.DeleteResponse) {
	if r.kind == "note" {
		return
	}
	id := stringValue(ctx, q.State, "id")
	var e error
	switch r.kind {
	case "license":
		var b json.RawMessage
		b, e = r.client.ConfigurationRequest(ctx, "POST", "/api/v1/licenses/"+url.PathEscape(id)+"/removal-preview", nil)
		if client.IsNotFound(e) {
			return
		}
		if e == nil {
			var v struct {
				RequiresConfirmation bool `json:"requires_confirmation"`
			}
			e = json.Unmarshal(b, &v)
			if e == nil && v.RequiresConfirmation && !boolValue(ctx, q.State, "confirm_removal_over_capacity") {
				e = fmt.Errorf("license removal requires confirm_removal_over_capacity=true; apply that flag before destroying")
			}
			if e == nil {
				_, e = r.client.ConfigurationRequest(ctx, "DELETE", "/api/v1/licenses/"+url.PathEscape(id)+"?confirm_over_capacity="+strconv.FormatBool(boolValue(ctx, q.State, "confirm_removal_over_capacity")), nil)
			}
		}
	case "file_catalog_folder", "file_catalog_file":
		var b json.RawMessage
		b, e = catalogEntry(ctx, r.client, intValue(ctx, q.State, "location_id"), id)
		if e == nil && r.kind == "file_catalog_folder" {
			var o map[string]json.RawMessage
			o, e = object(b)
			if e == nil {
				var rows []json.RawMessage
				rows, e = catalogEntries(ctx, r.client, intValue(ctx, q.State, "location_id"), jsonString(o["path"]))
				if e == nil && len(rows) > 0 {
					e = fmt.Errorf("folder is not empty; refusing recursive deletion. Destroy managed children first and remove unowned contents explicitly")
				}
			}
		}
		if e == nil {
			_, e = r.client.ConfigurationRequest(ctx, "DELETE", "/api/v1/files/catalog/"+url.PathEscape(id), nil)
		}
	case "tree_element":
		_, e = treeEntry(ctx, r.client, id)
		if e == nil {
			_, e = r.client.ConfigurationRequest(ctx, "DELETE", "/api/v1/tree-elements/"+url.PathEscape(id), nil)
		}
	}
	if e != nil {
		p.Diagnostics.AddError("Error deleting "+r.kind, e.Error())
	}
}
func (r *Resource) ImportState(ctx context.Context, q resource.ImportStateRequest, p *resource.ImportStateResponse) {
	id := q.ID
	switch r.kind {
	case "license":
		if id == "" {
			p.Diagnostics.AddError("Invalid import ID", "License ID must not be empty.")
			return
		}
		p.Diagnostics.Append(set(ctx, &p.State, "confirm_over_capacity", false)...)
		p.Diagnostics.Append(set(ctx, &p.State, "confirm_removal_over_capacity", false)...)
	case "file_catalog_folder", "file_catalog_file":
		loc, entry, e := splitCatalogImport(id)
		if e != nil {
			p.Diagnostics.AddError("Invalid import ID", e.Error())
			return
		}
		id = entry
		p.Diagnostics.Append(set(ctx, &p.State, "location_id", loc)...)
	case "tree_element":
		if e := numericID(id); e != nil {
			p.Diagnostics.AddError("Invalid import ID", e.Error())
			return
		}
	case "note":
		parts := strings.Split(id, ":")
		if len(parts) != 2 {
			p.Diagnostics.AddError("Invalid import ID", "Expected resource:ID or location:ID.")
			return
		}
		n, e := strconv.ParseInt(parts[1], 10, 64)
		if e == nil {
			_, e = notePath(parts[0], n)
		}
		if e != nil {
			p.Diagnostics.AddError("Invalid import ID", e.Error())
			return
		}
		p.Diagnostics.Append(set(ctx, &p.State, "scope_type", parts[0])...)
		p.Diagnostics.Append(set(ctx, &p.State, "scope_id", n)...)
	}
	p.Diagnostics.Append(set(ctx, &p.State, "id", id)...)
}

func (r *Resource) refresh(ctx context.Context, s *tfsdk.State, b json.RawMessage) diag.Diagnostics {
	var d diag.Diagnostics
	o, e := object(b)
	if e != nil {
		d.AddError("Invalid API response", e.Error())
		return d
	}
	if v, ok := o["id"]; ok {
		d.Append(set(ctx, s, "id", jsonString(v))...)
	}
	switch r.kind {
	case "license":
		d.Append(setDynamic(ctx, s, "metadata", b)...)
	case "file_catalog_folder", "file_catalog_file":
		expected := "folder"
		if r.kind == "file_catalog_file" {
			expected = "file"
		}
		if jsonString(o["kind"]) != expected {
			d.AddError("Catalog kind mismatch", "The selected catalog entry is not a "+expected+".")
			return d
		}
		d.Append(set(ctx, s, "name", jsonString(o["name"]))...)
		full := jsonString(o["path"])
		parent := ""
		if i := strings.LastIndex(full, "/"); i >= 0 {
			parent = full[:i]
		}
		d.Append(set(ctx, s, "parent_path", parent)...)
		d.Append(set(ctx, s, "path", full)...)
		d.Append(setDynamic(ctx, s, "metadata", b)...)
		if r.kind == "file_catalog_file" {
			checksum := jsonString(o["sha256"])
			d.Append(set(ctx, s, "sha256", checksum)...)
			var content types.String
			d.Append(s.GetAttribute(ctx, path.Root("content_base64"), &content)...)
			if !content.IsNull() && !content.IsUnknown() {
				data, e := base64.StdEncoding.DecodeString(content.ValueString())
				if e != nil {
					d.AddError("Invalid file content", "content_base64 must be standard base64.")
				} else {
					sum := sha256.Sum256(data)
					if hex.EncodeToString(sum[:]) != checksum {
						d.Append(set(ctx, s, "content_base64", types.StringNull())...)
					}
				}
			}
		}
	case "tree_element":
		d.Append(set(ctx, s, "element_type", jsonString(o["element_type"]))...)
		label := jsonString(o["label"])
		if label == "null" {
			label = ""
		}
		d.Append(set(ctx, s, "label", label)...)
		var parent *string
		if v := o["parent_key"]; len(v) > 0 {
			_ = json.Unmarshal(v, &parent)
		} else if n := jsonInt(o["parent_id"]); n > 0 {
			v := "location:" + strconv.FormatInt(n, 10)
			parent = &v
		}
		d.Append(set(ctx, s, "parent_key", types.StringPointerValue(parent))...)
		config := o["config"]
		if len(config) == 0 {
			config = raw(nil)
		}
		d.Append(reconcileConfig(ctx, s, config)...)
	case "note":
		d.Append(set(ctx, s, "content", jsonString(o["content"]))...)
		d.Append(set(ctx, s, "version", jsonInt(o["version"]))...)
	}
	return d
}

// The server uses user-owned advisory locks and has no atomic version predicate.
// Never acquire an existing lock, even for our API user's other editor session.
func (r *Resource) writeNote(ctx context.Context, np string, expected int64, content string) (result json.RawMessage, err error) {
	_, version, locked, e := latestNote(ctx, r.client, np)
	if e != nil {
		return nil, e
	}
	if locked {
		return nil, fmt.Errorf("note already has an editor; use a dedicated API user and retry after the editor releases its lock")
	}
	if version != expected {
		return nil, fmt.Errorf("note changed from version %d to %d since planning; refresh and review before applying", expected, version)
	}
	if _, e = r.client.ConfigurationRequest(ctx, "POST", np+"/lock", nil); e != nil {
		return nil, e
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		if _, e := r.client.ConfigurationRequest(cleanup, "DELETE", np+"/lock", nil); e != nil && err == nil {
			err = fmt.Errorf("note mutation completed but lock cleanup failed; lock expires automatically: %w", e)
		}
	}()
	_, version, _, e = latestNote(ctx, r.client, np)
	if e != nil {
		return nil, e
	}
	if version != expected {
		return nil, fmt.Errorf("note changed while acquiring its edit lock; refresh and review before applying")
	}
	result, err = r.client.ConfigurationRequest(ctx, "PUT", np, map[string]any{"content_type": "text/markdown", "content": content})
	if err != nil {
		err = fmt.Errorf("note write failed; verify write permission, content length and API availability (server details withheld to protect note content)")
	}
	return result, err
}
