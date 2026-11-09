// Package configbindings manages reversible configuration of backend-owned objects.
package configbindings

import (
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

type definition struct {
	name, path, lookup       string
	fields                   map[string]string
	listOnly, customizations bool
}

func definitions() []definition {
	return []definition{
		{name: "scheduled_task_config", path: "/api/v1/tasks", lookup: "task_type", fields: map[string]string{"task_name": "string", "is_enabled": "bool", "interval_minutes": "interval", "timeout_seconds": "timeout", "config": "nullable_object", "next_run": "datetime"}},
		{name: "fact_collection_config", path: "/api/v1/facts/configs", listOnly: true, fields: map[string]string{"is_enabled": "bool", "display_order": "integer"}},
		{name: "metric_config", path: "/api/v1/metrics", fields: map[string]string{"is_enabled": "bool", "display_order": "integer", "options": "object"}},
		{name: "snap_config", path: "/api/v1/snaps", fields: map[string]string{"is_enabled": "bool", "display_order": "integer", "options": "object"}},
		{name: "metric_customizations", path: "/api/v1/metrics", customizations: true, fields: map[string]string{"items": "items"}},
		{name: "snap_customizations", path: "/api/v1/snaps", customizations: true, fields: map[string]string{"items": "items"}},
		{name: "local_auth_policy", path: "/api/v1/auth-providers/local", fields: map[string]string{"mfa_method": "mfa"}},
		{name: "builtin_sensor_source", path: "/api/v1/sensors/sources", lookup: "name", listOnly: true, fields: map[string]string{"enabled": "bool", "parameters": "parameters"}},
	}
}
func definitionByName(name string) definition {
	for _, d := range definitions() {
		if d.name == name {
			return d
		}
	}
	panic("unknown configuration binding")
}

// Resources returns the fixed, reviewed configuration binding resource constructors.
func Resources() []func() resource.Resource {
	var result []func() resource.Resource
	for _, d := range definitions() {
		result = append(result, func() resource.Resource { return &bindingResource{def: d} })
	}
	return result
}

// DataSources returns matching read-only configuration data source constructors.
func DataSources() []func() datasource.DataSource {
	var result []func() datasource.DataSource
	for _, d := range definitions() {
		result = append(result, func() datasource.DataSource { return &bindingDataSource{def: d} })
	}
	return result
}
