# BROKEN: secret version pointing at a secret that doesn't exist.
#
# `google_secret_manager_secret_version.secret` accepts a fully-qualified
# secret resource name. When the operator hand-writes that path (instead
# of using a resource reference) and the secret hasn't been created,
# Terraform happily plans the version create.
#
# ── Why standard tooling does not catch this ────────────────────────────
#
#   terraform validate  ✓ passes — `secret` is typed string
#   terraform plan      ✓ passes — the path looks well-formed
#
# ── What fakegcp catches ────────────────────────────────────────────────
#
#   $ terraform apply
#   Error: googleapi: Error 404: secret "missing-secret" not found
#
#   fakegcp's secretmanager_versions table joins to secretmanager_secrets;
#   the addVersion call rejects because the parent secret row doesn't
#   exist.
resource "google_secret_manager_secret_version" "broken" {
  secret      = "projects/fake-project/secrets/missing-secret"
  secret_data = "fakegcp-test-payload"
}
