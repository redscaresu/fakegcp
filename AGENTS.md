# fakegcp Agent Working Agreement

For AI coding agents working on the stateful GCP API mock.

## Mission
Build `fakegcp`, a stateful mock of the Google Cloud Platform API for offline Terraform/OpenTofu testing. Think LocalStack, but for GCP. Single Go binary, SQLite state, path-based routing. Modeled after [mockway](https://github.com/redscaresu/mockway).

## Architecture

- Single HTTP server, path-based routing via **chi**
- SQLite with `PRAGMA foreign_keys = ON` for referential integrity
- Auth: `Authorization: Bearer <token>` header required on all GCP routes (any non-empty value accepted)
- Admin endpoints under `/mock/` (no auth): reset, snapshot, restore, state
- `db.SetMaxOpenConns(1)` mandatory (same reason as mockway: `:memory:` isolation + FK enforcement)

**The Terraform provider SDK is the contract**: fakegcp must return responses in the exact shape the Google provider expects. Wrong shapes cause silent drift or provider panics.

## GCP API Conventions (differs from Scaleway)

| Aspect | Scaleway (mockway) | GCP (fakegcp) |
|--------|-------------------|---------------|
| Auth header | `X-Auth-Token` | `Authorization: Bearer <token>` |
| Resource identity | Auto-generated UUID | User-provided **name** (string) |
| Unique key | UUID | `project + scope(zone/region/global) + name` |
| `id` field | UUID (primary key) | Numeric string (display only, auto-generated) |
| `selfLink` | Not used | Computed URL for every resource |
| Mutations return | Resource directly | **Operation** object (always DONE in fakegcp) |
| List empty | `{"resources": [], "total_count": 0}` | Omit `"items"` key entirely |
| Error format | `{"message": "...", "type": "..."}` | `{"error": {"code": N, "message": "...", "errors": [...]}}` |
| Endpoint routing | Single base URL (`SCW_API_URL`) | Per-service `*_custom_endpoint` overrides |

### Operations Model
GCP mutations return an Operation object. The provider polls until `status: "DONE"`. fakegcp returns operations as immediately DONE with embedded `targetLink` — avoids polling complexity while satisfying the provider.

### Per-Service Endpoint Overrides
The Google provider requires each service endpoint to be set individually:
```hcl
provider "google" {
  compute_custom_endpoint                = "http://localhost:8080/compute/v1/"
  container_custom_endpoint              = "http://localhost:8080/"
  sql_custom_endpoint                    = "http://localhost:8080/sql/v1beta4/"
  iam_custom_endpoint                    = "http://localhost:8080/v1/"
  storage_custom_endpoint                = "http://localhost:8080/storage/v1/"
  cloud_resource_manager_custom_endpoint = "http://localhost:8080/v1/"
  pubsub_custom_endpoint                 = "http://localhost:8080/v1/"
  dns_custom_endpoint                    = "http://localhost:8080/dns/v1/"
  cloud_run_v2_custom_endpoint           = "http://localhost:8080/v2/"
  secret_manager_custom_endpoint         = "http://localhost:8080/v1/"
}
```

## Project Structure

```
fakegcp/
├── cmd/fakegcp/main.go       # Entrypoint, DI wiring
├── handlers/                  # Per-service handlers (compute, network, container, sql, iam, storage, operations, admin)
├── repository/repository.go   # SQLite schema, CRUD, FK enforcement
├── models/models.go           # ErrNotFound, ErrConflict, ErrAlreadyExists
├── testutil/testutil.go       # NewTestServer, HTTP helpers
└── examples/                  # working/ (idempotent) + misconfigured/ (FK violations)
```

**Key pattern**: DI via `Application` struct. Handlers are thin — delegate to repository. Repository returns domain errors; handlers map to HTTP status codes.

## Services in Scope

| Service | Path Prefix | Terraform Resources |
|---------|-------------|-------------------|
| Compute | `/compute/v1/projects/{project}/` | instance, network, subnetwork, firewall, disk, address |
| Container (GKE) | `/v1/projects/{project}/locations/{location}/` | cluster, node_pool |
| Cloud SQL | `/sql/v1beta4/projects/{project}/` | database_instance, database, user |
| IAM | `/v1/projects/{project}/` | service_account, service_account_key |
| Storage | `/storage/v1/` | bucket |
| DNS | `/dns/v1/projects/{project}/` | managed_zone, record_set |
| Pub/Sub | `/v1/projects/{project}/` | topic, subscription |
| Secret Manager | `/v1/projects/{project}/` | secret, secret_version |
| Cloud Run | `/v2/projects/{project}/locations/{location}/` | service |
| Memorystore | `/v1/projects/{project}/locations/{location}/` | redis_instance |

## Testing

- **Double-apply idempotency**: `terraform apply` twice — second must be no-op. Drift = GET response shape mismatch.
- **Test helpers**: `testutil.NewTestServer(t)` — httptest.Server + in-memory SQLite.
- **E2E**: `go test -tags provider_e2e ./e2e -v` — auto-discovers `examples/working/` and runs apply-plan-destroy.
- **Handler tests**: full HTTP round-trips (Create-Get-List-Delete-404), FK rejection (404/409).
- **Assertion convention (project-wide)**: default to `github.com/stretchr/testify` for assertions where possible. `assert.Equal` / `require.NoError` / `assert.Contains` over `if x != y { t.Fatalf(...) }`. Use `require` when a failure should stop the test (setup steps, anything whose failure would cause downstream nil-deref / panic). Use `assert` otherwise so multiple failures surface in one run. Don't bare-literal HTTP status codes — `http.StatusNoContent` not `204`. The qualifier "where possible" is real: if an if-fatalf block carries a fundamentally custom error message the assertion library can't express, leave it stdlib — don't force conversions that hurt readability. Same rule lives in every sibling fake's AGENTS.md.

```bash
go test ./...                                    # unit + integration
go test -tags provider_e2e ./e2e -v             # e2e (needs terraform/tofu in PATH)
```

## Provider smoke harness

`examples/provider_smoke_test.go` is the canonical wire-fidelity gate. Every directory under `examples/{working,misconfigured,updates}/` is auto-discovered and run through a per-tree contract. **No real GCP credentials needed** — the real `hashicorp/google` provider binary runs against this fake; if the provider's CRUD lifecycle works, the wire shape is correct by construction.

| Tree | Contract |
|---|---|
| `examples/working/<svc>/` | `tofu apply → plan -detailed-exitcode (no diff) → destroy` |
| `examples/misconfigured/<svc>/` | `tofu apply` MUST fail; if `expected.txt` is present, output MUST contain that fragment (e.g. `googleapi: Error 404`) |
| `examples/updates/<svc>/` | `tofu apply -var-file=v1.tfvars → plan no-op → apply -var-file=v2.tfvars → plan no-op → destroy` (idempotency under change) |

For every example dir the test starts a fresh `fakegcp` binary on a free port (no shared mock state across dirs), copies the example into a temp dir, and rewrites `localhost:8080` in `providers.tf` to point at the per-test port. Each subdir is its own `t.Run` sub-test.

Gating: `FAKEGCP_ENABLE_E2E=1`. Without the env var, the test `t.Skip`s with a clear message.

`examples/known_broken.yaml` is the only-shrink allowlist: dirs listed there are *expected* to drift and don't fail the gate. If a listed dir stops drifting, the test fails ("congratulations, remove this entry") — ratchet-only-tighten.

**When you add a new resource handler**: add an `examples/working/<resource>/` config that exercises CRUD. If your handler models a documented error path, add an `examples/misconfigured/<resource>/` config + `expected.txt`. If your handler has Update semantics distinct from Create, add an `examples/updates/<resource>/` v1→v2 pair.

## Fidelity strategy

fakegcp is **hybrid**: handler shapes were discovered through a mix of (a) `terraform-provider-google` source reading, (b) GCP discovery documents (Google's OpenAPI equivalent), and (c) provider-as-validator via the smoke harness. Discovery docs are referenced ad-hoc — there's no `specs/` tree.

This works but has slower per-resource discovery than mockway's spec-driven pattern. **For new handlers**, consider downloading the relevant GCP discovery doc (e.g. `https://compute.googleapis.com/$discovery/rest?version=v1` for compute) and committing into `specs/` — same shape as mockway. Existing handlers are stable; opportunistic upgrade rather than mandatory rework.

**Comparison with sibling fakes**: mockway is fully spec-driven (`specs/` tree, "Reverse fidelity" rule); fakeaws is reactive (provider source + `TF_LOG=DEBUG` capture); fakegenesys mirrors mockway's spec-driven approach via a filtered Genesys OpenAPI doc. See `../infrafactory/AGENTS.md` § "Sibling-fake fidelity strategies".

## Contract-coverage convention (canonical across all 4 sibling fakes)

`handlers/contract_audit_test.go` enforces the `CRITICAL[<id>]:` / `MUST[<id>]:` docstring → `TestContract_<id>` test pairing across `handlers/*.go`. A wire-shape invariant the consuming `terraform-provider-google` depends on must NOT live as a comment alone — drift becomes a failed `go test`, not a missed code review. Current contracts (as of S129 fakegcp sibling rollout): `cluster-cascade-deletes-nodepools`, `sql-instance-cascade-deletes-databases`, `sql-instance-cascade-deletes-users`, `service-account-cascade-deletes-keys`. Same convention live across mockway/fakeaws/fakegenesys.

## API Fidelity Principles

Same philosophy as mockway:
- **Must enforce**: FK references (404), dependency ordering (409), response shapes matching provider expectations.
- **Must NOT enforce**: field value constraints, required fields (provider validates before HTTP call), rate limiting.
- **Litmus test**: if a config passes fakegcp but fails on real GCP due to FK/dependency issues, that is a fakegcp bug.

## Checklist for New Handlers

- [ ] Create returns Operation (status: DONE), not the resource directly
- [ ] GET returns resource with `id`, `selfLink`, `creationTimestamp`
- [ ] List omits `items` key when empty (GCP convention)
- [ ] Auth middleware applied (Bearer token)
- [ ] FK violations return GCP-format 404 error
- [ ] Dependent resources block parent deletion (409)
- [ ] New tables added to both `init()` and `Reset()`
- [ ] New tables included in `FullState()` and `ServiceState()`
- [ ] Working example added to `examples/working/`
- [ ] Handler test with full CRUD lifecycle

## Safe Workflow

1. Add/adjust repository logic
2. Wire handlers and error mapping
3. Add handler test
4. Add working example in `examples/working/`
5. `go test ./...`
6. `go test -tags provider_e2e ./e2e -v`

## CLI Flags
```
fakegcp --port 8080                    # Default: 8080, in-memory DB
fakegcp --port 8080 --db ./fakegcp.db  # File-backed persistence
fakegcp --echo --port 8080             # Echo mode for endpoint discovery
```
