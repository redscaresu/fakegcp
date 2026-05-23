# BROKEN: GKE node pool's `cluster` references a cluster that was
# never declared. A common variant is splitting cluster + node-pool
# config across modules and importing the wrong module instance — the
# node pool then targets a cluster that exists in a different state
# file (or nowhere at all).
#
# ── Why standard tooling does not catch this ────────────────────────────
#
#   terraform validate  ✓ passes — `cluster` is typed string
#   terraform plan      ✓ passes — Terraform cannot verify the cluster
#                                  exists in this project
#
# ── What fakegcp catches ────────────────────────────────────────────────
#
#   $ terraform apply
#   Error: googleapi: Error 404: The resource was not found
#
#   The node pool create path is keyed by cluster name in the URL; the
#   container handler resolves the parent cluster before insert. A
#   missing cluster row returns models.ErrNotFound → 404. See
#   handlers/fk_violation_test.go::TestNodePoolFKViolationViaRelativePath.
resource "google_container_node_pool" "orphan" {
  name       = "orphan-pool"
  location   = "us-central1"
  cluster    = "ghost-cluster"
  node_count = 1

  node_config {
    machine_type = "e2-medium"
  }
}
