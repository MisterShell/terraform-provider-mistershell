default: build

MISTERSHELL_ENV_FILE ?= ./.env
ACC_TEST_FLAGS = -count=1 -parallel=1 -v -timeout=30m
ACC_SHELL_ENV = export TF_ACC=1; : "$${MISTERSHELL_ACC_SSH_HOST:=127.0.0.1}"; : "$${MISTERSHELL_ACC_SSH_PORT:=12223}"; : "$${MISTERSHELL_ACC_SSH_USERNAME:=tfacc}"; : "$${MISTERSHELL_ACC_SSH_PASSWORD:=tfacc-password}"; : "$${MISTERSHELL_ACC_SSH_UPDATE_HOST:=localhost}"; export MISTERSHELL_ACC_SSH_HOST MISTERSHELL_ACC_SSH_PORT MISTERSHELL_ACC_SSH_USERNAME MISTERSHELL_ACC_SSH_PASSWORD MISTERSHELL_ACC_SSH_UPDATE_HOST

build:
	go build -o terraform-provider-mistershell

install: build
	mkdir -p ~/.terraform.d/plugins/registry.terraform.io/mistershell/mistershell/2.0.15/linux_amd64
	cp terraform-provider-mistershell ~/.terraform.d/plugins/registry.terraform.io/mistershell/mistershell/2.0.15/linux_amd64/

# Run the full acceptance test suite against a live MisterShell instance.
# Sources MISTERSHELL_URL and MISTERSHELL_API_KEY (and optional MISTERSHELL_INSECURE)
# from .env (gitignored), so no manual export is needed.
test: test-live

test-live: build acceptance-preflight
	set -a; . "$(MISTERSHELL_ENV_FILE)"; set +a; \
	$(ACC_SHELL_ENV); \
	go test ./internal/provider/ $(ACC_TEST_FLAGS)

test-live-onboarding: acceptance-preflight
	set -a; . "$(MISTERSHELL_ENV_FILE)"; set +a; \
	$(ACC_SHELL_ENV); \
	go test ./internal/provider/ $(ACC_TEST_FLAGS) -run '^(TestAccNetworkResource|TestAccEndToEnd)'

test-live-core: acceptance-preflight
	set -a; . "$(MISTERSHELL_ENV_FILE)"; set +a; $(ACC_SHELL_ENV); \
	go test ./internal/provider/ $(ACC_TEST_FLAGS) -run '^TestAcc(Credential|Location|ResourceTags|Tags|ResourceDataSource_tags|Tag|Role|Permissions|LogDestination|Setting|User|Users|Worker)'

test-live-policy-auth: acceptance-preflight
	set -a; . "$(MISTERSHELL_ENV_FILE)"; set +a; $(ACC_SHELL_ENV); \
	go test ./internal/provider/ $(ACC_TEST_FLAGS) -run '^TestAcc(SessionPolicy|AuthProvider)'

test-live-vaults: acceptance-preflight
	set -a; . "$(MISTERSHELL_ENV_FILE)"; set +a; $(ACC_SHELL_ENV); \
	go test ./internal/provider/ $(ACC_TEST_FLAGS) -run '^TestAcc(ExternalVault|Credential_externalSources)'

test-live-ai: acceptance-preflight
	set -a; . "$(MISTERSHELL_ENV_FILE)"; set +a; $(ACC_SHELL_ENV); \
	go test ./internal/provider/ $(ACC_TEST_FLAGS) -run '^TestAcc(AI|EdgeAI_)'

test-live-edge: acceptance-preflight
	set -a; . "$(MISTERSHELL_ENV_FILE)"; set +a; $(ACC_SHELL_ENV); \
	go test ./internal/provider/ $(ACC_TEST_FLAGS) -run '^TestAccEdge_'

test-live-staged: test-live-onboarding test-live-core test-live-policy-auth test-live-vaults test-live-ai test-live-edge

# Run the live end-to-end onboarding graph.
test-e2e: build
	set -a; . "$(MISTERSHELL_ENV_FILE)"; set +a; \
	$(ACC_SHELL_ENV); \
	go test ./internal/provider/ $(ACC_TEST_FLAGS) -run '^TestAccEndToEnd'

