default: build

build:
	go build -o terraform-provider-mistershell

install: build
	mkdir -p ~/.terraform.d/plugins/registry.terraform.io/mistershell/mistershell/0.1.0/linux_amd64
	cp terraform-provider-mistershell ~/.terraform.d/plugins/registry.terraform.io/mistershell/mistershell/0.1.0/linux_amd64/

# Run the full acceptance test suite against a live MisterShell instance.
# Sources MISTERSHELL_URL and MISTERSHELL_API_KEY (and optional MISTERSHELL_INSECURE)
# from .env (gitignored), so no manual export is needed.
test: build
	set -a; . ./.env; set +a; TF_ACC=1 go test ./internal/provider/ -v -timeout 30m

# Run only the comprehensive end-to-end test (every resource + credential type).
test-e2e: build
	set -a; . ./.env; set +a; TF_ACC=1 go test ./internal/provider/ -v -timeout 30m -run TestAccEndToEnd

# Delete orphaned tfacc- test objects left behind by a crashed run.
sweep:
	set -a; . ./.env; set +a; go test ./internal/provider/ -v -timeout 10m -sweep=mistershell

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

.PHONY: default build install test test-e2e sweep fmt generate clean
