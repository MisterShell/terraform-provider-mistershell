package configbindings

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"terraform-provider-mistershell/internal/client"
	"time"
)

type api interface {
	ConfigurationRequest(context.Context, string, string, any) (json.RawMessage, error)
	ConfigurationList(context.Context, string, url.Values) ([]json.RawMessage, error)
}
type engine struct {
	api api
	def definition
}
type object = map[string]json.RawMessage

func decode(raw json.RawMessage) (object, error) {
	var obj object
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, fmt.Errorf("invalid configuration response")
	}
	if obj == nil {
		return nil, fmt.Errorf("expected configuration object")
	}
	return obj, nil
}
func jsonValue(v any) json.RawMessage      { b, _ := json.Marshal(v); return b }
func textValue(raw json.RawMessage) string { var s string; _ = json.Unmarshal(raw, &s); return s }
func (e engine) itemPath(id string) string {
	if e.def.name == "local_auth_policy" {
		return e.def.path
	}
	p := e.def.path + "/" + url.PathEscape(id)
	if e.def.customizations {
		p += "/customizations"
	}
	return p
}
func validID(d definition, id string) error {
	if id == "" || strings.TrimSpace(id) != id || strings.ContainsAny(id, "/?#") || id == "." || id == ".." {
		return fmt.Errorf("provide a nonempty stable object ID or key without path separators")
	}
	if d.name == "local_auth_policy" && id != "local" {
		return fmt.Errorf("Local policy ID must be local")
	}
	if d.lookup != "" {
		n, err := strconv.ParseInt(id, 10, 64)
		if err != nil || n < 1 {
			return fmt.Errorf("provide a positive numeric object ID")
		}
	}
	return nil
}
func (e engine) read(ctx context.Context, id string) (object, error) {
	if err := validID(e.def, id); err != nil {
		return nil, err
	}
	if e.def.customizations {
		rows, err := e.api.ConfigurationList(ctx, e.itemPath(id), nil)
		if err != nil {
			return nil, err
		}
		items := make([]object, 0, len(rows))
		for _, row := range rows {
			obj, err := decode(row)
			if err != nil {
				return nil, err
			}
			delete(obj, "position")
			items = append(items, obj)
		}
		return object{"items": jsonValue(items)}, nil
	}
	var raw json.RawMessage
	var err error
	if e.def.listOnly {
		rows, listErr := e.api.ConfigurationList(ctx, e.def.path, nil)
		if listErr != nil {
			return nil, listErr
		}
		field := "key"
		if e.def.name == "builtin_sensor_source" {
			field = "id"
		}
		for _, row := range rows {
			obj, err := decode(row)
			if err != nil {
				return nil, err
			}
			identity := textValue(obj[field])
			if identity == "" {
				identity = string(obj[field])
			}
			if identity == id {
				raw = row
				break
			}
		}
		if raw == nil {
			return nil, &client.NotFoundError{Path: e.itemPath(id)}
		}
	} else {
		raw, err = e.api.ConfigurationRequest(ctx, "GET", e.itemPath(id), nil)
		if err != nil {
			return nil, err
		}
	}
	obj, err := decode(raw)
	if err != nil {
		return nil, err
	}
	if e.def.name == "builtin_sensor_source" && textValue(obj["source_type"]) != "index" {
		return nil, fmt.Errorf("only index sources support built-in bindings; custom sources belong to mistershell_sensor_source")
	}
	return obj, nil
}
func validate(d definition, body object) error {
	if len(body) == 0 {
		return fmt.Errorf("configuration must own at least one supported field")
	}
	for key, raw := range body {
		kind, ok := d.fields[key]
		if !ok {
			return fmt.Errorf("unsupported configuration field %q", key)
		}
		var value any
		if err := json.Unmarshal(raw, &value); err != nil {
			return fmt.Errorf("invalid field %q", key)
		}
		valid := false
		switch kind {
		case "bool":
			_, valid = value.(bool)
		case "string":
			s, ok := value.(string)
			valid = ok && len(s) <= 255
		case "object", "nullable_object":
			_, valid = value.(map[string]any)
			valid = valid || (kind == "nullable_object" && value == nil)
		case "integer", "interval", "timeout":
			n, ok := value.(float64)
			valid = ok && n == float64(int64(n))
			if kind == "interval" {
				valid = valid && n >= 1
			}
			if kind == "timeout" {
				valid = valid && n >= 60 && n <= 86400
			}
		case "datetime":
			s, ok := value.(string)
			_, err := time.Parse(time.RFC3339Nano, s)
			valid = value == nil || (ok && err == nil)
		case "mfa":
			s, ok := value.(string)
			valid = ok && (s == "none" || s == "totp")
		case "parameters":
			params, ok := value.(map[string]any)
			valid = ok
			for _, v := range params {
				s, ok := v.(string)
				valid = valid && ok && strings.TrimSpace(s) != "" && s != "****" && s != "***"
			}
		case "items":
			if err := validateItems(d, value); err != nil {
				return err
			}
			valid = true
		}
		if !valid {
			return fmt.Errorf("invalid %s: expected %s (email MFA requires interactive proof; blank or masked parameters cannot clear secrets)", key, kind)
		}
	}
	return nil
}
func validateItems(d definition, value any) error {
	items, ok := value.([]any)
	if !ok {
		return fmt.Errorf("items must be an ordered list")
	}
	seen := map[float64]bool{}
	for _, v := range items {
		row, ok := v.(map[string]any)
		if !ok {
			return fmt.Errorf("customizations must be objects")
		}
		n, ok := row["tag_id"].(float64)
		if !ok || n <= 0 || n != float64(int64(n)) || seen[n] {
			return fmt.Errorf("customizations require unique positive integer tag_id values")
		}
		seen[n] = true
		for key, v := range row {
			switch key {
			case "tag_id":
			case "skip_collection":
				if _, ok := v.(bool); !ok {
					return fmt.Errorf("skip_collection must be boolean")
				}
			case "thresholds":
				if d.name != "metric_customizations" {
					return fmt.Errorf("snap customizations do not accept thresholds")
				}
				if _, ok := v.(map[string]any); !ok {
					return fmt.Errorf("thresholds must be an object")
				}
			default:
				return fmt.Errorf("unsupported customization field %q", key)
			}
		}
		if d.name == "metric_customizations" {
			if _, ok := row["thresholds"]; !ok {
				return fmt.Errorf("metric customizations require thresholds")
			}
		}
	}
	return nil
}

