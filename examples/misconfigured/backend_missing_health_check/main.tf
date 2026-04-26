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
#   Error: googleapi: Error 404: The resource was not found
#
#   fakegcp's CreateBackendService handler resolves every entry in
#   `healthChecks` to an existing health check before insert. The
#   reference must be a bare name or a same-project, same-collection
#   self-link; cross-project or cross-collection self-links are
#   rejected with a 400 even if a same-named local resource exists.
resource "google_compute_backend_service" "broken" {
  name          = "lb-be"
  protocol      = "HTTP"
  health_checks = ["projects/fake-project/global/healthChecks/missing-hc"]
}
