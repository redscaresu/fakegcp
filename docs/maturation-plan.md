# fakegcp Maturation Plan

**Goal:** bring fakegcp to fakeaws-level maturity so it is a trustworthy **Layer-2 mock pre-filter** for a GCP real-cloud digital twin (infrafactory Layer 3).

**Status of this doc:** planning. Base off `main` on a dedicated `fakegcp-maturation` branch — do **not** fold into the `tests-testify-sweep` branch.

---

## Why this matters (the Layer 3 context)

infrafactory's real-cloud-twin architecture is a two-stage gate:

1. **Layer 2 — the mock twin (fakegcp):** cheap, instant, free. Filters structurally-broken changes before they cost real GCP cycles.
2. **Layer 3 — the real twin (real GCP):** `plan → apply → probe → destroy` against real Google APIs. Expensive, slow, authoritative.

The economics only work if the cheap gate (fakegcp) catches most defects. Today fakegcp is the least-mature mock in the fleet, so the cheap gate is leakier for GCP than for AWS/Scaleway. This plan closes that.

> **Scope boundary:** this plan matures the *mock*. Building the *real* GCP Layer 3 harness (project bootstrap, billing, credentials, auto-destroy) is separate work that lands in **infrafactory**, and it depends on this plan being done first.

---

## Where fakegcp stands today

| Dimension | State |
|---|---|
| Landed services | **15** (compute, LB stack, GKE, Cloud SQL, IAM, storage, pub/sub, DNS, Cloud Run, secret manager, KMS, service-networking, service-usage, memorystore, cloudresourcemanager) |
| Tests | 150 handler + 27 repository funcs; 17 FK-violation + 6 cascade; **13 regression patterns** (M79) |
| Idempotency gate | `examples/known_broken.yaml` **empty** — every `examples/working` dir passes `apply → plan(no-op) → destroy` |
| Handler coverage | **~64%** (fleet-lowest; fakeaws is 82.4%) |
| Release | pre-1.0, **untagged** |
| Contract audit | `handlers/contract_audit_test.go` present; `CRITICAL[<id>]` annotations sparse vs fakegenesys |

Not immature — narrower and less battle-tested than fakeaws, with two genuinely thin spots (KMS, coverage) and one GCP-only structural ceiling (escape-to-real-cloud).

---

## Workstreams

### WS1 — KMS: stub → real handler set *(P0)*

Cloud KMS is CMEK-only, in-memory synthesised, **zero tests**. Any encryption-at-rest scenario currently rests on nothing.

- [ ] SQLite-backed key rings + crypto keys + IAM policies (drop the in-memory map)
- [ ] FK enforcement: crypto key → key ring; reject create against missing ring (404)
- [ ] Handler tests + FK-violation test + cascade test
- [ ] `examples/working/kms` driven through the idempotency gate
- [ ] Flip nothing in `LandedServices` (already listed) — but make the listing honest

**Effort:** ~2–3 days. **Exit:** `kms` has ≥1 working example passing `plan -detailed-exitcode 0` and matches the coverage bar the other services hold.

### WS2 — Coverage 64% → 80%+ *(P0)*

Several services are unit-tested only, never driven end-to-end through tofu.

- [ ] Run `make test-coverage`; rank handlers by uncovered lines
- [ ] Add targeted handler tests for the cold paths (error branches, update paths, list-empty shapes)
- [ ] Promote unit-only services to `examples/working` + `examples/updates` coverage where a tofu drive is feasible
- [ ] Gate: CI coverage floor set to 80% so it can only ratchet up

**Effort:** ~3–4 days. **Exit:** `handlers` package ≥80%, floor enforced in CI.

### WS3 — Escape-to-real-cloud ceiling: spike *(P0 — do this FIRST, it's a go/no-go)*

fakegcp alone has provider calls that **bypass `*_custom_endpoint` and hit real Google APIs even in mock mode**:

