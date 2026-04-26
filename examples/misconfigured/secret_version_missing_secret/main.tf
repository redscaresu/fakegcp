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
#   Error: googleapi: Error 404: Referenced resource not found
#
#   The Secret Manager addVersion handler resolves the parent secret
#   before inserting a version. If the parent secret doesn't exist in
#   this project, the call is rejected.
resource "google_secret_manager_secret_version" "broken" {
  secret      = "projects/fake-project/secrets/missing-secret"
  secret_data = "fakegcp-test-payload"
}
