# Update flow for google_secret_manager_secret: add a label.
# (Versions are immutable so the secret itself is what we patch.)
#
#   terraform apply -var-file=v1.tfvars -auto-approve
#   terraform apply -var-file=v2.tfvars -auto-approve
#   terraform destroy -var-file=v2.tfvars -auto-approve
variable "rotation_label" {
  type = string
}

resource "google_secret_manager_secret" "main" {
  secret_id = "db-credentials"

  labels = {
    rotation = var.rotation_label
  }

  replication {
    auto {}
  }
}

resource "google_secret_manager_secret_version" "main" {
  secret      = google_secret_manager_secret.main.id
  secret_data = "fakegcp-test-payload"
}