| Resource | Mechanism | Today's coping strategy |
|---|---|---|
| `google_project_service` | v5+ preflight builds its own serviceusage client | M70 serviceusage stub (partial) |
| `google_service_networking_connection` | same preflight escape | prompt-avoid + S78 carve-out |
| `google_project_iam_member` | `BatchingConfig` builds its own cloudresourcemanager client | `batching { enable_batching = false }` |
| `google_container_node_pool` | `GetNodePool` missing fields SDK derefs → `plugin did not respond` | field-population fixes (M45) |

**Some of these may be genuinely unmockable** — the provider hardcodes the client. This is a *fidelity ceiling*, not a backlog item.

- [ ] For each offender: can we model the exact endpoint the preflight client hits, or does the provider hardcode `googleapis.com`?
- [ ] Produce a verdict per resource: **mockable / partially-mockable / unmockable-by-design**
- [ ] For unmockable ones, decide the honest handling: keep the prompt-avoid + carve-out, and document them in `README.md` "Known Limitations" as permanent
- [ ] Write findings to `docs/escape-ceiling-spike.md`

**Effort:** ~2 days. **Exit:** a documented, per-resource verdict. This bounds how good the GCP cheap-gate can ever be — needed before committing to GCP as the twin.

### WS4 — Service breadth *(P1 — scope to target scenarios only)*

15 services covers a lot but misses common infra. **Don't build speculatively** — add only what the talk's demo scenarios need. Candidates by likely demand:

- [ ] Serverless VPC Access connector (`google_vpc_access_connector`) — needed if Cloud Run touches a VPC
- [ ] Cloud NAT / Router NAT — needed for egress scenarios
- [ ] Artifact Registry — needed if any container scenario pushes images
- [ ] (defer: Cloud Functions, BigQuery, Cloud Armor, Certificate Manager)

**Effort:** ~1–2 days *each* via the repo's per-bundle PR pattern (handler + repo + examples + tests + `coverage_matrix.yaml` + `LandedServices` flip, all in one PR).

### WS5 — Contract-audit depth *(P1)*

- [ ] Adopt fakegenesys's `CRITICAL[<id>]:` / `MUST[<id>]:` docstring → `TestContract_<id>` convention densely on load-bearing invariants (self-link rewrite, empty-list `items` omission, operation-DONE envelope, numericID 18-digit cap)
- [ ] Verify `contract_audit_test.go` enforces the pairing (no orphan tags, no orphan tests)

**Effort:** ~1–2 days.

### WS6 — Tagged release + OSS polish *(P2)*

- [ ] Cut `v0.1.0` (SECURITY/COC/CONTRIBUTING already present)
- [ ] CHANGELOG `[Unreleased]` → `[0.1.0]`

**Effort:** hours.

---

## Sequencing

```
WS3 (escape spike, go/no-go) ──┐
                               ├─▶ WS1 (KMS) ──┐
                               │               ├─▶ WS4 (breadth, as needed) ─▶ WS5 ─▶ WS6 (tag)
                               └─▶ WS2 (coverage) ┘
```

Do **WS3 first** — it's the go/no-go on whether GCP can be a good cheap-gate at all. WS1 + WS2 are the parallel P0 core. WS4 is demand-driven. WS5/WS6 are finish work.

**Total to fakeaws-parity:** ~1.5–2 weeks of focused work, plus the WS3 spike.

## Definition of done

- `handlers` coverage ≥80%, CI floor enforced
- KMS is SQLite-backed with a passing working example
- Escape ceiling documented with a per-resource verdict
- `known_broken.yaml` still empty (no regressions)
- Tagged `v0.1.0`
- infrafactory `TestE2E_GCP*` still green; a fresh `make sweep-N` shows no GCP regression

## Then: hand off to infrafactory

Once fakegcp is the trustworthy cheap gate, the GCP Layer 3 harness is built in **infrafactory** (separate plan): real-GCP project bootstrap (org + billing-account linkage + project-creator SA), credential handling, `plan-live.txt` capture, auto-destroy on failure — mirroring the Scaleway Slices 26–30 work but for GCP's heavier project ceremony.
