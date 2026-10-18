package apimanaged

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"terraform-provider-mistershell/internal/client"
)

// Validation also runs at apply: a certificate-producing dependency can leave
// PEM unknown during ValidateConfig and resolve to an unsafe bundle later.
func validateCACertificatePEM(ctx context.Context, reader attributeReader, allowUnknown bool) diag.Diagnostics {
	var diags diag.Diagnostics
	var value types.String
	diags.Append(reader.GetAttribute(ctx, path.Root("pem"), &value)...)
	if diags.HasError() {
		return diags
	}
	if allowUnknown && (value.IsUnknown() || value.IsNull()) {
		return diags
	}
	raw := bytes.TrimSpace([]byte(value.ValueString()))
	block, rest := pem.Decode(raw)
	if value.IsUnknown() || value.IsNull() || !bytes.HasPrefix(raw, []byte("-----BEGIN CERTIFICATE-----")) || block == nil || block.Type != "CERTIFICATE" || len(bytes.TrimSpace(rest)) != 0 {
		diags.AddAttributeError(path.Root("pem"), "Single certificate required", "Supply exactly one known PEM CERTIFICATE block, without a bundle or private-key material. Manage each trusted certificate separately.")
	}
	return diags
}

// A single-block request should create at most one certificate. If an API
// unexpectedly creates more, roll back every newly created ID before returning
// an error so no unmanaged certificates remain trusted.
func (r *ManagedResource) singleCreatedCertificate(ctx context.Context, obj map[string]json.RawMessage) (json.RawMessage, error) {
	var created []json.RawMessage
	if err := json.Unmarshal(obj["created"], &created); err != nil {
		return nil, fmt.Errorf("invalid certificate create response")
	}
	if len(created) == 0 {
		return nil, fmt.Errorf("the certificate already exists; import the existing certificate explicitly")
	}
	if len(created) == 1 {
		return created[0], nil
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	var failures []string
	for _, raw := range created {
		entry, err := object(raw)
		if err != nil {
			failures = append(failures, "an unidentifiable created certificate")
			continue
		}
		id := idString(entry["id"])
		number, err := strconv.ParseInt(id, 10, 64)
		if err != nil || number <= 0 {
			failures = append(failures, "an unidentifiable created certificate")
			continue
		}
		if _, err := r.client.ConfigurationRequest(cleanupCtx, "DELETE", itemPath(r.definition, id), nil); err != nil && !client.IsNotFound(err) {
			failures = append(failures, "certificate ID "+id)
		}
	}
	if len(failures) > 0 {
		return nil, fmt.Errorf("API unexpectedly created %d certificates; rollback could not remove %s; remove these unmanaged trust entries immediately", len(created), strings.Join(failures, ", "))
	}
	return nil, fmt.Errorf("API unexpectedly created %d certificates from one request; all newly created certificates were removed", len(created))
}
