package apimanaged

import (
	"encoding/json"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	ds "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	rs "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/dynamicdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/dynamicplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"terraform-provider-mistershell/internal/tfjson"
)

func resourceFields(d Definition) map[string]rs.Attribute {
	attrs := map[string]rs.Attribute{"id": rs.StringAttribute{Computed: true, Description: "Object identity. Import using this ID; resource variables use resource_id:stack_id.", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}}}
	for _, f := range d.Fields {
		required := f.Required
		optional := f.Writable() && !required
		computed := !f.Writable() || f.Computed || (len(f.Default) > 0 && string(f.Default) != "null")
		switch f.Kind {
		case "string":
			a := rs.StringAttribute{Description: f.Description, Required: required, Optional: optional, Computed: computed, Sensitive: f.Sensitive}
			if f.Immutable {
				a.PlanModifiers = []planmodifier.String{stringplanmodifier.RequiresReplace()}
			}
			if len(f.Default) > 0 && string(f.Default) != "null" {
				var v string
				_ = json.Unmarshal(f.Default, &v)
				a.Default = stringdefault.StaticString(v)
			}
			if len(f.Enum) > 0 {
				a.Validators = append(a.Validators, stringvalidator.OneOf(f.Enum...))
			}
			if f.MinLength > 0 {
				a.Validators = append(a.Validators, stringvalidator.LengthAtLeast(f.MinLength))
			}
			if f.MaxLength > 0 {
				a.Validators = append(a.Validators, stringvalidator.LengthAtMost(f.MaxLength))
			}
			attrs[f.Name] = a
		case "integer":
			a := rs.Int64Attribute{Description: f.Description, Required: required, Optional: optional, Computed: computed, Sensitive: f.Sensitive}
			if f.Immutable {
				a.PlanModifiers = []planmodifier.Int64{int64planmodifier.RequiresReplace()}
			}
			if len(f.Default) > 0 && string(f.Default) != "null" {
				var v int64
				_ = json.Unmarshal(f.Default, &v)
				a.Default = int64default.StaticInt64(v)
			}
			attrs[f.Name] = a
		case "boolean":
			a := rs.BoolAttribute{Description: f.Description, Required: required, Optional: optional, Computed: computed, Sensitive: f.Sensitive}
			if len(f.Default) > 0 && string(f.Default) != "null" {
				var v bool
				_ = json.Unmarshal(f.Default, &v)
				a.Default = booldefault.StaticBool(v)
			}
			attrs[f.Name] = a
		default:
			a := rs.DynamicAttribute{Description: f.Description + " Accepts a native Terraform value.", Required: required, Optional: optional, Computed: computed, Sensitive: f.Sensitive}
			if f.Immutable {
				a.PlanModifiers = []planmodifier.Dynamic{dynamicplanmodifier.RequiresReplace()}
			}
			if len(f.Default) > 0 && string(f.Default) != "null" {
				v, e := tfjson.FromRawJSON(f.Default)
				if e != nil {
					panic(e)
				}
				a.Default = dynamicdefault.StaticValue(v)
			}
			attrs[f.Name] = a
		}
	}
	return attrs
}
func dataFields(d Definition) map[string]ds.Attribute {
	attrs := map[string]ds.Attribute{"id": ds.StringAttribute{Optional: true, Computed: true, Description: "Object ID. Specify either ID or an exact name; resource variables use resource_id:stack_id."}}
	for _, f := range d.Fields {
		optional := f.Name == "name"
		switch f.Kind {
		case "string":
			attrs[f.Name] = ds.StringAttribute{Description: f.Description, Optional: optional, Computed: true, Sensitive: f.Sensitive, Validators: []validator.String{}}
		case "integer":
			attrs[f.Name] = ds.Int64Attribute{Description: f.Description, Computed: true, Sensitive: f.Sensitive}
		case "boolean":
			attrs[f.Name] = ds.BoolAttribute{Description: f.Description, Computed: true, Sensitive: f.Sensitive}
		default:
			attrs[f.Name] = ds.DynamicAttribute{Description: f.Description, Computed: true, Sensitive: f.Sensitive}
		}
	}
	return attrs
}
