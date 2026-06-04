package main

// Code generation directives for the provider.
//
// The supported resource_type and credential_type lists in
// internal/client/types_gen.go are generated from the MisterShell backend's
// checked-in OpenAPI spec so they never drift behind upstream. By default the
// generator fetches the spec from git (see internal/gen/types); set
// MISTERSHELL_OPENAPI to a local ui/openapi.json path to run offline.

//go:generate go run ./internal/gen/types
