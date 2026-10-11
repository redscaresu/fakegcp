# WS3 Spike: The GCP Escape-to-Real-Cloud Ceiling

**Question (go/no-go):** Is `terraform-provider-google`'s "escape to real cloud" class a bounded, mockable set — or an open-ended risk that caps how good fakegcp can ever be as a Layer-2 pre-filter?

**Verdict:** **Bounded and understood. GCP is viable as a Layer-2 filter; the escape ceiling costs ~zero real filter coverage — but it carries a permanent empirical tax that Scaleway does not.**

> Scope note: `terraform-provider-google` source is not checked out locally, so provider-internal mechanisms are graded `CONFIRMED` (backed by a fakegcp handler, a provider knob that demonstrably works, or an end-to-end validated run) vs `INFERRED` (from documented symptoms / TF_LOG traces / binary-strings hunts in infrafactory's investigation docs).

## Per-resource verdicts

| Resource | Root cause | Verdict | Confidence | fakegcp status |
|---|---|---|---|---|
| `google_project_service` | v5 `getProject` preflight hits Resource-Manager **v3**; only the v1 endpoint override was wired, so v3 escaped. Plus a serviceusage post-enable poll that 404'd. Both are *missing-override / missing-handler*, not hardcoded host. | **MOCKABLE** | CONFIRMED | Fully mocked — `serviceusage.go` (M70) + CRM v1 **and** v3 GetProject; infrafactory wired `resource_manager_v3_custom_endpoint`. Validated `gcp-cloud-sql` → target_reached. |
| `google_service_networking_connection` | Two paths: (a) v3 getProject preflight — **closed**. (b) servicenetworking's internal `retrieveProject` (projectID→number) builds its **own** CRM client with **no override knob**. | **UNMOCKABLE-BY-DESIGN** (path b) | escape CONFIRMED; mechanism INFERRED | Handlers complete + probe 200, but request (b) **never reaches fakegcp**. Worked around LLM-side: omit SNC, attach `private_network` directly to the SQL instance. |
| `google_project_iam_member` | `BatchingConfig` aggregates IAM writes through a **self-constructed** CRM client that ignores the endpoint. Possible deeper v5 IAM path too. | **MOCKABLE-WITH-PROVIDER-CONFIG** (`batching { enable_batching = false }`) | batching CONFIRMED (PR #23); deeper path INFERRED | `iam_bindings.go` serves project IAM. infrafactory chose defense-in-depth: retired project-level IAM from prompts, substituting SA-level `google_service_account_iam_member` (routes via iam.googleapis.com → fully mocked). |
| `google_container_node_pool` | **Not an escape.** `GetNodePool` omitted nested fields the v5 SDK derefs without nil guards → provider panic surfacing as `plugin did not respond`. Request stays on fakegcp. | **MOCKABLE** (misclassified as escape) | CONFIRMED | Fully mocked — `container.go::populateNodePoolDefaults` fills every deref'd sub-block (incl. `maxPodsPerNode` as JSON string, `diskSizeGb` as number). Validated `gcp-gke-cluster` → target_reached iter 1. |

## Bounded vs. open-ended

**Bounded and mechanism-predictable; membership discovered empirically.** The escape is one precise thing: *the provider builds an internal API client that doesn't thread the user's `*_custom_endpoint`.* Two sub-mechanisms:

1. **Missing-override escapes** (project_service; SNC preflight leg) — provider uses a service/version whose override wasn't wired. **Fully closable**: add override + handler. Discoverable *before* they bite by binary-strings-hunting the provider for `GOOGLE_*_CUSTOM_ENDPOINT` env vars.
2. **No-knob internal clients** (SNC `retrieveProject`; deeper IAM) — no corresponding override exists. IAM happened to expose `enable_batching=false`; SNC exposes nothing. **Unmockable by design** until the provider ships a knob.

**Reliable detector:** a real escape returns GCP's `reason: ACCESS_TOKEN_TYPE_UNSUPPORTED`; fakegcp emits `reason: required`. infrafactory fingerprints exactly this to route escapes to the pitfalls carve-out (S78) instead of faking a mock.

**At-risk population is small and characterizable:** meta / cross-service-networking resources that resolve a project number or do a cross-service preflight (API enablement, Private Service Access, project-level IAM aggregation). Ordinary compute/storage/SQL/DNS/GKE/SA-IAM route cleanly. True escape set ≈ **2–3 resources**, all meta/networking, with a stable predictor *and* detector. Finite and understood — but not *closed* (SNC remains a corner; it's the one persistent `gcp-full-stack` non-convergence).

## Recommendation

GCP **is** viable as a Layer-2 pre-filter. Every escaping resource is a meta/networking resource that is *unnecessary against the mock target* (fakegcp implicitly enables APIs, synthesizes private endpoints, accepts `private_network` on SQL directly; SA-level IAM substitutes for project-level). So the LLM-avoidance workaround loses **~zero real filter coverage**, and 3 of 4 offenders are genuinely mocked. The cost is paid in prompt engineering + pitfalls + one unmockable corner — **not** in filter fidelity.

But that tax is **permanent and empirical**: each new escape must be swept, fingerprinted, and endpoint-patched or carved out. **Scaleway pays no escape tax** — the provider honors `SCW_API_URL` uniformly, there is no internal-client-bypass class, and Layer 3 is already built.

**Decision for the talk:** use **Scaleway as the demo spine / real twin** (clean, low-variance, ready). Keep **GCP's bounded-escape story as a narrative contrast** — "even keeping a change *off* real cloud is hard; `custom_endpoint` isn't airtight" is a strong, true beat for the *why plan/mock can't see reality* section. fakegcp maturation (WS1/WS2) is **not on the talk's critical path** under this decision; it stays a background quality investment, not a blocker.
