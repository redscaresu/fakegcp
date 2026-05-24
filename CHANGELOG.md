# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Memorystore: `google_redis_instance` handler + repository support (fakegcp@9212459). End-to-end lifecycle parity for `apply → plan-no-op → destroy`.
- `.github/workflows/docker.yml` triggers an image build on every CI success (fakegcp@6bc8f01); `Dockerfile` cleanup.
- README "API Compatibility" section documenting the wire-shape contract + the `examples/working/<svc>` smoke harness every handler is validated against (fakegcp@205a3ea).
- 134 handler tests covering GCP Compute (networks, subnetworks, firewalls, disks, instances, addresses), Container (clusters, node pools), Cloud SQL (instances, databases, users), IAM (service accounts, keys, bindings), Storage, DNS, Pub/Sub, Secret Manager, Cloud Run, and Load Balancer.
- `repository/repository_test.go` (881 lines, 27 test functions) covering CRUD for all 15 named tables + schema migration + FK enforcement (S41-T2).
- `handlers/fk_violation_test.go` (323 lines, 17 HTTP-layer FK violation tests) + `handlers/cascade_delete_test.go` (237 lines, 6 cascade tests) (S41-T3, T4).
- 10 working terraform examples + 9 misconfigured + 7 updates for integration testing.
- `scripts/e2e.sh` (S41-T6) double-apply idempotency harness gated by `FAKEGCP_ENABLE_E2E=1`.
- Admin endpoints (`/mock/state`, `/mock/reset`, `/mock/snapshot`, `/mock/restore`) and per-service state filtering (`/mock/state/{service}`).
- `.github/workflows/ci.yml` (added 2026-05-23) — lint, build, test (-race), gitleaks.
- `.githooks/pre-commit` + `make install-hooks` parity with fakeaws.

### Fixed
- `ListSAKeys` and `ListSQLUsers` returned 200 + empty list when the parent resource was missing; now return 404 to match real Cloud IAM / Cloud SQL behavior (fakegcp@e3ec44f).
- `cloud_sql_custom_endpoint` typo in provider examples — corrected to `sql_custom_endpoint` (matches google provider v7) (fakegcp@b617d2f).
- `Repository.Reset()` now removes the `*.snapshot` baseline alongside table truncation, preventing a follow-up Restore from resurrecting pre-reset state.
- M44 — DNS zone handler `stripEmptyDNSZoneSubObjects` + `isShallowEmpty` helper eliminate phantom drift from provider-sent empty placeholder maps (`{"networks": []}` etc.).
- M45 — GKE cluster + node pool idempotency: CreateCluster auto-creates `default-pool` (so `remove_default_node_pool=true` can succeed), `stripEmptyClusterSubObjects` removes provider-sent default sub-blocks, networkConfig.{network,subnetwork} populated for terraform-provider-google's Read flow, CreateNodePool mirrors `nodeCount` from `initialNodeCount` and populates `instanceGroupUrls`. New synthetic `GET /compute/v1/projects/{p}/zones/{z}/instanceGroupManagers[/{name}]` returns TargetSize derived from the matching node pool — provider sums TargetSize across URLs to compute `node_count`.
- M46 — `CreateSAKey` now echoes back `keyAlgorithm`, `privateKeyType`, and `keyOrigin` (missing fields previously surfaced as "forces replacement" drift in `google_service_account_key`).
- M47 — Storage bucket idempotency: `stripEmptyBucketSubObjects` strips provider-sent placeholder sub-blocks, location normalized to UPPER_CASE, populated default `projectNumber` / `locationType` / `publicAccessPrevention` / `rpo` / `softDeletePolicy`.
- M48 — Added `GET /compute/v1/projects/{project}/zones/{zone}` returning a static zone descriptor (provider pre-flights zones before `google_compute_instance` create).
- M49 — Reduced `numericID()` from 19 to 18 digits; 19-digit values could exceed int64 max (9.22e18) and terraform-provider-google rejected them as "expected type 'int', got unconvertible type 'string'".
- M50 — Added `GET /compute/v1/projects/{project}/global/images/{image}` + `.../family/{family}` returning a static image descriptor for public-image lookups.

### Security
- `gitleaks` pre-commit hook installable via `make install-hooks`; `.gitleaks.toml` config shipped.
- `SECURITY.md` with private vulnerability reporting via GitHub Security Advisories.
- Apache-2.0 LICENSE (added 2026-05-23).