// prepare preserves only fields owned by the binding. Omission releases a field
// by restoring its captured original, rather than sending a destructive null.
func prepare(d definition, desired, original, current, restoreParams object) (object, object, error) {
	if err := validate(d, desired); err != nil {
		return nil, nil, err
	}
	body := object{}
	snapshot := object{}
	if d.name == "scheduled_task_config" {
		_, enabled := desired["is_enabled"]
		_, interval := desired["interval_minutes"]
		baseline, captured := original["_scheduled_next_run"]
		if enabled || interval {
			if !captured {
				var exists bool
				baseline, exists = original["next_run"]
				if !exists {
					baseline, exists = current["next_run"]
				}
				if !exists {
					return nil, nil, fmt.Errorf("API response omitted scheduler next_run restoration value")
				}
			}
			snapshot["_scheduled_next_run"] = baseline
		} else if captured {
			body["next_run"] = baseline
		}
	}
	for key, raw := range desired {
		baseline, owned := original[key]
		if !owned {
			var exists bool
			baseline, exists = current[key]
			if !exists {
				return nil, nil, fmt.Errorf("API response omitted restorable field %q", key)
			}
		}
		if d.name == "scheduled_task_config" && key == "next_run" && !owned {
			if saved, ok := original["_scheduled_next_run"]; ok {
				baseline = saved
			}
		}
		if d.name == "scheduled_task_config" && key == "interval_minutes" {
			var n int64
			if json.Unmarshal(baseline, &n) != nil || n < 1 {
				return nil, nil, fmt.Errorf("cannot own an original non-recurring interval: the API cannot restore interval_minutes below 1")
			}
		}
		if d.name == "local_auth_policy" && textValue(baseline) == "email" {
			return nil, nil, fmt.Errorf("cannot bind email MFA: restoring it requires a new interactive activation proof")
		}
		if key == "parameters" && d.name == "builtin_sensor_source" {
			var wanted, stored, saved map[string]string
			_ = json.Unmarshal(raw, &wanted)
			_ = json.Unmarshal(current[key], &stored)
			_ = json.Unmarshal(baseline, &saved)
			captured := map[string]string{}
			for k := range wanted {
				if _, exists := stored[k]; !exists {
					return nil, nil, fmt.Errorf("cannot add Sensor parameter %q: the backend cannot remove it on destroy", k)
				}
				old := saved[k]
				if !owned || old == "" || old == "****" || old == "***" {
					old = textValue(restoreParams[k])
				}
				if old == "" || old == "****" || old == "***" {
					return nil, nil, fmt.Errorf("restore_parameters must supply the original value for masked Sensor parameter %q", k)
				}
				captured[k] = old
			}
			// Removing a key restores its original secret before releasing ownership.
			for k, old := range saved {
				if _, stillOwned := wanted[k]; owned && !stillOwned {
					wanted[k] = old
				}
			}
			raw = jsonValue(wanted)
			baseline = jsonValue(captured)
		}
		if d.name == "metric_customizations" {
			if err := restorableThresholds(baseline, raw); err != nil {
				return nil, nil, err
			}
		}
		body[key] = raw
		snapshot[key] = baseline
	}
	for key, baseline := range original {
		if key == "_scheduled_next_run" {
			continue
		}
		if _, stillOwned := desired[key]; !stillOwned {
			body[key] = baseline
		}
	}
	return body, snapshot, nil
}
func restorableThresholds(original, desired json.RawMessage) error {
	var old, next []struct {
		Tag        int64          `json:"tag_id"`
		Thresholds map[string]any `json:"thresholds"`
	}
	_ = json.Unmarshal(original, &old)
	_ = json.Unmarshal(desired, &next)
	for _, a := range old {
		for _, b := range next {
			if a.Tag == b.Tag {
				for k, v := range a.Thresholds {
					if v == nil && b.Thresholds[k] != nil {
						return fmt.Errorf("cannot change originally null threshold %q: backend forbids clearing it during restore", k)
					}
				}
			}
		}
	}
	return nil
}
func (e engine) write(ctx context.Context, id string, body object) error {
	if len(body) == 0 {
		return nil
	}
	if e.def.name == "builtin_sensor_source" {
		// Enabling a replacement must not silently disable another, unowned feed.
		if string(body["enabled"]) == "true" {
			current, err := e.read(ctx, id)
			if err != nil {
				return err
			}
			if string(current["obsolete"]) == "true" {
				return fmt.Errorf("cannot enable an obsolete Sensor source")
			}
			var replaces []string
			_ = json.Unmarshal(current["replaces"], &replaces)
			if len(replaces) > 0 {
				rows, err := e.api.ConfigurationList(ctx, e.def.path, nil)
				if err != nil {
					return err
				}
				for _, row := range rows {
					obj, err := decode(row)
					if err != nil {
						return err
					}
					for _, name := range replaces {
						if textValue(obj["name"]) == name && string(obj["enabled"]) == "true" {
							return fmt.Errorf("disable replaced Sensor source %q explicitly before enabling this source", name)
						}
					}
				}
			}
		}
		if params, ok := body["parameters"]; ok {
			if _, err := e.api.ConfigurationRequest(ctx, "PATCH", e.itemPath(id)+"/params", object{"parameters": params}); err != nil {
				return err
			}
		}
		if enabled, ok := body["enabled"]; ok {
			verb := "disable"
			if string(enabled) == "true" {
				verb = "enable"
			}
			_, err := e.api.ConfigurationRequest(ctx, "POST", e.itemPath(id)+"/"+verb, nil)
			return err
		}
		return nil
	}
	method := "PATCH"
	if e.def.customizations || e.def.name == "local_auth_policy" {
		method = "PUT"
	}
	if e.def.customizations {
		parent, err := e.api.ConfigurationRequest(ctx, "GET", e.def.path+"/"+url.PathEscape(id), nil)
		if err != nil {
			return err
		}
		obj, err := decode(parent)
		if err != nil {
			return err
		}
		if string(obj["is_enabled"]) != "true" {
			return fmt.Errorf("enable the parent before changing or restoring customizations")
		}
	}
	_, err := e.api.ConfigurationRequest(ctx, method, e.itemPath(id), body)
	return err
}

