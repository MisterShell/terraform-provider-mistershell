# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

A Terraform provider (built on the [terraform-plugin-framework](https://github.com/hashicorp/terraform-plugin-framework)) for managing [MisterShell](https://www.mistershell.com) inventory: locations, network resources, and credentials. This repo is the provider (client) only and talks to a running MisterShell instance over its REST API; the backend lives in a separate repository (`git@github.com:MisterShell/mistershell.git`).

## Commands

```bash
make build        # go build -o terraform-provider-mistershell
make install      # build + copy into ~/.terraform.d/plugins/.../0.1.0/linux_amd64 for local terraform use
make fmt          # go fmt ./...
make test         # TF_ACC=1 go test ./internal/provider/ -v -timeout 5m  (acceptance tests)
make clean
```

Acceptance tests are **real** — they require a live MisterShell instance and create/destroy actual objects. They are gated by `TF_ACC=1` and the `MISTERSHELL_URL` / `MISTERSHELL_API_KEY` env vars (enforced by `testAccPreCheck`). Without these set they fail fast.

```bash
# Run the full suite against a live instance
MISTERSHELL_URL=http://localhost:13000 MISTERSHELL_API_KEY=yami_xxx make test

# Run a single acceptance test (test funcs are named TestAcc*)
TF_ACC=1 MISTERSHELL_URL=... MISTERSHELL_API_KEY=... \
  go test ./internal/provider/ -v -run TestAccLocationDataSource_byID
```

Lint in CI is just `test -z "$(gofmt -l .)"` — keep everything gofmt-clean. There is no golangci-lint config; the `//nolint:gosec` comments target a linter that isn't wired into CI.

## Architecture

Three layers, each its own package under `internal/`:

- **`internal/client`** — a single `Client` struct wrapping `*http.Client`. All API calls go through `doRequest`, which adds `Authorization: Bearer <api_key>`, unwraps the standard MisterShell response envelope (`{success, message, data}`), and maps HTTP 404 → a typed `*NotFoundError` (check with `client.IsNotFound`). One file (`client.go`) holds the typed `*Input` / `*Response` / `*ListFilter` structs and CRUD+List methods for **all three** entities.
- **`internal/resources`** — managed resources (`mistershell_location`, `mistershell_resource`, `mistershell_credential`). Each implements full CRUD + `ImportState` (import by integer ID).
- **`internal/datasources`** — read-only data sources mirroring the three entities, supporting lookup by `id` (direct GET) or by search filters (list + exact-match client-side filtering).

`internal/provider/provider.go` is the wiring: resolves config (`url`, `api_key`, `insecure`) from attributes or `MISTERSHELL_*` env vars, builds the `Client`, and hands it to every resource/data source via `resp.ResourceData` / `resp.DataSourceData`. Each resource/data source picks it up in `Configure` by type-asserting `req.ProviderData.(*client.Client)`. `main.go` serves the provider under address `registry.terraform.io/mistershell/mistershell`.

### Key conventions when adding/editing entities

- **IDs are `int64`** end to end (API, state, import). Not strings/UUIDs.
- **State-to-API mapping goes through `helpers.go`** in each of `internal/resources` and `internal/datasources`. Use the `*PtrToValue` / `*ValueToPtr` converters rather than hand-rolling null checks — they centralize the `IsNull()/IsUnknown()` handling. Each entity also has a `map<Entity>ResponseToModel` function as the single place that copies an API response into the tfsdk model.
- **Optional vs. update semantics matter in the client structs.** `*CreateInput` fields use `omitempty`; `*UpdateInput` fields for nullable attributes deliberately **omit** `omitempty` (e.g. `Description`, `ParentID`, `ExtraData`) so that clearing a value sends explicit `null` to the PATCH endpoint. Preserve this distinction.
- **Free-form JSON fields** (`extra_data`, `connector_data`, `credential_data`) are modeled as `jsontypes.Normalized` in tfsdk and `json.RawMessage` over the wire; convert with `normalizedToRawJSON` / `rawJSONToNormalized`. In HCL these are set with `jsonencode(...)`.
- **`Read` removes resources on 404**: `if client.IsNotFound(err) { resp.State.RemoveResource(ctx); return }` — keep this so deleted-out-of-band objects re-create cleanly.
- **Data source search is fuzzy server-side**; data sources re-apply an exact `name ==` match client-side and error explicitly on zero or >1 matches. Filters must resolve to exactly one result.
- **Supported type lists are generated, not hardcoded.** `client.SupportedResourceTypes` and `client.SupportedCredentialTypes` live in `internal/client/types_gen.go` and feed the `resource_type` / `credential_type` `stringvalidator.OneOf(...)` validators. They are generated from the MisterShell OpenAPI spec (`ui/openapi.json`, `components.schemas.NetworkResourceType` / `CredentialType`) by `internal/gen/types` (`//go:generate` directive in `generate.go`). Do **not** re-hardcode these enums or hand-edit `types_gen.go`. Refresh with `make generate` (set `MISTERSHELL_OPENAPI=/path/to/ui/openapi.json` to run offline against a local backend clone; `MISTERSHELL_REPO` / `MISTERSHELL_REF` override the git source). The generator fails loudly if either enum is missing/empty, guarding against an upstream schema rename.

### Docs

`docs/` is generated by [tfplugindocs](https://github.com/hashicorp/terraform-plugin-docs) from schema `Description` fields and the `examples/` directory — do not hand-edit generated files. Update the schema descriptions and example `.tf` files, then regenerate. `.goreleaser.yml` drives the registry release (GPG-signed checksums) via the `release.yml` workflow.
