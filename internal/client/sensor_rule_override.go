package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// SensorCatalogRule includes staged override state. EffectiveEnabled reflects
// the staged selection; Pending distinguishes it from the active ruleset.
type SensorCatalogRule struct {
	GID              int64   `json:"gid"`
	SID              int64   `json:"sid"`
	Message          *string `json:"msg"`
	ClassType        *string `json:"classtype"`
	SourceName       *string `json:"source_name"`
	DefaultEnabled   bool    `json:"default_enabled"`
	Revision         *int64  `json:"rev"`
	OverrideAction   *string `json:"override_action"`
	EffectiveEnabled bool    `json:"effective_enabled"`
	Pending          bool    `json:"pending"`
	Raw              string  `json:"raw"`
}

func (c *Client) GetSensorCatalogRule(ctx context.Context, gid, sid int64) (*SensorCatalogRule, error) {
	data, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/api/v1/sensors/rule-catalog/%d/%d", gid, sid), nil)
	if err != nil {
		return nil, err
	}
	var row SensorCatalogRule
	if err = json.Unmarshal(data, &row); err != nil {
		return nil, fmt.Errorf("decoding sensor catalog rule: %w", err)
	}
	return &row, nil
}

// SetSensorRuleOverride stages exactly one override. It never invokes the
// global apply endpoint, which would also publish other operators' changes.
func (c *Client) SetSensorRuleOverride(ctx context.Context, gid, sid int64, action string) error {
	_, err := c.doRequest(ctx, http.MethodPost, "/api/v1/sensors/rule-overrides", map[string]any{"items": []map[string]any{{"gid": gid, "sid": sid, "action": action}}})
	return err
}
func (c *Client) ResetSensorRuleOverride(ctx context.Context, gid, sid int64) error {
	_, err := c.doRequest(ctx, http.MethodPost, "/api/v1/sensors/rule-overrides", map[string]any{"items": []map[string]any{{"gid": gid, "sid": sid, "reset": true}}})
	return err
}
