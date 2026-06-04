default: build

build:
	go build -o terraform-provider-mistershell

install: build
	mkdir -p ~/.terraform.d/plugins/registry.terraform.io/mistershell/mistershell/0.1.0/linux_amd64
	cp terraform-provider-mistershell ~/.terraform.d/plugins/registry.terraform.io/mistershell/mistershell/0.1.0/linux_amd64/

# Run the full acceptance test suite against a live MisterShell instance.
# Requires: MISTERSHELL_URL and MISTERSHELL_API_KEY environment variables.
# Example:
#   MISTERSHELL_URL=http://localhost:13000 MISTERSHELL_API_KEY=yami_xxx make test
test: build
	TF_ACC=1 go test ./internal/provider/ -v -timeout 5m

fmt:
	go fmt ./...

# Regenerate the supported resource/credential type lists from the MisterShell
# OpenAPI spec (internal/client/types_gen.go). By default the generator fetches
# the spec from git; set MISTERSHELL_OPENAPI to a local ui/openapi.json path to
# run offline. See internal/gen/types and README "Development".
generate:
	go generate ./...
	go fmt ./...

clean:
	rm -f terraform-provider-mistershell

.PHONY: default build install test fmt generate clean
