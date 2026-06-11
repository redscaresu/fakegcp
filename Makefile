.PHONY: install-hooks build test test-race test-coverage test-short test-e2e vet clean run demo-help demo-up demo-down demo-env demo-shell demo-apply demo-destroy demo-clean

# install-hooks wires the tracked hook installer at .githooks/ via
# core.hooksPath so the gitleaks + go test pre-commit gate runs locally
# on every commit. Mirrors fakeaws/mockway pattern.
install-hooks:
	git config core.hooksPath .githooks
	chmod +x .githooks/pre-commit
	@echo "Hooks installed: pre-commit will run gitleaks then go test."

build:
	go build -o fakegcp ./cmd/fakegcp

test:
	go test -count=1 ./...

test-race:
	go test -count=1 -race ./...

test-short:
	go test -count=1 -short ./...

# test-coverage runs the suite, writes a profile, prints a total summary,
# and emits an HTML report at coverage.html. Useful for spotting untested
# handler paths during S41-T2..T7 work. Excludes the repository and models
# packages from coverage instrumentation since they have no tests yet
# (S41-T2 will fill that in).
test-coverage:
	go test -count=1 -coverprofile=coverage.out -covermode=count ./handlers/...
	@go tool cover -func=coverage.out | tail -1
	@go tool cover -html=coverage.out -o coverage.html
	@echo "coverage report: coverage.html"

# test-e2e runs the double-apply idempotency harness against every
# examples/working/* directory using a freshly built fakegcp. Gated by
# FAKEGCP_ENABLE_E2E=1 to keep it out of the default `make test` path.
# Requires tofu or terraform on PATH.
test-e2e:
	@if [ "$$FAKEGCP_ENABLE_E2E" != "1" ]; then \
		echo "test-e2e is gated by FAKEGCP_ENABLE_E2E=1"; exit 0; \
	fi
	bash scripts/e2e.sh

vet:
	go vet ./...

clean:
	rm -f fakegcp coverage.out coverage.html

run: build
	./fakegcp --port 8080

# ─── demo targets ───────────────────────────────────────────────────────
# Drive a real google provider through init → apply → plan-no-op → destroy
# against a local fakegcp. Useful for blog demos, manual exploration, and
# proving the wire-shape contract end-to-end.
#
#   make demo-apply                       # one-shot: up + apply + plan-no-op (default: secret_manager)
#   make demo-apply EXAMPLE=iam           # pick a different example
#   make demo-shell                       # bash subshell with env set + cd'd to example
#   make demo-down                        # kill fakegcp + remove temp files
#
# Override the example with EXAMPLE=<dir> (any subdir of examples/working/).
# Each example's providers.tf hardcodes localhost:8080 as the endpoint, so
# DEMO_PORT is effectively pinned at 8080.
DEMO_PORT      ?= 8080
EXAMPLE        ?= secret_manager
DEMO_EXAMPLE_DIR := examples/working/$(EXAMPLE)
DEMO_ENV_FILE  := /tmp/fakegcp.env
DEMO_BASE      := http://localhost:$(DEMO_PORT)
DEMO_BIN       := $(shell command -v tofu 2>/dev/null || command -v terraform 2>/dev/null)

demo-help:
	@echo "Demo targets (drive real terraform/tofu against this fakegcp):"
	@echo "  demo-up                        boot fakegcp + write env to /tmp"
	@echo "  demo-apply [EXAMPLE=<dir>]     one-shot: init + apply + plan-no-op"
	@echo "  demo-shell [EXAMPLE=<dir>]     bash subshell with env set + cd'd to example"
	@echo "  demo-destroy [EXAMPLE=<dir>]   tofu destroy on the current example"
	@echo "  demo-down                      kill fakegcp + remove temp files"
	@echo "  demo-clean                     demo-destroy + nuke .terraform/ + state files"
	@echo ""
	@echo "Available examples:"
	@ls examples/working/ | sed 's/^/  /'

