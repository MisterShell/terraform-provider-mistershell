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
// The two enum sets are read from these JSON pointers:
//   - components.schemas.NetworkResourceType.enum -> resource types
//   - components.schemas.CredentialType.enum       -> credential types
//
// If either enum is missing or empty the generator fails loudly (non-zero
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
)

// openAPISpec is a partial view of the OpenAPI document — only the two schemas
// we care about.
type openAPISpec struct {
	Components struct {
		Schemas struct {
			NetworkResourceType struct {
				Enum []string `json:"enum"`
			} `json:"NetworkResourceType"`
			CredentialType struct {
				Enum []string `json:"enum"`
			} `json:"CredentialType"`
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

	resourceTypes := spec.Components.Schemas.NetworkResourceType.Enum
	credentialTypes := spec.Components.Schemas.CredentialType.Enum

	if len(resourceTypes) == 0 {
		return fmt.Errorf("no values found at %s in %s — has the upstream schema been renamed?", resourcePtr, source)
	}
	if len(credentialTypes) == 0 {
		return fmt.Errorf("no values found at %s in %s — has the upstream schema been renamed?", credentialPtr, source)
	}

	sort.Strings(resourceTypes)
	sort.Strings(credentialTypes)

	out, err := render(resourceTypes, credentialTypes)
	if err != nil {
		return err
	}

	if err := os.WriteFile(outputPath, out, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", outputPath, err)
	}

	fmt.Printf("gen/types: wrote %s from %s (%d resource types, %d credential types)\n",
		outputPath, source, len(resourceTypes), len(credentialTypes))
	return nil
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

var fileTemplate = template.Must(template.New("types_gen").Parse(`// Code generated by internal/gen/types; DO NOT EDIT.

package client

// Supported type lists generated from the MisterShell OpenAPI spec
// (ui/openapi.json):
//   - SupportedResourceTypes   <- components.schemas.NetworkResourceType
//   - SupportedCredentialTypes <- components.schemas.CredentialType
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
`))

func render(resourceTypes, credentialTypes []string) ([]byte, error) {
	var buf bytes.Buffer
	data := struct {
		ResourceTypes   []string
		CredentialTypes []string
	}{
		ResourceTypes:   resourceTypes,
		CredentialTypes: credentialTypes,
	}
	if err := fileTemplate.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("rendering template: %w", err)
	}
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("gofmt of generated output: %w", err)
	}
	return formatted, nil
}
