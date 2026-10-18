package resources

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"terraform-provider-mistershell/internal/client"
	"terraform-provider-mistershell/internal/credentialrefs"
)

var _ resource.ResourceWithValidateConfig = &CredentialResource{}

func (r *CredentialResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config CredentialResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(validateCredentialSources(config)...)
	}
}

func credentialSource(v types.String) string {
	if v.IsNull() || v.IsUnknown() {
		return "local"
	}
	return v.ValueString()
}

func validateCredentialSources(m CredentialResourceModel) diag.Diagnostics {
	var diagnostics diag.Diagnostics
	for _, a := range []struct {
		prefix string
		source types.String
		id     types.Int64
		ref    types.Object
		data   types.Dynamic
	}{
		{"", m.Source, m.ExternalVaultID, m.ExternalVaultRef, m.CredentialData},
		{"user_", m.UserSource, m.UserExternalVaultID, m.UserExternalVaultRef, m.UserCredentialData},
	} {
		problem := func(message string) { diagnostics.AddError("Invalid credential source", a.prefix+message) }
		if !a.id.IsNull() && !a.id.IsUnknown() && a.id.ValueInt64() <= 0 {
			problem("external_vault_id must be positive.")
		}
		if !a.source.IsNull() && !a.source.IsUnknown() {
			switch a.source.ValueString() {
			case "local":
				if !a.id.IsNull() && !a.id.IsUnknown() || !a.ref.IsNull() && !a.ref.IsUnknown() {
					problem("local credentials cannot carry vault references; remove external_vault_id and external_vault_ref.")
				}
				if a.prefix == "user_" && !a.data.IsNull() && !a.data.IsUnknown() {
					problem("credential_data must be omitted for a local user source.")
				}
				if a.prefix == "" && a.data.IsNull() {
					problem("credential_data is required for a local source.")
				}
			case "external":
				if a.id.IsNull() {
					problem("external_vault_id is required for an external source.")
				}
				if a.ref.IsNull() {
					problem("external_vault_ref is required for an external source.")
				}
			default:
				problem("source must be local or external.")
			}
		}
		var literals map[string]json.RawMessage
		if !a.data.IsNull() && credentialrefs.Known(a.data) {
			raw, err := dynamicToRawJSON(a.data)
			if err != nil || json.Unmarshal(raw, &literals) != nil || literals == nil {
				problem("credential_data must be an object.")
			}
		}
		if a.ref.IsNull() || !credentialrefs.Known(a.ref) {
			continue
		}
		raw, err := credentialrefs.ToJSON(a.ref)
		if err != nil {
			problem("external_vault_ref must be known before apply.")
			continue
		}
		var ref struct {
			Base   map[string]json.RawMessage `json:"base"`
			Fields map[string]*string         `json:"fields"`
		}
		if err := json.Unmarshal(raw, &ref); err != nil || ref.Base == nil || len(ref.Fields) == 0 {
			problem("external_vault_ref requires an object base and a nonempty map of string fields.")
			continue
		}
		for _, v := range ref.Base {
			var text string
			if len(v) > 0 && v[0] == '"' && json.Unmarshal(v, &text) == nil {
				continue
			}
			n, ok := new(big.Rat).SetString(string(v))
			if !ok || !n.IsInt() {
				problem("external_vault_ref.base values must be strings or integers.")
			}
		}
		for k, v := range ref.Fields {
			if v == nil {
				problem("external_vault_ref.fields values cannot be null.")
			}
			if _, ok := literals[k]; ok {
				problem("credential_data and external_vault_ref.fields must have disjoint field names.")
			}
		}
	}
	return diagnostics
}

func credentialLiterals(v types.Dynamic, changing bool) (json.RawMessage, error) {
	raw, err := dynamicToRawJSON(v)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		raw = json.RawMessage(`{}`)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return nil, fmt.Errorf("credential literals must be an object")
	}
	if changing {
		for _, v := range fields {
			var s string
			if json.Unmarshal(v, &s) == nil && s == "****" {
				return nil, fmt.Errorf("stored secret masks cannot be reused across credential source or type changes; configure the secret or map it to a vault")
			}
		}
	}
	return raw, nil
}

