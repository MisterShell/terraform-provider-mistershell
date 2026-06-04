// Command types generates internal/client/types_gen.go from the MisterShell
// backend's checked-in OpenAPI spec, so the provider's supported resource and
// credential type lists never drift behind upstream.
//
// Spec resolution order:
//  1. -spec <path> flag or MISTERSHELL_OPENAPI env var: read that local JSON
//     file directly (offline / CI / test path).
//  2. Otherwise fetch from git: MISTERSHELL_REPO (default
//     git@github.com:MisterShell/mistershell.git) at MISTERSHELL_REF
//     (default main) via a minimal shallow sparse checkout of ui/openapi.json.
//
// The enum sets are read from these JSON pointers:
//   - components.schemas.NetworkResourceType.enum -> resource types
//   - components.schemas.CredentialType.enum       -> credential types
//   - components.schemas.LogDestinationCreate.properties.type.enum         -> log destination types
//   - components.schemas.LogDestinationCreate.properties.streams.items.enum -> log streams
//   - components.schemas.LogDestinationCreate.properties.min_severity.enum  -> log severities
//   - components.schemas.SyslogConfig.properties.protocol.enum  -> syslog protocols
//   - components.schemas.SyslogConfig.properties.format.enum    -> syslog formats
//   - components.schemas.SyslogConfig.properties.facility.enum  -> syslog facilities
//   - components.schemas.WebhookConfig.properties.method.enum       -> webhook methods
//   - components.schemas.WebhookConfig.properties.body_format.enum  -> webhook body formats
//   - components.schemas.WebhookConfig.properties.auth.discriminator.mapping (keys) -> webhook auth types
//
// If any enum is missing or empty the generator fails loudly (non-zero
// exit) to guard against the upstream schema being renamed.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"text/template"
)

const (
	specRelPath   = "ui/openapi.json"
	outputPath    = "internal/client/types_gen.go"
	defaultRepo   = "git@github.com:MisterShell/mistershell.git"
	defaultRef    = "main"
	resourcePtr   = "components.schemas.NetworkResourceType.enum"
	credentialPtr = "components.schemas.CredentialType.enum"

	logTypePtr           = "components.schemas.LogDestinationCreate.properties.type.enum"
	logStreamsPtr        = "components.schemas.LogDestinationCreate.properties.streams.items.enum"
	logSeverityPtr       = "components.schemas.LogDestinationCreate.properties.min_severity.enum"
	syslogProtocolPtr    = "components.schemas.SyslogConfig.properties.protocol.enum"
	syslogFormatPtr      = "components.schemas.SyslogConfig.properties.format.enum"
	syslogFacilityPtr    = "components.schemas.SyslogConfig.properties.facility.enum"
	webhookMethodPtr     = "components.schemas.WebhookConfig.properties.method.enum"
	webhookBodyFormatPtr = "components.schemas.WebhookConfig.properties.body_format.enum"
	webhookAuthTypePtr   = "components.schemas.WebhookConfig.properties.auth.discriminator.mapping (keys)"
)

// openAPISpec is a partial view of the OpenAPI document — only the two schemas
// we care about.
type enumProp struct {
	Enum []string `json:"enum"`
}

