# Service account + key against fakegcp's IAM handler set
# (handlers/iam.go). Project-level IAM bindings are exercised
# separately by the iam_bindings handlers; this example focuses
# on the service-account life-cycle which is the most common shape.
resource "google_service_account" "ci" {
  account_id   = "ci-runner"
  display_name = "CI runner service account"
}

resource "google_service_account_key" "ci" {
  service_account_id = google_service_account.ci.name
  key_algorithm      = "KEY_ALG_RSA_2048"
}
