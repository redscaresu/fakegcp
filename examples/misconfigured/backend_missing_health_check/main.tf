# BROKEN: backend service references a health check by full self-link
# that never got created.
#
# Real-world variant: the operator copies a health-check self-link from
# a Slack snippet, but the corresponding `google_compute_health_check`
# resource was deleted before the backend service was applied.
#
# ── Why standard tooling does not catch this ────────────────────────────
#
#   terraform validate  ✓ passes — `health_checks` is a list of strings
#   terraform plan      ✓ passes — Terraform cannot dereference the
#                                  self-link to confirm it exists
#
# ── What fakegcp catches ────────────────────────────────────────────────
#
#   $ terraform apply
#   Error: googleapi: Error 404: health check "missing-hc" not found
#
#   fakegcp's compute_backend_services FK validates that every entry in
#   `healthChecks` resolves to an existing health check before insert.
resource "google_compute_backend_service" "broken" {
  name          = "lb-be"
  protocol      = "HTTP"
  health_checks = ["projects/fake-project/global/healthChecks/missing-hc"]
}
