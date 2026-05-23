.PHONY: build test test-race test-coverage test-short test-e2e vet clean run

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
