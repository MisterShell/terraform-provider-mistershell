package main

import (
	"bytes"
	"testing"
)

func TestRenderedTypesExcludeRuntimeRDKCatalogs(t *testing.T) {
	out, err := render(templateData{})
	if err != nil {
		t.Fatalf("render returned an error: %v", err)
	}
	for _, obsolete := range [][]byte{[]byte("SupportedResourceTypes"), []byte("SupportedCredentialTypes")} {
		if bytes.Contains(out, obsolete) {
			t.Fatalf("generated output contains runtime catalog %q", obsolete)
		}
	}
}