acceptance-fixture-up:
	@if test -r "$(MISTERSHELL_ENV_FILE)"; then set -a; . "$(MISTERSHELL_ENV_FILE)"; set +a; fi; \
	: "$${MISTERSHELL_ACC_SSH_PORT:=12223}"; export MISTERSHELL_ACC_SSH_PORT; \
	docker compose -f docker-compose.acceptance.yml up -d --build ssh-linux

acceptance-fixture-wait:
	@if test -r "$(MISTERSHELL_ENV_FILE)"; then set -a; . "$(MISTERSHELL_ENV_FILE)"; set +a; fi; \
	: "$${MISTERSHELL_ACC_SSH_PORT:=12223}"; export MISTERSHELL_ACC_SSH_PORT; bash -c '\
		for attempt in $$(seq 1 30); do \
			if (: >/dev/tcp/127.0.0.1/$$MISTERSHELL_ACC_SSH_PORT) 2>/dev/null; then \
				echo "SSH acceptance fixture is ready on port $$MISTERSHELL_ACC_SSH_PORT"; exit 0; \
			fi; \
			sleep 1; \
		done; \
		echo "SSH acceptance fixture did not become ready" >&2; exit 1'

acceptance-fixture-down:
	docker compose -f docker-compose.acceptance.yml down

acceptance-preflight:
	@test -r "$(MISTERSHELL_ENV_FILE)" || { echo "missing acceptance environment file: $(MISTERSHELL_ENV_FILE)" >&2; exit 1; }
	@set -a; . "$(MISTERSHELL_ENV_FILE)"; set +a; \
	test -n "$$MISTERSHELL_URL" && test -n "$$MISTERSHELL_API_KEY" || { echo "MISTERSHELL_URL and MISTERSHELL_API_KEY are required" >&2; exit 1; }; \
	curl_insecure=; case "$$MISTERSHELL_INSECURE" in 1|[Tt][Rr][Uu][Ee]) curl_insecure=-k;; esac; \
	curl -fsS $$curl_insecure -H "Authorization: Bearer $$MISTERSHELL_API_KEY" "$$MISTERSHELL_URL/api/v1/locations?size=1" >/dev/null

# Delete orphaned tfacc- test objects left behind by a crashed run.
sweep:
	set -a; . "$(MISTERSHELL_ENV_FILE)"; set +a; go test ./internal/provider/ -v -timeout 10m -sweep=mistershell

fmt:
	go fmt ./...

# Regenerate stable enums from the MisterShell OpenAPI spec
# (internal/client/types_gen.go). Point MISTERSHELL_OPENAPI at a
# local checkout of the (private) backend's ui/openapi.json. See internal/gen/types
# and README "Development".
generate:
	go generate ./...
	go fmt ./...

# Regenerate the Terraform Registry docs (docs/) from the schema Description
# fields, the examples/ directory, and templates/ via tfplugindocs.
# Requires tfplugindocs, terraform, and go on PATH.
docs:
	tfplugindocs generate --provider-name mistershell --rendered-provider-name MisterShell

clean:
	rm -f terraform-provider-mistershell

.PHONY: default build install test test-live test-live-onboarding test-live-core test-live-policy-auth test-live-vaults test-live-ai test-live-edge test-live-staged test-e2e acceptance-fixture-up acceptance-fixture-wait acceptance-fixture-down acceptance-preflight sweep fmt generate docs clean

# Run against a uniquely owned disposable Core and real SSH/vault fixtures.
# The image must be built from MISTERSHELL_SOURCE with its test licensing key.
MISTERSHELL_SOURCE ?= ../mistershell
MISTERSHELL_TEST_IMAGE ?= mistershell-core:tf-provider-compat
MISTERSHELL_TEST_PYTHON ?= $(MISTERSHELL_SOURCE)/.venv/bin/python

test-disposable:
	"$(MISTERSHELL_TEST_PYTHON)" testdata/disposable/run.py \
		--source "$(MISTERSHELL_SOURCE)" --image "$(MISTERSHELL_TEST_IMAGE)" \
		-- go test ./internal/provider/ $(ACC_TEST_FLAGS)
