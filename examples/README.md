# fakegcp examples

fakegcp is a local Google Cloud API mock that catches infrastructure mistakes at apply time — mistakes that `terraform validate`, `terraform plan`, and `terraform test` all let through.

The broken configs in this directory are valid Terraform. They pass validation. They produce a clean plan. The errors only surface when the provider actually calls the API and fakegcp enforces the same FK constraints as the real Google Cloud API.

---

## Prerequisites

- Go 1.21+
- Terraform or OpenTofu

---

## Step 1 — Install fakegcp

```bash
go install github.com/redscaresu/fakegcp/cmd/fakegcp@latest
```

---

## Step 2 — Start fakegcp

Open a dedicated terminal and leave it running:

```bash
fakegcp --port 8080
```

To confirm fakegcp is ready:

```bash
curl -s http://localhost:8080/mock/state
```

---

## Step 3 — Run an example

Each `working/` and `misconfigured/` example is self-contained: `cd` into it and run the usual Terraform workflow.

```bash
cd working/basic_instance

terraform init
terraform apply -auto-approve
terraform destroy -auto-approve
```

The `misconfigured/` examples will error during apply with a 404 from fakegcp. The comments at the top of each `main.tf` show the error shape and how to fix it.

The `updates/` examples are split across two `tfvars` files so the apply-then-update lifecycle is explicit:

```bash
cd updates/update_pubsub_subscription

terraform init
terraform apply -var-file=v1.tfvars -auto-approve
terraform apply -var-file=v2.tfvars -auto-approve   # in-place patch
terraform destroy -var-file=v2.tfvars -auto-approve
```

---

## Step 4 — Reset state between runs

fakegcp holds state in SQLite (in-memory by default). After a failed apply, partial resources may remain. Reset without restarting:

```bash
curl -s -X POST http://localhost:8080/mock/reset
```

Inspect current state at any time:

```bash
curl -s http://localhost:8080/mock/state | jq .
```

---

## Examples

### working

Configs that apply, can be updated, and destroy cleanly. These show the right way to express GCP resource dependencies so that fakegcp's FK constraints — and the real API's — are satisfied.

| Example | What it demonstrates |
|---|---|
| `working/basic_instance` | Network → subnetwork → instance dependency chain (compute) |
| `working/cloud_sql` | Cloud SQL instance, database, and user |
| `working/cloud_run` | Cloud Run v2 service with a placeholder image |
| `working/dns` | Managed zone with a default A record |
| `working/gke_cluster` | GKE cluster + node pool |
| `working/iam` | Service account with a service-account key |
| `working/load_balancer` | Global external HTTPS LB stack: address → health-check → backend → URL-map → cert + HTTPS proxy → forwarding rule |
| `working/pubsub` | Pub/Sub topic + pull subscription |
| `working/secret_manager` | Secret Manager secret with one initial version |
| `working/storage` | Cloud Storage bucket with uniform bucket-level access + CMEK |

### misconfigured

Valid Terraform that produces a clean plan, but the apply fails because fakegcp enforces the same FK constraints as the real Google Cloud API.

| Example | What fakegcp catches |
|---|---|
| `misconfigured/instance_missing_network` | Subnetwork references a non-existent VPC network |
| `misconfigured/subscription_missing_topic` | Pub/Sub subscription references a topic that doesn't exist in the project |
| `misconfigured/record_set_missing_zone` | DNS record set is created in a managed zone that hasn't been declared |
| `misconfigured/secret_version_missing_secret` | Secret version points at a fully-qualified secret path that doesn't resolve |
| `misconfigured/backend_missing_health_check` | Backend service `health_checks` self-link doesn't resolve to a real health check |

### updates

Update scenarios that verify in-place resource modifications work correctly. Each directory contains `main.tf` (with variables), `v1.tfvars` (initial state), and `v2.tfvars` (updated state). The test cycle is: apply v1 → apply v2 → destroy.

| Example | What it demonstrates |
|---|---|
| `updates/update_pubsub_subscription` | Patch `ack_deadline_seconds` on an existing subscription |
| `updates/update_dns_record` | Change a record-set TTL via the v1 transactional changes API |
| `updates/update_cloud_run_service` | Add or change service labels on Cloud Run v2 |
| `updates/update_secret_manager_secret` | Patch labels on the secret resource (versions are immutable) |
| `updates/update_load_balancer_backend` | Update a backend service's description |
| `updates/update_iam_service_account` | Rename a service account via display_name |
| `updates/update_storage_bucket` | Add labels to a Cloud Storage bucket |
