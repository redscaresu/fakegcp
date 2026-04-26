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
  compute_custom_endpoint         = "http://localhost:8080/compute/v1/"
  container_custom_endpoint       = "http://localhost:8080/"
  sql_custom_endpoint             = "http://localhost:8080/sql/v1beta4/"
  iam_custom_endpoint             = "http://localhost:8080/"
  storage_custom_endpoint         = "http://localhost:8080/storage/v1/"
  cloud_resource_manager_custom_endpoint = "http://localhost:8080/"
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

## Services in Scope (Phase 1)

| Service | Path Prefix | Terraform Resources |
|---------|-------------|-------------------|
| Compute | `/compute/v1/projects/{project}/` | instance, network, subnetwork, firewall, disk, address |
| Container (GKE) | `/v1/projects/{project}/locations/{location}/` | cluster, node_pool |
| Cloud SQL | `/sql/v1beta4/projects/{project}/` | database_instance, database, user |
| IAM | `/v1/projects/{project}/` | service_account, service_account_key |
| Storage | `/storage/v1/` | bucket |

## Testing

- **Double-apply idempotency**: `terraform apply` twice — second must be no-op. Drift = GET response shape mismatch.
- **Test helpers**: `testutil.NewTestServer(t)` — httptest.Server + in-memory SQLite.
- **E2E**: `go test -tags provider_e2e ./e2e -v` — auto-discovers `examples/working/` and runs apply-plan-destroy.
- **Handler tests**: full HTTP round-trips (Create-Get-List-Delete-404), FK rejection (404/409).

```bash
go test ./...                                    # unit + integration
go test -tags provider_e2e ./e2e -v             # e2e (needs terraform/tofu in PATH)
```

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