func credentialCreateInput(plan *CredentialResourceModel) (client.CredentialCreateInput, error) {
	plan.Source = types.StringValue(credentialSource(plan.Source))
	plan.UserSource = types.StringValue(credentialSource(plan.UserSource))
	if d := validateCredentialSources(*plan); d.HasError() {
		return client.CredentialCreateInput{}, fmt.Errorf("%s", d.Errors()[0].Detail())
	}
	literals, err := credentialLiterals(plan.CredentialData, true)
	if err != nil {
		return client.CredentialCreateInput{}, err
	}
	extra, err := dynamicToRawJSON(plan.ExtraData)
	if err != nil {
		return client.CredentialCreateInput{}, err
	}
	input := client.CredentialCreateInput{Name: plan.Name.ValueString(), CredentialType: plan.CredentialType.ValueString(), CredentialData: literals, Description: stringValueToPtr(plan.Description), RequiresUserMapping: boolValueToPtr(plan.RequiresUserMapping), ExtraData: extra}
	if plan.Source.ValueString() == "external" {
		input.Source = "external"
		input.ExternalVaultID = int64ValueToPtr(plan.ExternalVaultID)
		input.ExternalVaultRef, err = credentialrefs.ToJSON(plan.ExternalVaultRef)
		if err != nil {
			return input, err
		}
	}
	if plan.UserSource.ValueString() == "external" {
		input.UserSource = "external"
		input.UserExternalVaultID = int64ValueToPtr(plan.UserExternalVaultID)
		input.UserExternalVaultRef, err = credentialrefs.ToJSON(plan.UserExternalVaultRef)
		if err != nil {
			return input, err
		}
		input.UserCredentialData, err = credentialLiterals(plan.UserCredentialData, true)
		if err != nil {
			return input, err
		}
	}
	return input, nil
}

func credentialUpdateInput(plan *CredentialResourceModel, state CredentialResourceModel) (client.CredentialUpdateInput, error) {
	if plan.Source.IsNull() || plan.Source.IsUnknown() {
		plan.Source = types.StringValue(credentialSource(state.Source))
	}
	if plan.UserSource.IsNull() || plan.UserSource.IsUnknown() {
		plan.UserSource = types.StringValue(credentialSource(state.UserSource))
	}
	input := client.CredentialUpdateInput{}
	if d := validateCredentialSources(*plan); d.HasError() {
		return input, fmt.Errorf("%s", d.Errors()[0].Detail())
	}
	if !plan.Name.Equal(state.Name) {
		input.Name = stringValueToPtr(plan.Name)
	}
	if !plan.Description.Equal(state.Description) {
		input.Description, _ = json.Marshal(stringValueToPtr(plan.Description))
	}
	if !plan.RequiresUserMapping.Equal(state.RequiresUserMapping) {
		input.RequiresUserMapping = boolValueToPtr(plan.RequiresUserMapping)
	}
	if !plan.ExtraData.Equal(state.ExtraData) {
		var err error
		input.ExtraData, err = dynamicToRawJSON(plan.ExtraData)
		if err != nil {
			return input, err
		}
		if len(input.ExtraData) == 0 {
			input.ExtraData = json.RawMessage(`null`)
		}
	}
	typeChanged := !plan.CredentialType.Equal(state.CredentialType)
	if typeChanged {
		input.CredentialType = stringValueToPtr(plan.CredentialType)
	}
	for _, a := range []struct {
		user              bool
		source, oldSource types.String
		id, oldID         types.Int64
		ref, oldRef       types.Object
		data, oldData     types.Dynamic
	}{
		{false, plan.Source, state.Source, plan.ExternalVaultID, state.ExternalVaultID, plan.ExternalVaultRef, state.ExternalVaultRef, plan.CredentialData, state.CredentialData},
		{true, plan.UserSource, state.UserSource, plan.UserExternalVaultID, state.UserExternalVaultID, plan.UserExternalVaultRef, state.UserExternalVaultRef, plan.UserCredentialData, state.UserCredentialData},
	} {
		source := credentialSource(a.source)
		transition := source != credentialSource(a.oldSource)
		changing := transition || typeChanged
		var data, id, ref json.RawMessage
		var err error
		if transition {
			if a.user {
				input.UserSource = source
			} else {
				input.Source = source
			}
		}
		if changing || !a.data.Equal(a.oldData) {
			if a.user && source == "local" {
				data = json.RawMessage(`null`)
			} else {
				data, err = credentialLiterals(a.data, changing)
				if err != nil {
					return input, err
				}
			}
		}
		if changing || !a.id.Equal(a.oldID) {
			id, _ = json.Marshal(int64ValueToPtr(a.id))
		}
		if changing || !a.ref.Equal(a.oldRef) {
			ref, err = credentialrefs.ToJSON(a.ref)
			if err != nil {
				return input, err
			}
			if len(ref) == 0 {
				ref = json.RawMessage(`null`)
			}
		}
		if a.user {
			input.UserCredentialData = data
			input.UserExternalVaultID = id
			input.UserExternalVaultRef = ref
		} else {
			input.CredentialData = data
			input.ExternalVaultID = id
			input.ExternalVaultRef = ref
		}
	}
	return input, nil
}
