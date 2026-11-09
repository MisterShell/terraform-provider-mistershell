package contentconfig

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"terraform-provider-mistershell/internal/client"
)

// catalogPage deliberately reads the nested nodes envelope rather than using the
// standard flat-list paginator. A short page ends traversal; a full page always
// requests another page, so child-location navigation nodes count correctly.
func catalogEntries(ctx context.Context, c *client.Client, location int64, parent string) ([]json.RawMessage, error) {
	var out []json.RawMessage
	for page := 1; page <= 10000; page++ {
		q := url.Values{"location_id": {strconv.FormatInt(location, 10)}, "path": {parent}, "page": {strconv.Itoa(page)}, "size": {"100"}}
		b, e := c.ConfigurationRequest(ctx, "GET", "/api/v1/files/catalog?"+q.Encode(), nil)
		if e != nil {
			return nil, e
		}
		var v struct {
			Nodes []json.RawMessage `json:"nodes"`
		}
		if e = json.Unmarshal(b, &v); e != nil {
			return nil, e
		}
		out = append(out, v.Nodes...)
		if len(v.Nodes) < 100 {
			return out, nil
		}
	}
	return nil, fmt.Errorf("catalog pagination exceeded 10000 pages")
}
func catalogEntry(ctx context.Context, c *client.Client, location int64, id string) (json.RawMessage, error) {
	if e := numericID(id); e != nil {
		return nil, e
	}
	queue := []string{""}
	seen := map[string]bool{}
	for len(queue) > 0 {
		parent := queue[0]
		queue = queue[1:]
		if seen[parent] {
			continue
		}
		seen[parent] = true
		if len(seen) > 10000 {
			return nil, fmt.Errorf("catalog traversal exceeded 10000 directories")
		}
		rows, e := catalogEntries(ctx, c, location, parent)
		if e != nil {
			return nil, e
		}
		for _, row := range rows {
			o, e := object(row)
			if e != nil {
				return nil, e
			}
			if jsonString(o["id"]) == id {
				return row, nil
			}
			if jsonString(o["kind"]) == "folder" {
				queue = append(queue, jsonString(o["path"]))
			}
		}
	}
	return nil, fmt.Errorf("catalog entry %s not visible in location %d; deletion and lost scope are indistinguishable, so state is preserved. Restore access or remove a confirmed-deleted entry from Terraform state", id, location)
}
func treeEntry(ctx context.Context, c *client.Client, id string) (json.RawMessage, error) {
	if e := numericID(id); e != nil {
		return nil, e
	}
	b, e := c.ConfigurationRequest(ctx, "GET", "/api/v1/browse/tree", nil)
	if e != nil {
		return nil, e
	}
	var nodes []json.RawMessage
	if e = json.Unmarshal(b, &nodes); e != nil {
		return nil, e
	}
	for len(nodes) > 0 {
		row := nodes[0]
		nodes = nodes[1:]
		o, e := object(row)
		if e != nil {
			return nil, e
		}
		if jsonString(o["kind"]) == "element" && jsonString(o["id"]) == id {
			return row, nil
		}
		var children []json.RawMessage
		if len(o["children"]) > 0 {
			if e = json.Unmarshal(o["children"], &children); e != nil {
				return nil, e
			}
			nodes = append(nodes, children...)
		}
	}
	return nil, fmt.Errorf("tree element %s is not visible; deletion and lost scope are indistinguishable, so state is preserved. Restore access or remove a confirmed-deleted element from Terraform state", id)
}
func licenseEntry(ctx context.Context, c *client.Client, id string) (json.RawMessage, error) {
	rows, e := c.ConfigurationList(ctx, "/api/v1/licenses", nil)
	if e != nil {
		return nil, e
	}
	for _, row := range rows {
		o, e := object(row)
		if e != nil {
			return nil, e
		}
		if jsonString(o["license_id"]) == "" {
			return nil, fmt.Errorf("license listing returned a row without an identity; state preserved")
		}
		if jsonString(o["license_id"]) == id {
			return row, nil
		}
	}
	return nil, nil
}
func notePath(scope string, id int64) (string, error) {
	if scope != "resource" && scope != "location" {
		return "", fmt.Errorf("scope_type must be resource or location")
	}
	if id < 1 {
		return "", fmt.Errorf("scope_id must be positive")
	}
	return fmt.Sprintf("/api/v1/notes/%s/%d", scope, id), nil
}
func latestNote(ctx context.Context, c *client.Client, p string) (json.RawMessage, int64, bool, error) {
	b, e := c.ConfigurationRequest(ctx, "GET", p, nil)
	if e != nil {
		return nil, 0, false, e
	}
	o, e := object(b)
	if e != nil {
		return nil, 0, false, e
	}
	locked := len(o["current_editor"]) > 0 && string(o["current_editor"]) != "null"
	v := o["latest_version"]
	if len(v) == 0 {
		return nil, 0, locked, fmt.Errorf("note response omitted latest_version; state preserved")
	}
	if string(v) == "null" {
		return nil, 0, locked, nil
	}
	n, e := object(v)
	if e != nil {
		return nil, 0, locked, e
	}
	if jsonInt(n["version"]) < 1 {
		return nil, 0, locked, fmt.Errorf("note response has no valid version; state preserved")
	}
	return v, jsonInt(n["version"]), locked, nil
}
func splitCatalogImport(id string) (int64, string, error) {
	parts := strings.Split(id, ":")
	if len(parts) != 2 {
		return 0, "", fmt.Errorf("import requires location_id:entry_id")
	}
	loc, e := strconv.ParseInt(parts[0], 10, 64)
	if e != nil || loc < 1 {
		return 0, "", fmt.Errorf("location ID must be positive")
	}
	if e = numericID(parts[1]); e != nil {
		return 0, "", e
	}
	return loc, parts[1], nil
}