type openAPISpec struct {
	Components struct {
		Schemas struct {
			NetworkResourceType struct {
				Enum []string `json:"enum"`
			} `json:"NetworkResourceType"`
			CredentialType struct {
				Enum []string `json:"enum"`
			} `json:"CredentialType"`
			LogDestinationCreate struct {
				Properties struct {
					Type    enumProp `json:"type"`
					Streams struct {
						Items enumProp `json:"items"`
					} `json:"streams"`
					MinSeverity enumProp `json:"min_severity"`
				} `json:"properties"`
			} `json:"LogDestinationCreate"`
			SyslogConfig struct {
				Properties struct {
					Protocol enumProp `json:"protocol"`
					Format   enumProp `json:"format"`
					Facility enumProp `json:"facility"`
				} `json:"properties"`
			} `json:"SyslogConfig"`
			WebhookConfig struct {
				Properties struct {
					Method     enumProp `json:"method"`
					BodyFormat enumProp `json:"body_format"`
					Auth       struct {
						Discriminator struct {
							Mapping map[string]string `json:"mapping"`
						} `json:"discriminator"`
					} `json:"auth"`
				} `json:"properties"`
			} `json:"WebhookConfig"`
		} `json:"schemas"`
	} `json:"components"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "gen/types: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	specFlag := flag.String("spec", "", "path to a local OpenAPI JSON file (overrides git fetch)")
	flag.Parse()

	raw, source, err := loadSpec(*specFlag)
	if err != nil {
		return err
	}

	var spec openAPISpec
	if err := json.Unmarshal(raw, &spec); err != nil {
		return fmt.Errorf("parsing OpenAPI spec from %s: %w", source, err)
	}

	s := &spec.Components.Schemas
	td := templateData{
		ResourceTypes:       s.NetworkResourceType.Enum,
		CredentialTypes:     s.CredentialType.Enum,
		LogDestinationTypes: s.LogDestinationCreate.Properties.Type.Enum,
		LogStreams:          s.LogDestinationCreate.Properties.Streams.Items.Enum,
		LogSeverities:       s.LogDestinationCreate.Properties.MinSeverity.Enum,
		SyslogProtocols:     s.SyslogConfig.Properties.Protocol.Enum,
		SyslogFormats:       s.SyslogConfig.Properties.Format.Enum,
		SyslogFacilities:    s.SyslogConfig.Properties.Facility.Enum,
		WebhookMethods:      s.WebhookConfig.Properties.Method.Enum,
		WebhookBodyFormats:  s.WebhookConfig.Properties.BodyFormat.Enum,
		WebhookAuthTypes:    mapKeys(s.WebhookConfig.Properties.Auth.Discriminator.Mapping),
	}

	// Fail loudly if any pointer is missing/empty (anti-drift guard).
	for _, check := range []struct {
		name string
		ptr  string
		vals []string
	}{
		{"SupportedResourceTypes", resourcePtr, td.ResourceTypes},
		{"SupportedCredentialTypes", credentialPtr, td.CredentialTypes},
		{"SupportedLogDestinationTypes", logTypePtr, td.LogDestinationTypes},
		{"SupportedLogStreams", logStreamsPtr, td.LogStreams},
		{"SupportedLogSeverities", logSeverityPtr, td.LogSeverities},
		{"SupportedSyslogProtocols", syslogProtocolPtr, td.SyslogProtocols},
		{"SupportedSyslogFormats", syslogFormatPtr, td.SyslogFormats},
		{"SupportedSyslogFacilities", syslogFacilityPtr, td.SyslogFacilities},
		{"SupportedWebhookMethods", webhookMethodPtr, td.WebhookMethods},
		{"SupportedWebhookBodyFormats", webhookBodyFormatPtr, td.WebhookBodyFormats},
		{"SupportedWebhookAuthTypes", webhookAuthTypePtr, td.WebhookAuthTypes},
	} {
		if len(check.vals) == 0 {
			return fmt.Errorf("no values found at %s in %s (for %s) — has the upstream schema been renamed?", check.ptr, source, check.name)
		}
	}

	sort.Strings(td.ResourceTypes)
	sort.Strings(td.CredentialTypes)
	sort.Strings(td.LogDestinationTypes)
	sort.Strings(td.LogStreams)
	sort.Strings(td.LogSeverities)
	sort.Strings(td.SyslogProtocols)
	sort.Strings(td.SyslogFormats)
	sort.Strings(td.SyslogFacilities)
	sort.Strings(td.WebhookMethods)
	sort.Strings(td.WebhookBodyFormats)
	sort.Strings(td.WebhookAuthTypes)

	out, err := render(td)
	if err != nil {
		return err
	}

	if err := os.WriteFile(outputPath, out, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", outputPath, err)
	}

	fmt.Printf("gen/types: wrote %s from %s (%d resource types, %d credential types, %d log destination types, %d log streams, %d log severities)\n",
		outputPath, source, len(td.ResourceTypes), len(td.CredentialTypes), len(td.LogDestinationTypes), len(td.LogStreams), len(td.LogSeverities))
	return nil
}

// mapKeys returns the keys of a string-keyed map.
func mapKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// loadSpec resolves the spec bytes per the documented resolution order and
// returns the bytes plus a human-readable description of the source.
func loadSpec(specFlag string) ([]byte, string, error) {
	path := specFlag
	if path == "" {
		path = os.Getenv("MISTERSHELL_OPENAPI")
	}
	if path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, "", fmt.Errorf("reading local spec %q: %w", path, err)
		}
		return raw, fmt.Sprintf("local file %s", path), nil
	}
	return loadSpecFromGit()
}

// loadSpecFromGit performs a minimal shallow sparse checkout of ui/openapi.json
// and returns its contents.
func loadSpecFromGit() ([]byte, string, error) {
	repo := os.Getenv("MISTERSHELL_REPO")
	if repo == "" {
		repo = defaultRepo
	}
	ref := os.Getenv("MISTERSHELL_REF")
	if ref == "" {
		ref = defaultRef
	}

	tmp, err := os.MkdirTemp("", "mistershell-openapi-")
	if err != nil {
		return nil, "", fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmp)

	clone := exec.Command("git", "clone",
		"--depth", "1",
		"--filter=blob:none",
		"--sparse",
		"--branch", ref,
		repo, tmp,
	)
	if out, err := clone.CombinedOutput(); err != nil {
		return nil, "", fmt.Errorf("git clone of %s (ref %s) failed: %w\n%s\n"+
			"hint: ensure git/SSH access to the repo, or set MISTERSHELL_OPENAPI to a local spec path",
			repo, ref, err, bytes.TrimSpace(out))
	}

	sparse := exec.Command("git", "-C", tmp, "sparse-checkout", "set", specRelPath)
	if out, err := sparse.CombinedOutput(); err != nil {
		return nil, "", fmt.Errorf("git sparse-checkout of %s failed: %w\n%s", specRelPath, err, bytes.TrimSpace(out))
	}

	specPath := filepath.Join(tmp, filepath.FromSlash(specRelPath))
	raw, err := os.ReadFile(specPath)
	if err != nil {
		return nil, "", fmt.Errorf("reading %s from checkout of %s (ref %s): %w", specRelPath, repo, ref, err)
	}
	return raw, fmt.Sprintf("%s at %s:%s", specRelPath, repo, ref), nil
}

// templateData holds every generated enum slice rendered into types_gen.go.
type templateData struct {
	ResourceTypes       []string
	CredentialTypes     []string
	LogDestinationTypes []string
	LogStreams          []string
	LogSeverities       []string
	SyslogProtocols     []string
	SyslogFormats       []string
	SyslogFacilities    []string
	WebhookMethods      []string
	WebhookBodyFormats  []string
	WebhookAuthTypes    []string
}

var fileTemplate = template.Must(template.New("types_gen").Parse(`// Code generated by internal/gen/types; DO NOT EDIT.

package client

// Supported type lists generated from the MisterShell OpenAPI spec
// (ui/openapi.json):
//   - SupportedResourceTypes          <- components.schemas.NetworkResourceType
//   - SupportedCredentialTypes        <- components.schemas.CredentialType
//   - SupportedLogDestinationTypes    <- LogDestinationCreate.properties.type
//   - SupportedLogStreams             <- LogDestinationCreate.properties.streams.items
//   - SupportedLogSeverities          <- LogDestinationCreate.properties.min_severity
//   - SupportedSyslogProtocols        <- SyslogConfig.properties.protocol
//   - SupportedSyslogFormats          <- SyslogConfig.properties.format
//   - SupportedSyslogFacilities       <- SyslogConfig.properties.facility
//   - SupportedWebhookMethods         <- WebhookConfig.properties.method
//   - SupportedWebhookBodyFormats     <- WebhookConfig.properties.body_format
//   - SupportedWebhookAuthTypes       <- WebhookConfig.properties.auth.discriminator.mapping
//
// Regenerate with: make generate

// SupportedResourceTypes are the resource_type values accepted by the
// MisterShell backend.
var SupportedResourceTypes = []string{
{{- range .ResourceTypes}}
	{{printf "%q" .}},
{{- end}}
}

// SupportedCredentialTypes are the credential_type values accepted by the
// MisterShell backend.
var SupportedCredentialTypes = []string{
{{- range .CredentialTypes}}
	{{printf "%q" .}},
{{- end}}
}

// SupportedLogDestinationTypes are the log-destination type values.
var SupportedLogDestinationTypes = []string{
{{- range .LogDestinationTypes}}
	{{printf "%q" .}},
{{- end}}
}

// SupportedLogStreams are the log-destination stream values.
var SupportedLogStreams = []string{
{{- range .LogStreams}}
	{{printf "%q" .}},
{{- end}}
}

// SupportedLogSeverities are the log-destination min_severity values.
var SupportedLogSeverities = []string{
{{- range .LogSeverities}}
	{{printf "%q" .}},
{{- end}}
}

// SupportedSyslogProtocols are the syslog config protocol values (docs/anti-drift).
var SupportedSyslogProtocols = []string{
{{- range .SyslogProtocols}}
	{{printf "%q" .}},
{{- end}}
}

// SupportedSyslogFormats are the syslog config format values (docs/anti-drift).
var SupportedSyslogFormats = []string{
{{- range .SyslogFormats}}
	{{printf "%q" .}},
{{- end}}
}

// SupportedSyslogFacilities are the syslog config facility values (docs/anti-drift).
var SupportedSyslogFacilities = []string{
{{- range .SyslogFacilities}}
	{{printf "%q" .}},
{{- end}}
}

// SupportedWebhookMethods are the webhook config method values (docs/anti-drift).
var SupportedWebhookMethods = []string{
{{- range .WebhookMethods}}
	{{printf "%q" .}},
{{- end}}
}

// SupportedWebhookBodyFormats are the webhook config body_format values (docs/anti-drift).
var SupportedWebhookBodyFormats = []string{
{{- range .WebhookBodyFormats}}
	{{printf "%q" .}},
{{- end}}
}

// SupportedWebhookAuthTypes are the webhook config auth.type values (docs/anti-drift).
var SupportedWebhookAuthTypes = []string{
{{- range .WebhookAuthTypes}}
	{{printf "%q" .}},
{{- end}}
}
`))

func render(data templateData) ([]byte, error) {
	var buf bytes.Buffer
	if err := fileTemplate.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("rendering template: %w", err)
	}
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("gofmt of generated output: %w", err)
	}
	return formatted, nil
}
