package apimanaged

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
)

// Canary persists these arrays with stable deduplication. Reject duplicates
// rather than allowing a successful write to return a different known value.
func validateCanarySelectors(ctx context.Context, d Definition, reader attributeReader, allowUnknown bool) diag.Diagnostics {
	var diags diag.Diagnostics
	fields := []string{}
	switch d.Name {
	case "canary_service":
		fields = []string{"location_ids"}
	case "canary_alert_rule":
		fields = []string{"service_types", "event_codes", "src_cidrs", "canary_ids", "location_ids"}
	default:
		return diags
	}
	for _, name := range fields {
		value, dd := get(ctx, reader, Field{Name: name, Kind: "dynamic"})
		diags.Append(dd...)
		if dd.HasError() || value.IsNull() {
			continue
		}
		tv, err := value.ToTerraformValue(ctx)
		if err != nil {
			diags.AddAttributeError(path.Root(name), "Invalid selector", err.Error())
			continue
		}
		if !tv.IsFullyKnown() {
			if !allowUnknown {
				diags.AddAttributeError(path.Root(name), "Unknown selector", name+" must be fully known before applying.")
			}
			continue
		}
		raw, err := rawValue(value)
		if err != nil {
			diags.AddAttributeError(path.Root(name), "Invalid selector", err.Error())
			continue
		}
		var entries []json.RawMessage
		if err = json.Unmarshal(raw, &entries); err != nil {
			diags.AddAttributeError(path.Root(name), "Invalid selector", name+" must be a list.")
			continue
		}
		seen := map[string]bool{}
		for _, entry := range entries {
			key := string(entry)
			if seen[key] {
				diags.AddAttributeError(path.Root(name), "Duplicate selector", name+" must not contain duplicate values: the API deduplicates selectors.")
				break
			}
			seen[key] = true
		}
	}
	return diags
}

// Full and recovery fleet projections share a required identity/status shape.
// A recovery projection is explicitly marked by its non-empty license_reason.
func validateCanaryProjection(obj map[string]json.RawMessage, expectedID string) (bool, error) {
	var common struct {
		ID            *int64  `json:"id"`
		Name          *string `json:"name"`
		LocationID    *int64  `json:"location_id"`
		Status        *string `json:"status"`
		Enabled       *bool   `json:"is_enabled"`
		LicenseReason *string `json:"license_reason"`
	}
	raw, err := json.Marshal(obj)
	if err == nil {
		err = json.Unmarshal(raw, &common)
	}
	if err != nil {
		return false, err
	}
	if common.ID == nil || *common.ID <= 0 || common.Name == nil || common.LocationID == nil || *common.LocationID <= 0 || common.Status == nil || common.Enabled == nil {
		return false, fmt.Errorf("Canary response must include a valid id, name, location_id, status and is_enabled")
	}
	if expectedID != "" && fmt.Sprint(*common.ID) != expectedID {
		return false, fmt.Errorf("Canary response identity does not match requested object %s", expectedID)
	}
	recovery := common.LicenseReason != nil && strings.TrimSpace(*common.LicenseReason) != ""
	if !recovery {
		for _, field := range []string{"ip_ignorelist", "rate_limit_per_min", "health", "counters"} {
			if valueIsEmpty(obj[field]) {
				return false, fmt.Errorf("licensed Canary response is missing required field %s", field)
			}
		}
	}
	return recovery, nil
}
