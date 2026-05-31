# fakegcp

[![ci](https://github.com/redscaresu/fakegcp/actions/workflows/ci.yml/badge.svg)](https://github.com/redscaresu/fakegcp/actions/workflows/ci.yml)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
[![Go 1.25+](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go)](go.mod)

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
        |  /v1/projects/{p}/ |          |
        |    locations/{l}/  |          |
        |      /clusters     |          |
        |      /nodePools    |          |
        |  /sql/v1beta4/...  |          |
        |    /instances      |          |
        |    /databases      |          |
        |    /users          |          |
        |  /v1/projects/{p}/ |          |
        |    /serviceAccounts|          |
        |    /secrets        |          |
        |    /topics         |          |
        |    /subscriptions  |          |
        |  /storage/v1/b/... |          |
        |  /dns/v1/...       |          |
        |  /v2/projects/...  |          |
        |    /services       |          |
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

[`infrafactory`](https://github.com/redscaresu/infrafactory) drives fakegcp as the Layer-2 mock-deploy backend for `cloud: gcp` scenarios. The cross-repo e2e helpers in `internal/e2e/helpers.go` build fakegcp from this source tree on a free port for every gated GCP e2e test (`TestE2E_GCP*`), and the topology derivation in `internal/harness/topology_derive_gcp.go` reads `/mock/state` to evaluate connectivity, http_probe, and policy criteria. The seven services with `examples/working/` + `examples/updates/` coverage (Pub/Sub, DNS, Cloud Run, Secret Manager, Compute LB, IAM, Storage) are driven through infrafactory's gated GCP e2e tests when run with `INFRAFACTORY_ENABLE_E2E=1`; the FK-violation paths are pinned by the subset of those services that have `examples/misconfigured/` entries (Pub/Sub, DNS, Secret Manager, Compute LB, plus the Compute instance / network FK). Other resource shapes (e.g. `compute.instances`, `sql.instances`, `container.clusters`) are exercised by handler unit tests in `handlers/handlers_test.go` rather than end-to-end tofu drives.

## Install / Run

```bash
go build -o fakegcp ./cmd/fakegcp
./fakegcp --port 8080
```

In-memory SQLite by default; pass `--db ./fakegcp.db` for state across restarts.

### Docker

Pre-built multi-arch images are published to GitHub Container Registry on every push to `main`:

```bash
docker run --rm -p 8080:8080 ghcr.io/redscaresu/fakegcp:latest --port 8080
```

The Dockerfile in the repo root produces a `~15MB` static image (multi-stage build from `golang:1.25-alpine`).

Point Terraform at it via the per-service `*_custom_endpoint` overrides. The path you append to `localhost:8080` must match where fakegcp registers each service in `handlers/handlers.go` (`/v1/projects/...` for IAM, Pub/Sub, Secret Manager and Cloud Resource Manager; `/v2/...` for Cloud Run; service-prefixed paths for Compute, DNS, Cloud SQL, Storage):

```hcl
provider "google" {
  project                                = "test-project"
  compute_custom_endpoint                = "http://localhost:8080/compute/v1/"
  container_custom_endpoint              = "http://localhost:8080/"
  cloud_resource_manager_custom_endpoint = "http://localhost:8080/v1/"
  iam_custom_endpoint                    = "http://localhost:8080/v1/"
  storage_custom_endpoint                = "http://localhost:8080/storage/v1/"
  sql_custom_endpoint                    = "http://localhost:8080/sql/v1beta4/"
  pubsub_custom_endpoint                 = "http://localhost:8080/v1/"
  dns_custom_endpoint                    = "http://localhost:8080/dns/v1/"
  cloud_run_v2_custom_endpoint           = "http://localhost:8080/v2/"
  secret_manager_custom_endpoint         = "http://localhost:8080/v1/"
}
```

Each `examples/working/<service>` directory has a `providers.tf` set up with just the endpoint(s) that example uses — copy from there if in doubt.

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

## API compatibility

The point of fakegcp is to be wire-shape compatible with the real `hashicorp/google` provider — every byte the provider sends or expects to receive must match what real GCP would do, or the provider detects "drift" and the apply loop fails. Three guardrails enforce this; they're identical across [`mockway`](https://github.com/redscaresu/mockway) (Scaleway), [`fakegcp`](https://github.com/redscaresu/fakegcp) (GCP), and [`fakeaws`](https://github.com/redscaresu/fakeaws) (AWS).

### 1. Three example trees, auto-discovered

Every directory under `examples/` is an executable contract against a real Terraform/OpenTofu provider:

| Tree | Contract |
|---|---|
| `examples/working/<svc>/` | `apply → plan -detailed-exitcode 0 → destroy` — second plan MUST be a no-op |
| `examples/misconfigured/<svc>/` | `apply` MUST fail; if `expected.txt` is present, the error output MUST contain that fragment |
| `examples/updates/<svc>/` | `apply -var-file=v1.tfvars → plan no-op → apply -var-file=v2.tfvars → plan no-op → destroy` |

`examples/provider_smoke_test.go` walks the three trees with `runtime.Caller` and registers each subdirectory as its own `t.Run` sub-test. Adding a directory adds a test — no per-example test wiring. Each sub-test boots a fresh fakegcp on a kernel-assigned port so there's no cross-example state leakage.

The **idempotency gate** (`plan -detailed-exitcode 0`) is the strongest compatibility signal: if fakegcp returns a single field with the wrong case, type, or default, the provider sees drift on the second plan and the test fails. Most of the recent M-ticket fixes (M44 DNS, M45 GKE, M47 Storage, M49 numericID overflow) were caught by this gate.

### 2. `examples/known_broken.yaml` — ratchet-only allowlist

Working examples whose idempotency gate is currently expected to fail are listed in `examples/known_broken.yaml`, each entry pointing at a tracking ticket in [`infrafactory/BACKLOG.md`](https://github.com/redscaresu/infrafactory/blob/main/BACKLOG.md). The smoke harness skips the drift assertion for allowlisted dirs but still runs `apply + destroy`.

The list is **ratchet-only-tighten**: if an allowlisted dir starts passing idempotency, the test fails with `"congratulations, remove this entry"`. Compatibility coverage can only grow, never silently regress.

### 3. Cross-repo e2e from infrafactory

[`infrafactory`](https://github.com/redscaresu/infrafactory) builds fakegcp from this source tree on a free port for every gated GCP e2e test (`TestE2E_GCP*` in `internal/e2e/`, gated by `INFRAFACTORY_ENABLE_E2E=1`). Those tests drive scenarios end-to-end through infrafactory's harness (plan → mock-apply → topology derivation → destroy), so a compatibility regression surfaces in two places: the local `make test-e2e` and the upstream infrafactory CI.

### Adding coverage for a new resource

1. Add an `examples/working/<svc>/` directory with `providers.tf` + `main.tf`.
2. Run `FAKEGCP_ENABLE_E2E=1 go test ./examples/...` — auto-discovery picks it up.
3. If it drifts: either fix the handler, or (if the fix is non-trivial) add a `known_broken.yaml` entry pointing at a new BACKLOG ticket.
4. Mirror with `examples/misconfigured/<svc>/` (FK / validation paths) and `examples/updates/<svc>/` (update paths) as the service warrants.
5. Add a `TestE2E_GCP<Svc>` in infrafactory's `internal/e2e/gcp_services_test.go` so the cross-repo gate covers the scenario flow too.
6. Append the service id to `LandedServices` in `handlers/regression_manifest.go`. This trips infrafactory's `TestCrossRepoParity_EveryLandedServiceHasScenario` (in its `internal/e2e/cross_repo_parity_test.go`) until either (a) a `scenarios/training/gcp-<svc>.yaml` is added on the infrafactory side AND a `cloudParityMap["gcp"]["<svc>"]` entry pointing at it lands in the same PR, or (b) the service is added to that test's `exempt` map with a written reason (current exemptions: `serviceusage` — meta-API exercised transitively by every GCP scenario via `google_project_service`; `memorystore` — handler landed but no infrafactory scenario yet). The parity test runs in infrafactory CI on every push, so landing here without the upstream change will break the badge — coordinate the two PRs.

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

## Documentation

- [`examples/README.md`](examples/README.md) — quickstart for running examples + the per-example breakdown
- [`AGENTS.md`](AGENTS.md) — fresh-agent entry point: layout, conventions, GCP-vs-Scaleway wire-shape differences
- [`PLAN.md`](PLAN.md) — phase-by-phase delivery history
- [`CONTRIBUTING.md`](CONTRIBUTING.md) — PR contract, quality gates, pre-commit hook setup
- [`SECURITY.md`](SECURITY.md) — disclosure policy
- [`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md) — community standards
- [`CHANGELOG.md`](CHANGELOG.md) — Keep a Changelog format

## Status

Three columns: **Handler** = HTTP routes wired, **Tests** = `handlers/handlers_test.go` covers CRUD (and FK / cascade where the resource has dependents), **Terraform example** = `examples/working/<name>` exercises a full `apply → plan (no-op exit 0) → destroy` cycle against the real `hashicorp/google` provider. A row is fully verified only when all three are checked.

| Service | Handler | Tests | Terraform example | Notes |
|---|---|---|---|---|
| Compute (instances, networks, subnetworks, firewalls, disks, addresses) | ✅ | ✅ | [`examples/working/basic_instance`](examples/working/basic_instance) | self-link rewrite on Create |
| Compute LB stack (forwarding rules, backend services, health checks, URL maps, target HTTPS proxies, SSL certs) | ✅ | ✅ FK chain | [`examples/working/load_balancer`](examples/working/load_balancer) | health-check, urlMap, sslCertificates, target, IPAddress all FK-validated on Create + Update |
| Container / GKE (clusters, node pools) | ✅ | ✅ | [`examples/working/gke_cluster`](examples/working/gke_cluster) | cluster delete cascades to pools |
| Cloud SQL (instances, databases, users) | ✅ | ✅ | [`examples/working/cloud_sql`](examples/working/cloud_sql) | private + public IP configurations |
| IAM (service accounts, policies, sa-keys, bindings) | ✅ | ✅ | [`examples/working/iam`](examples/working/iam) | fully-qualified `serviceAccount:` principals |
| Storage (buckets) | ✅ | ✅ | [`examples/working/storage`](examples/working/storage) | uniform bucket-level access, encryption |
| Pub/Sub (topics, subscriptions) | ✅ | ✅ FK | [`examples/working/pubsub`](examples/working/pubsub) | subscription topic immutable on PATCH; topic delete succeeds and subscriptions survive (real Pub/Sub orphan semantics) |
| DNS (managed zones, record sets) | ✅ | ✅ FK | [`examples/working/dns`](examples/working/dns) | mutations via v1 transactional changes; zone delete refused while rrsets exist |
| Cloud Run (services) | ✅ | ✅ CRUD | [`examples/working/cloud_run`](examples/working/cloud_run) | TestCloudRunServiceCRUD |
| Secret Manager (secrets, versions) | ✅ | ✅ FK + cascade | [`examples/working/secret_manager`](examples/working/secret_manager) | TestSecretCRUD, TestSecretVersionCRUD, TestSecretDeleteWithVersions |
| Cloud KMS (key rings, crypto keys, IAM policies) | ✅ | — | n/a | minimal stubs for CMEK-required scenarios (gcp.encryption policy); in-memory synthesised state. Added 2026-05-31 (fakegcp@c7999b5) |
| Operations | ✅ | ✅ | n/a | always-DONE shim used by every mutation |

Each entry has been driven manually through `tofu apply → mutate → tofu apply → tofu destroy` against the live `hashicorp/google` Terraform provider, and the matching infrafactory e2e tests (`TestE2E_GCP*`, gated by `INFRAFACTORY_ENABLE_E2E=1`) replay that lifecycle programmatically. Within this repo, the GitHub Actions CI workflow (`.github/workflows/ci.yml`) runs the handler unit tests + regression tests + gitleaks on every push; the `examples/` smoke gate runs under `FAKEGCP_ENABLE_E2E=1 go test ./examples/...` locally and via the upstream infrafactory CI when gated AWS/GCP e2e tests are enabled. The companion `examples/misconfigured/` and `examples/updates/` directories pin FK-violation and update-path coverage for the matching service. See [`examples/README.md`](examples/README.md) for the full lifecycle.
