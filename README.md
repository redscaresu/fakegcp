# fakegcp

Local mock of the Google Cloud Platform API for offline OpenTofu and Terraform testing.

fakegcp runs as a single Go binary, tracks resource state in SQLite, and exposes GCP-shaped REST routes on one port. Modeled after [mockway](https://github.com/redscaresu/mockway) — same admin endpoints, same `apply → plan (no-op) → destroy` drift contract, different cloud.

> **This project is pre-1.0.** Use `make test` for the verified handler set; expect rough edges in services that don't yet have tests.

## Architecture

```
              +------------------+
              |  Terraform /     |   provider "google" {
              |  OpenTofu apply  |     compute_custom_endpoint = "..."
              +--------+---------+     ...
                       |             }
                       v
              +-----------------------------+
              |   fakegcp HTTP server       |
              |   chi router                |
              +-----+-----------------+-----+
                    |                 |
        +-----------+--------+ +------+---------------+
        | GCP routes         | | Admin routes         |
        |  /compute/v1/...   | |  /mock/state         |
        |    /networks       | |  /mock/state/{svc}   |
        |    /subnetworks    | |  /mock/reset         |
        |    /firewalls      | |  /mock/snapshot      |
        |    /instances      | |  /mock/restore       |
        |    /forwardingRules| |                      |
        |    /backendServices| | (no auth)            |
        |    /urlMaps        | +----------------------+
        |    /targetHttps... |          |
        |    /sslCertificates|          |
        |    /healthChecks   |          |
        |    /addresses      |          |
        |    /globalAddresses|          |
        |  /container/v1/... |          |
        |    /clusters       |          |
        |    /nodePools      |          |
        |  /sql/v1beta4/...  |          |
        |    /instances      |          |
        |    /databases      |          |
        |    /users          |          |
        |  /iam/v1/...       |          |
        |    /serviceAccounts|          |
        |    /policies       |          |
        |    /sa-keys        |          |
        |  /storage/v1/...   |          |
        |    /buckets        |          |
        |  /pubsub/v1/...    |          |
        |  /dns/v1/...       |          |
        |  /run/v2/...       |          |
        |  /secretmanager... |          |
        +---------+----------+          |
                  |                     |
                  v                     v
              +---------------------------------+
              |   SQLite repository             |
              |   - PRAGMA foreign_keys = ON    |
              |   - SetMaxOpenConns(1)          |
              |   - .snapshot file on demand    |
              |   - cascade delete via FKs      |
              +---------------------------------+

   Auth header  : Authorization: Bearer <any non-empty token>
   Resource ids : numeric strings, GCP-shaped self-links
   Timestamps   : RFC3339
   Mutation API : returns Operation{status:"DONE", ...}
   List empty   : items key OMITTED (matches GCP wire shape)
   Errors       : {"error":{"code":N,"message":"...","errors":[...]}}
```

## Consumer

[`infrafactory`](https://github.com/redscaresu/infrafactory) drives fakegcp as the Layer-2 mock-deploy backend for `cloud: gcp` scenarios. The cross-repo e2e helpers in `internal/e2e/helpers.go` build fakegcp from this source tree on a free port for every GCP test, and the topology derivation in `internal/harness/topology_derive_gcp.go` reads `/mock/state` to evaluate connectivity, http_probe, and policy criteria. Any change you ship to fakegcp's resource shapes (notably `compute.instances`, `sql.instances`, `lb.global_forwarding_rules`, `container.clusters`) is exercised by infrafactory's GCP training scenarios automatically.

## Install / Run

```bash
go build -o fakegcp ./cmd/fakegcp
./fakegcp --port 8080
```

In-memory SQLite by default; pass `--db ./fakegcp.db` for state across restarts.

Point Terraform at it via the per-service `*_custom_endpoint` overrides:

```hcl
provider "google" {
  project                       = "test-project"
  compute_custom_endpoint       = "http://localhost:8080/compute/v1/"
  container_custom_endpoint     = "http://localhost:8080/container/v1/"
  cloud_resource_manager_custom_endpoint = "http://localhost:8080/cloudresourcemanager/v1/"
  iam_custom_endpoint           = "http://localhost:8080/iam/v1/"
  storage_custom_endpoint       = "http://localhost:8080/storage/v1/"
  sql_custom_endpoint           = "http://localhost:8080/sql/v1beta4/"
  pubsub_custom_endpoint        = "http://localhost:8080/pubsub/v1/"
  dns_custom_endpoint           = "http://localhost:8080/dns/v1/"
  cloud_run_custom_endpoint     = "http://localhost:8080/run/v2/"
  secret_manager_custom_endpoint = "http://localhost:8080/secretmanager/v1/"
}
```

`Authorization: Bearer <anything>` is accepted on all GCP routes; admin routes are unauthenticated.

## Admin endpoints

| Route | Method | Purpose |
|---|---|---|
| `/mock/state` | GET | Full state snapshot, all services |
| `/mock/state/{service}` | GET | Single-service slice |
| `/mock/reset` | POST | Truncate all tables AND clear `.snapshot` baseline |
| `/mock/snapshot` | POST | `VACUUM INTO` a baseline snapshot |
| `/mock/restore` | POST | Replace DB with the baseline snapshot |

Reset → Restore returns to an empty state; the snapshot baseline is intentionally cleared on Reset (mirrors mockway's contract — see `repository.Reset`).

## Make targets

```bash
make build          # binary at ./fakegcp
make test           # go test -count=1 ./...
make test-race      # go test -race ./...
make test-short     # go test -short ./...
make test-coverage  # writes coverage.out + coverage.html
make vet
make clean
make run            # build + run on :8080
```

`make test-coverage` excludes the `repository` and `models` packages from coverage instrumentation since they have no tests yet (S41-T2 will fill that in). Current handlers package coverage: ~64%.

## Resource shape conventions

GCP differs from Scaleway in three ways infrafactory's topology derivation cares about:

1. **Field naming**: GCP REST API uses camelCase (`portRange`, `sourceRanges`, `ipv4Enabled`, `authorizedNetworks`). fakegcp persists request bodies verbatim, so `/mock/state` surfaces camelCase. infrafactory's GCP topology derivation accepts both camelCase and snake_case for backward compatibility.

2. **Mutations return Operation**: Create/Update/Delete return `{kind:"compute#operation", status:"DONE", ...}` rather than the resource itself.

3. **Self-link rewriting**: fakegcp rewrites `zone` and `region` fields on Create to full self-link URLs (e.g. `http://HOST/compute/v1/projects/PROJECT/zones/europe-west1-a`). Downstream consumers (and infrafactory's `policies/gcp/region_restriction.rego` `deny_state` rule) handle this by stripping the trailing path segment.

## Project Layout

```
fakegcp/
├── cmd/fakegcp/        # main binary
├── handlers/            # per-service HTTP handlers
├── repository/          # SQLite repository (CRUD + snapshot/restore)
├── models/              # error types
├── testutil/            # test server + JSON request helpers
├── examples/            # working & misconfigured Terraform examples
├── scripts/             # smoke harness
├── AGENTS.md            # AI-agent working agreement
├── PLAN.md              # phase-based delivery plan
└── README.md            # this file
```

## Status

| Service | Verified | Notes |
|---|---|---|
| Compute (instances, networks, subnetworks, firewalls, disks, addresses) | ✅ | self-link rewrite on Create |
| Compute LB stack (forwarding rules, backend services, health checks, URL maps, target HTTPS proxies, SSL certs) | ✅ | global only |
| Container / GKE (clusters, node pools) | ✅ | cluster delete cascades to pools |
| Cloud SQL (instances, databases, users) | ✅ | private + public IP configurations |
| IAM (service accounts, policies, sa-keys) | ✅ | fully-qualified `serviceAccount:` principals |
| Storage (buckets) | ✅ | uniform bucket-level access, encryption |
| Pub/Sub (topics, subscriptions) | ⚠️ handler only | |
| DNS (managed zones, record sets) | ⚠️ handler only | |
| Cloud Run (services) | ⚠️ handler only | |
| Secret Manager (secrets, versions) | ⚠️ handler only | |
| Operations | ✅ | always-DONE shim |
