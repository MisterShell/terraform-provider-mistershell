package apimanaged

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Imported masks become null in Terraform state. A graph PATCH replaces the
// whole graph, so use the API preservation sentinel for unchanged unavailable
// secrets. Match nodes by identity rather than their canvas/list position.
func preserveImportedPlaybookSecrets(prior, planned json.RawMessage) (json.RawMessage, error) {
	var old, next map[string]any
	if err := json.Unmarshal(prior, &old); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(planned, &next); err != nil {
		return nil, err
	}
	oldByID := map[string]map[string]any{}
	oldNodes, _ := old["nodes"].([]any)
	for _, value := range oldNodes {
		node, ok := value.(map[string]any)
		if !ok {
			continue
		}
		id, _ := node["id"].(string)
		oldByID[id] = node
	}
	nodes, _ := next["nodes"].([]any)
	for _, value := range nodes {
		node, ok := value.(map[string]any)
		if !ok || node["action_id"] != "webhook" {
			continue
		}
		config, _ := node["config"].(map[string]any)
		auth, _ := config["auth"].(map[string]any)
		kind, _ := auth["type"].(string)
		key := map[string]string{"bearer": "token", "basic": "password", "header": "header_value"}[kind]
		if key == "" {
			continue
		}
		if secret, exists := auth[key]; exists && secret != nil {
			if text, ok := secret.(string); ok && (strings.TrimSpace(text) == "" || text == "****") {
				return nil, fmt.Errorf("webhook authentication requires a real secret; leave an imported unknown secret null or set auth.type to none to disable authentication")
			}
			continue
		}
		id, _ := node["id"].(string)
		previousNode := oldByID[id]
		previousConfig, _ := previousNode["config"].(map[string]any)
		previousAuth, _ := previousConfig["auth"].(map[string]any)
		if previousNode["action_id"] != "webhook" || previousAuth["type"] != kind {
			return nil, fmt.Errorf("new webhook nodes and authentication types require an explicit secret")
		}
		if previousAuth[key] != nil {
			return nil, fmt.Errorf("cannot remove a known webhook authentication secret; supply a replacement or set auth.type to none to disable authentication")
		}
		auth[key] = "****"
	}
	return json.Marshal(next)
}
