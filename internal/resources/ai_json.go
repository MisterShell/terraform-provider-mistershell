package resources

import (
	"encoding/json"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-mistershell/internal/tfjson"
)

// Optional absent settings remain null when the backend has no readable values.
// This also avoids treating preserved, unknown model secrets as new config after
// a user removes config. Imports still hydrate every readable setting.
func reconcileAIJSON(prior types.Dynamic, raw json.RawMessage, secretKeys []string) (types.Dynamic, error) {
	if prior.IsNull() {
		var fields map[string]any
		if json.Unmarshal(raw, &fields) == nil && fields != nil {
			readable := false
			for key, value := range fields {
				masked := false
				for _, secret := range secretKeys {
					if strings.EqualFold(key, secret) && (value == "***" || value == "****") {
						masked = true
						break
					}
				}
				if !masked {
					readable = true
					break
				}
			}
			if !readable {
				return types.DynamicNull(), nil
			}
		}
	}
	return tfjson.Reconcile(prior, raw, secretKeys)
}