// Scheduler enablement and cadence changes also move next_run. Retain that
// side effect separately so ordinary updates do not keep rewinding the clock.
func restoreBody(original object) object {
	body := object{}
	for k, v := range original {
		if k != "_scheduled_next_run" {
			body[k] = v
		}
	}
	if baseline, ok := original["_scheduled_next_run"]; ok {
		if _, explicit := body["next_run"]; !explicit {
			body["next_run"] = baseline
		}
	}
	return body
}

// Removing an existing metric row changes the backend's threshold baseline
// from that saved row to the parent's effective options. Refuse a removal if
// the original null threshold would then be rejected when restoring the row.
func (e engine) validateRestoration(ctx context.Context, id string, desired, original object) error {
	if e.def.name != "metric_customizations" {
		return nil
	}
	var old, next []struct {
		Tag        int64          `json:"tag_id"`
		Thresholds map[string]any `json:"thresholds"`
	}
	if err := json.Unmarshal(original["items"], &old); err != nil {
		return fmt.Errorf("invalid original metric customizations")
	}
	if err := json.Unmarshal(desired["items"], &next); err != nil {
		return fmt.Errorf("invalid desired metric customizations")
	}
	retained := map[int64]bool{}
	for _, row := range next {
		retained[row.Tag] = true
	}
	nullableRemoved := map[string]bool{}
	for _, row := range old {
		if !retained[row.Tag] {
			for field, value := range row.Thresholds {
				if value == nil {
					nullableRemoved[field] = true
				}
			}
		}
	}
	if len(nullableRemoved) == 0 {
		return nil
	}
	raw, err := e.api.ConfigurationRequest(ctx, "GET", e.def.path+"/"+url.PathEscape(id), nil)
	if err != nil {
		return err
	}
	parent, err := decode(raw)
	if err != nil {
		return err
	}
	defaults, err := decode(parent["default_options"])
	if err != nil {
		return fmt.Errorf("metric response omitted effective threshold defaults")
	}
	options, err := decode(parent["options"])
	if err != nil {
		return fmt.Errorf("metric response omitted effective threshold options")
	}
	for field, value := range options {
		defaults[field] = value
	}
	for field := range nullableRemoved {
		if value, exists := defaults[field]; exists && string(value) != "null" {
			return fmt.Errorf("cannot remove a customization with original null %s: the current parent threshold would prevent restoring it", field)
		}
	}
	return nil
}