demo-up:
	@if pgrep -f "fakegcp --port $(DEMO_PORT)" >/dev/null 2>&1; then \
	  echo "✓ fakegcp already running on :$(DEMO_PORT)"; \
	else \
	  [ -x ./fakegcp ] || { echo "ERROR: ./fakegcp binary not found. Run 'make build' first." >&2; exit 1; }; \
	  ./fakegcp --port $(DEMO_PORT) --db ':memory:' >/tmp/fakegcp.log 2>&1 & \
	  for i in 1 2 3 4 5 6 7 8 9 10; do sleep 0.5; curl -sf $(DEMO_BASE)/mock/state >/dev/null 2>&1 && break; done; \
	  echo "✓ fakegcp booted on :$(DEMO_PORT)  (logs: /tmp/fakegcp.log)"; \
	fi
	@{ \
	  echo 'export GOOGLE_PROJECT=fake-project'; \
	  echo 'export GOOGLE_REGION=us-central1'; \
	  echo 'export GOOGLE_ZONE=us-central1-a'; \
	  echo 'export GOOGLE_OAUTH_ACCESS_TOKEN=fake-token'; \
	  echo 'export GOOGLE_CREDENTIALS='; \
	} > $(DEMO_ENV_FILE)
	@echo "✓ env written to $(DEMO_ENV_FILE)"

demo-down:
	@pkill -f "fakegcp --port $(DEMO_PORT)" 2>/dev/null && echo "✓ killed" || echo "✓ nothing to kill"
	@rm -f $(DEMO_ENV_FILE)

demo-env: demo-up
	@cat $(DEMO_ENV_FILE)

demo-shell: demo-up
	@[ -d "$(DEMO_EXAMPLE_DIR)" ] || { echo "ERROR: $(DEMO_EXAMPLE_DIR) not found" >&2; exit 1; }
	@echo "→ entering subshell with fakegcp env. Type 'exit' to leave."
	@cd $(DEMO_EXAMPLE_DIR) && /bin/bash --rcfile <(echo "source ~/.bashrc 2>/dev/null; source $(DEMO_ENV_FILE); PS1='[fakegcp $(EXAMPLE)] $$PS1'")

demo-apply: demo-up
	@[ -n "$(DEMO_BIN)" ] || { echo "ERROR: neither tofu nor terraform on PATH" >&2; exit 1; }
	@[ -d "$(DEMO_EXAMPLE_DIR)" ] || { echo "ERROR: $(DEMO_EXAMPLE_DIR) not found" >&2; exit 1; }
	@set -e; . $(DEMO_ENV_FILE); cd $(DEMO_EXAMPLE_DIR); \
	  echo "=== $(DEMO_BIN) init ==="; $(DEMO_BIN) init -input=false; \
	  echo ""; echo "=== $(DEMO_BIN) apply ==="; $(DEMO_BIN) apply -auto-approve -input=false; \
	  echo ""; echo "=== $(DEMO_BIN) plan -detailed-exitcode (brutal correctness check) ==="; \
	  if $(DEMO_BIN) plan -detailed-exitcode -input=false >/dev/null 2>&1; then \
	    echo "✓ exit 0 — wire shape correct (real provider's state matches fakegcp's responses)."; \
	  else \
	    echo "✗ exit $$? — drift detected."; exit 1; \
	  fi

demo-destroy:
	@[ -n "$(DEMO_BIN)" ] || { echo "ERROR: neither tofu nor terraform on PATH" >&2; exit 1; }
	@[ -f $(DEMO_ENV_FILE) ] || { echo "ERROR: no env file — run 'make demo-up' first" >&2; exit 1; }
	@set -e; . $(DEMO_ENV_FILE); cd $(DEMO_EXAMPLE_DIR); $(DEMO_BIN) destroy -auto-approve -input=false

demo-clean:
	@-$(MAKE) demo-destroy 2>/dev/null
	@find examples/working -name '.terraform' -type d -prune -exec rm -rf {} + 2>/dev/null || true
	@find examples/working -name '.terraform.lock.hcl' -delete 2>/dev/null || true
	@find examples/working -name 'terraform.tfstate*' -delete 2>/dev/null || true
	@echo "✓ terraform state cleaned"
