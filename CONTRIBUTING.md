# Contributing to fakegcp

`fakegcp` is the GCP-side mock server for the [InfraFactory](https://github.com/redscaresu/infrafactory) project. It simulates GCP APIs against a local SQLite database so InfraFactory's Layer 2 validation can run offline against a deterministic backend.

## TL;DR

1. Open an issue first for non-trivial changes.
2. Each PR is one focused change (a handler, a fidelity fix, a regression test).
3. `make test` must be green.
4. Pre-commit hook (`make install-hooks`) runs `gitleaks` + `go test`.

## Setup

Required: Go 1.24+, `make`. Optional: `gitleaks` for the pre-commit hook, OpenTofu for the double-apply harness (`scripts/e2e.sh`).

```bash
git clone https://github.com/redscaresu/fakegcp.git
cd fakegcp
make install-hooks
make test
make run    # serves the mock at :8080
```

The double-apply idempotency harness (every `examples/working/*` dir gets `tofu init && apply && plan -detailed-exitcode && destroy`) is gated by `FAKEGCP_ENABLE_E2E=1`:

```bash
FAKEGCP_ENABLE_E2E=1 make test-e2e
```

## Workflow

1. Pick a focused change — usually one handler, one repository method, or one regression test.
2. Add tests in `handlers/handlers_test.go` + `handlers/fk_violation_test.go` + `handlers/cascade_delete_test.go` as applicable. CRUD coverage in repository_test.go.
3. Run `make test` locally.
4. If you added a working example, place it under `examples/working/<name>/` with a `providers.tf` pointing at the fakegcp custom endpoints; the example will auto-register with the e2e harness.
5. Open a PR referencing the InfraFactory ticket (if any) or describing the fidelity gap being closed.

## Fidelity issues

If you find a case where terraform-provider-google behaves differently against fakegcp than against real GCP, that's a **fidelity issue**. File it with the `fidelity` label and include:

- The exact terraform-provider-google version.
- The HCL block that triggers the divergence.
- The raw HTTP request + response from both real GCP and fakegcp.

Two known fidelity gaps are documented inline in `handlers/fk_violation_test.go`:
- `ListSAKeys` returns 200 + empty list when the parent service account is missing (real GCP returns 404). **Fixed in fakegcp@e3ec44f.**
- `ListSQLUsers` same shape. **Fixed in fakegcp@e3ec44f.**

## Code of Conduct

See [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md). Contributor Covenant v2.1.

## License

By contributing, you agree your work will be released under Apache-2.0.
